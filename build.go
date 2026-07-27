package hypertexting

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/sync/errgroup"

	hypertemplates "github.com/herdworks/hypertemplates-go"
	"github.com/herdworks/hypertemplates-go/directives"
	"github.com/herdworks/hypertemplates-go/document"
	htfiles "github.com/herdworks/hypertemplates-go/files"
	htvariable "github.com/herdworks/hypertemplates-go/variable"
)

type Build struct {
	URI           *url.URL
	ID            string
	Website       *Website
	Config        BuildConfig
	Environment   Environment
	Incremental   bool
	IncludeDrafts bool
	Plugins       htvariable.Plugins

	Pages  *Pages
	Status BuildStatus
}

type BuildConfig struct {
	WebsiteConfig `json:",inline" yaml:",inline"`

	BaseURL  string `json:"base_url,omitempty" yaml:"base_url,omitempty"`
	DataFile string `json:"data_file,omitempty" yaml:"data_file,omitempty"`
	LogFile  string `json:"log_file,omitempty" yaml:"log_file,omitempty"`
	Verbose  bool   `json:"verbose,omitempty" yaml:"verbose,omitempty"`
}

type BuildStatus struct {
	Assets     Counter `json:"assets,omitempty" yaml:"assets,omitempty"`
	Tags       Counter `json:"tags,omitempty" yaml:"tags,omitempty"`
	Bytes      Counter `json:"bytes,omitempty" yaml:"bytes,omitempty"`
	DurationMS int     `json:"duration_ms,omitempty" yaml:"duration_ms,omitempty"`
}

type BuildResult struct {
	ID         string         `json:"id,omitempty"`
	URI        string         `json:"uri,omitempty"`
	PageCount  int            `json:"page_count"`
	FeedCount  int            `json:"feed_count"`
	Assets     map[string]int `json:"assets,omitempty"`
	Tags       map[string]int `json:"tags,omitempty"`
	Bytes      map[string]int `json:"bytes,omitempty"`
	DurationMS int            `json:"duration_ms"`
}

type synchronizedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (w *synchronizedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.w.Write(p)
}

func (build *Build) Execute(ctx context.Context) (err error) {
	if build.URI == nil || build.URI.Path == "" {
		return fmt.Errorf("build.uri is required")
	}

	// load an io.Writer for stdout & optional file logging
	w, wc, err := build.logger()
	if err != nil {
		return err
	}
	defer wc() // close the log writer
	fmt.Fprintf(w, "build %q starting\n", build.ID)

	// measure build execution time from here
	start := time.Now()

	// load the build context
	if err = build.loadContext(ctx, w); err != nil {
		return err
	}

	// load plugins
	build.Plugins = htvariable.MergePlugins(build.Website.Theme.Plugins, build.Website.Plugins)

	// validate build
	if err = build.Validate(); err != nil {
		return err
	}

	// perform build
	writegroup, _ := errgroup.WithContext(ctx)
	writegroup.Go(func() error { return build.WriteAssets(ctx, w) })
	writegroup.Go(func() error { return build.WriteFeeds(ctx, w) })
	writegroup.Go(func() error { return build.WritePages(ctx, w) })
	if err = writegroup.Wait(); err != nil {
		return err
	}

	// finalize build
	build.Status.DurationMS = int(time.Since(start).Milliseconds())
	result := build.Result()
	build.writeBuildData(result, w)
	build.writeDeploymentIgnore(w)
	fmt.Fprintf(w, "build %q completed\n", build.ID)
	build.logResult(result, w)
	return nil
}

func (build *Build) logger() (w io.Writer, wc func(), err error) {
	w = io.Writer(os.Stdout)
	wc = func() {}
	if build.Config.LogFile != "" {
		if filepath.IsAbs(build.Config.LogFile) {
			return nil, nil, fmt.Errorf("build artifact path must be relative: %s", build.Config.LogFile)
		}
		if strings.HasSuffix(build.Config.LogFile, "/") || strings.HasSuffix(build.Config.LogFile, string(filepath.Separator)) {
			return nil, nil, fmt.Errorf("build artifact path must be a file: %s", build.Config.LogFile)
		}
		logFile, err := build.AbsolutePath(build.Config.LogFile)
		if err != nil {
			return nil, nil, err
		}
		if err := os.MkdirAll(filepath.Dir(logFile), 0755); err != nil {
			return nil, nil, err
		}
		file, err := os.OpenFile(logFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			return nil, nil, err
		}
		wc = func() { _ = file.Close() }
		w = io.MultiWriter(w, file)
	}
	return &synchronizedWriter{w: w}, wc, nil
}

// LoadContext populates the build's data context in place: it loads
// the site config and environment, parses static and theme template
// data, discovers and resolves pages (including feeds and tag pages),
// and runs the computed-data scripts. When it returns,
// build.Website.GlobalData, build.Website.Theme.TemplateData,
// build.Website.Pages, and each page's resolved template data are
// ready to read. No assets, feeds, or pages are written.
//
// Inspection callers (e.g. `hyperctl cms data ls/inspect`) call
// LoadContext and then read build.Context() to view the unified
// template context without producing a build artifact.
//
// Build.Config.Verbose toggles the per-file "loading" log lines emitted while
// loading the website, data files, pages, and computed-data scripts.
func (build *Build) LoadContext(ctx context.Context, w io.Writer) (err error) {
	if w == nil {
		w = os.Stdout
	}
	return build.loadContext(ctx, w)
}

func (build *Build) loadContext(ctx context.Context, w io.Writer) (err error) {
	build.Reset()
	usesWebsitePages := build.Website != nil && build.Pages == &build.Website.Pages
	requestedPages := build.Pages != nil && len(*build.Pages) > 0 && !usesWebsitePages
	if !build.Incremental && requestedPages {
		return fmt.Errorf("build.pages is only supported for incremental builds")
	}
	if build.Incremental && (build.Pages == nil || len(*build.Pages) == 0) {
		return fmt.Errorf("build.pages is required for incremental builds")
	}
	if build.Pages == nil {
		build.Pages = &Pages{}
	}

	// load website
	if err = build.Website.Load(build.Config.Verbose, &build.Config.WebsiteConfig, w); err != nil {
		return err
	}
	if err = build.Environment.Load(); err != nil {
		return err
	}
	if build.Config.BaseURL != "" {
		build.Website.BaseURL = build.Config.BaseURL
	}
	if build.Website.GlobalData, err = NamespacedTemplateData(build.Website, build.Website.Config.Markdown, build.Config.Verbose, w); err != nil {
		return err
	}
	if build.Website.Theme.TemplateData, err = ThemeTemplateData(build.Website, build.Website.Config.Markdown, build.Config.Verbose, w); err != nil {
		return err
	}
	if build.Website.Pages, err = GetPages(build.Website, build.Website.ContentDirPath()); err != nil {
		return err
	}
	if !build.Incremental {
		build.Pages = &build.Website.Pages
	}

	// load build pages first (in case of incremental build)
	if err = LoadPages(ctx, build.Website, *build.Pages, build.Website.Pages, build.Config.Verbose, w); err != nil {
		return err
	}
	// load website pages second (sitemap)
	if err = LoadPages(ctx, build.Website, build.Website.Pages, *build.Pages, build.Config.Verbose, w); err != nil {
		return err
	}
	if !build.IncludeDrafts {
		build.Website.Pages = build.Website.Pages.FilterDraft(false)
		*build.Pages = build.Pages.FilterDraft(false)
	}
	build.Website.TemplateData.Set("pages", build.Website.Pages.TemplateData())

	// check if build pages are in any feeds
	if build.Incremental {
		if err = AddFeedPages(ctx, build, w); err != nil {
			return err
		}
	}

	// load page feeds
	if err = LoadPageFeeds(ctx, build.Website, *build.Pages, w); err != nil {
		return err
	}
	// load tag pages
	if err = LoadTagPages(build, w); err != nil {
		return err
	}
	build.Pages.ResolveTemplateData(build.Website)

	// Run computed-data scripts. Each script observes the full
	// static-data context (including site.pages) and installs its
	// result into Website.GlobalData; per-page renders read these
	// values back through TemplateData.
	return generateComputedData(build, build.Config.Verbose, w)
}

// Context returns the unified template context that page templates
// and computed-data scripts read during a build: ht, build, env, site,
// data, theme. Call LoadContext first to populate it; otherwise the
// entries are empty.
//
// build.date and ht.release_date resolve at call time, so each
// invocation captures a fresh timestamp.
func (build *Build) Context() hypertemplates.TemplateData {
	siteData := hypertemplates.TemplateData{}
	var globalData hypertemplates.TemplateData
	var themeData hypertemplates.TemplateData
	if build.Website != nil {
		if build.Website.TemplateData != nil {
			siteData = *build.Website.TemplateData
		}
		globalData = build.Website.GlobalData
		themeData = build.Website.Theme.TemplateData
	}
	return hypertemplates.TemplateData{
		"ht": hypertemplates.TemplateData{
			"version":      LibVersion,
			"release_date": LibReleaseDate.Format(time.RFC3339),
		},
		"build": hypertemplates.TemplateData{
			"id":   build.ID,
			"date": time.Now().Format(time.RFC3339),
		},
		"env":   build.Environment.TemplateData,
		"site":  siteData,
		"data":  globalData,
		"theme": themeData,
	}
}

func (build *Build) Reset() {
	build.Status.Reset()
}

func (build *Build) Validate() (err error) {
	if build.URI == nil {
		return fmt.Errorf("build.uri is required")
	}
	if build.URI.Path == "" {
		return fmt.Errorf("build.uri is required")
	}
	if filepath.IsAbs(build.Config.DataFile) {
		return fmt.Errorf("build artifact path must be relative: %s", build.Config.DataFile)
	}
	if filepath.IsAbs(build.Config.LogFile) {
		return fmt.Errorf("build artifact path must be relative: %s", build.Config.LogFile)
	}
	if strings.HasSuffix(build.Config.DataFile, "/") || strings.HasSuffix(build.Config.DataFile, string(filepath.Separator)) {
		return fmt.Errorf("build artifact path must be a file: %s", build.Config.DataFile)
	}
	if strings.HasSuffix(build.Config.LogFile, "/") || strings.HasSuffix(build.Config.LogFile, string(filepath.Separator)) {
		return fmt.Errorf("build artifact path must be a file: %s", build.Config.LogFile)
	}
	if build.Config.DataFile != "" {
		if _, err := build.AbsolutePath(build.Config.DataFile); err != nil {
			return err
		}
	}
	if build.Config.LogFile != "" {
		if _, err := build.AbsolutePath(build.Config.LogFile); err != nil {
			return err
		}
	}
	if build.Pages == nil {
		return fmt.Errorf("build.pages is required")
	}
	if len(*build.Pages) == 0 {
		return fmt.Errorf("build.pages is empty")
	}
	return nil
}

func (build *Build) WriteAssets(ctx context.Context, w io.Writer) (err error) {
	if w == nil {
		w = os.Stdout
	}

	assets, err := AssetEntries(&build.Website.Theme, build.Website, *build.Pages)
	if err != nil {
		return err
	}
	if build.Incremental {
		assets = assets.FilterSource(AssetSourcePage)
	}
	for _, asset := range assets {
		build.Status.Assets.Increment(1, build.AssetCounterKey(asset.RelPath))
	}
	taskgroup, _ := errgroup.WithContext(ctx)
	for _, asset := range assets {
		asset := asset
		taskgroup.Go(func() error {
			return build.copyAsset(asset.SourcePath, asset.RelPath, w)
		})
	}
	return taskgroup.Wait()
}

func (build *Build) CopyAsset(asset Asset, w io.Writer) error {
	if w == nil {
		w = os.Stdout
	}
	return build.copyAsset(asset.SourcePath, asset.RelPath, w)
}

func (build *Build) CopyThemeAsset(relPath string, w io.Writer) error {
	if w == nil {
		w = os.Stdout
	}

	srcPath, err := build.Website.ResourcePath(build.Website.Theme.StaticDirPath(), relPath)
	if err != nil {
		return err
	}
	return build.copyAsset(srcPath, relPath, w)
}

func (build *Build) CopyWebsiteAsset(relPath string, w io.Writer) error {
	if w == nil {
		w = os.Stdout
	}

	srcPath, err := build.Website.ResourcePath(build.Website.StaticDirPath(), relPath)
	if err != nil {
		return err
	}
	return build.copyAsset(srcPath, relPath, w)
}

func (build *Build) CopyPageAsset(relPath string, w io.Writer) error {
	if w == nil {
		w = os.Stdout
	}

	srcPath, err := build.Website.ResourcePath(build.Website.ContentDirPath(), relPath)
	if err != nil {
		return err
	}
	return build.copyAsset(srcPath, relPath, w)
}

func (build *Build) copyAsset(srcPath string, relPath string, w io.Writer) (err error) {
	var srcFile *os.File
	var dstPath string
	var dstFile *os.File
	var dstFileBytesWritten int64
	if dstPath, err = build.Website.AssetBuildPath(build, relPath); err != nil {
		return err
	}
	dstRelPath, err := build.RelativePath(dstPath)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "copying \"%s\" to \"%s\"\n", build.Website.RelativePath(srcPath), filepath.ToSlash(filepath.Join(build.ID, dstRelPath)))
	if err = os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		return err
	}
	if srcFile, err = os.Open(srcPath); err != nil {
		return err
	} else {
		defer srcFile.Close()
	}
	if dstFile, err = os.Create(dstPath); err != nil {
		return err
	} else {
		defer dstFile.Close()
	}
	if dstFileBytesWritten, err = io.Copy(dstFile, srcFile); err != nil {
		return err
	}
	build.Status.Bytes.Increment(int(dstFileBytesWritten), "assets")
	return nil
}

// AssetCounterKey returns the normalized asset counter map key for relPath.
//
// Asset public paths are rooted with "/", but Counter keys are stored without a
// leading slash so the same relative asset path is used for display and lookup.
func (build *Build) AssetCounterKey(relPath string) string {
	publicPath, err := build.Website.AssetPublicPath(relPath)
	if err != nil {
		return relPath
	}
	return strings.TrimPrefix(publicPath, "/")
}

func (build *Build) WriteFeeds(ctx context.Context, w io.Writer) (err error) {
	if w == nil {
		w = os.Stdout
	}

	taskgroup, _ := errgroup.WithContext(ctx)
	for _, page := range *build.Pages {
		taskgroup.Go(func() error {
			// skip non-feed pages
			if page.Feed == nil {
				return nil
			}
			return page.Feed.WriteAtom(build, page.Path, w)
		})
	}
	return taskgroup.Wait()
}

func (build *Build) WritePages(ctx context.Context, w io.Writer) (err error) {
	if w == nil {
		w = os.Stdout
	}

	taskgroup, _ := errgroup.WithContext(ctx)
	for i := range *build.Pages {
		taskgroup.Go(func() error {
			if (*build.Pages)[i].Draft && !build.IncludeDrafts {
				fmt.Fprintf(w, "draft \"%s\" skipped\n", build.Website.RelativePath((*build.Pages)[i].URI.Path))
				return nil
			}
			return build.WritePage((*build.Pages)[i], w)
		})
	}
	return taskgroup.Wait()
}
func (build *Build) WritePage(page *Page, w io.Writer) (err error) {
	if w == nil {
		w = os.Stdout
	}

	var filePath string = build.BuildPath(page)
	var fileBytes []byte
	if fileBytes, err = RenderPage(build, build.Website, page, w); err != nil {
		return fmt.Errorf("failed to generate %q with error: %w", build.Website.RelativePath(page.URI.Path), err)
	}
	if err = os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return err
	}
	build.Status.Bytes.Increment(len(fileBytes), "pages")
	return build.WriteFile(filePath, fileBytes, w)
}
func (build *Build) WriteFile(fileName string, fileBytes []byte, w io.Writer) (err error) {
	if w == nil {
		w = os.Stdout
	}

	var file *os.File
	var fileBytesWritten int
	fileRelPath, err := build.RelativePath(fileName)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "writing %d bytes to %s\n", len(fileBytes), filepath.ToSlash(filepath.Join(build.ID, fileRelPath)))
	if file, err = os.OpenFile(fileName, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644); err != nil {
		return err
	}
	defer file.Close()
	if fileBytesWritten, err = file.Write(fileBytes); err != nil {
		return err
	}
	if fileBytesWritten != len(fileBytes) {
		return fmt.Errorf("expected to write %v bytes, but wrote %v", len(fileBytes), fileBytesWritten)
	}
	return nil
}

func (build *Build) BuildPath(page *Page) string {
	publicPath := page.Path
	if publicPath == "" && build.Website != nil && page.URI != nil {
		publicPath = PagePath(build.Website, page.URI)
	}
	publicPath = strings.TrimPrefix(strings.ToLower(filepath.ToSlash(publicPath)), "/")
	var relPath string
	if publicPath == "" {
		relPath = "index.html"
	} else if strings.HasSuffix(publicPath, "/") {
		relPath = filepath.ToSlash(filepath.Join(publicPath, "index.html"))
	} else if filepath.Ext(publicPath) == ".html" {
		relPath = publicPath
	} else {
		relPath = filepath.ToSlash(filepath.Join(publicPath, "index.html"))
	}
	filePath, err := build.AbsolutePath(relPath)
	if err != nil {
		if build != nil && build.URI != nil {
			return filepath.Join(build.URI.Path, relPath)
		}
		return relPath
	}
	return filePath
}

func (build *Build) AbsolutePath(relPath string) (string, error) {
	if build == nil || build.URI == nil || build.URI.Path == "" {
		return "", fmt.Errorf("build.uri is required")
	}
	if filepath.IsAbs(relPath) {
		return "", fmt.Errorf("build path must be relative: %s", relPath)
	}
	relPath, err := htfiles.CleanRelativePath(relPath)
	if err != nil {
		return "", err
	}
	root, err := filepath.Abs(build.URI.Path)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, relPath), nil
}

func (build *Build) RelativePath(path string) (string, error) {
	if build == nil || build.URI == nil || build.URI.Path == "" {
		return "", fmt.Errorf("build.uri is required")
	}
	root, err := filepath.Abs(build.URI.Path)
	if err != nil {
		return "", err
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return "", err
	}
	relPath, err := filepath.Rel(root, path)
	if err != nil {
		return "", err
	}
	if filepath.IsAbs(relPath) || relPath == ".." || strings.HasPrefix(relPath, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("build path %q is outside build root %q", path, root)
	}
	return filepath.ToSlash(relPath), nil
}

func (*Build) logResult(result BuildResult, w io.Writer) {
	if w == nil {
		w = os.Stdout
	}
	fmt.Fprintf(w,
		"\nPages:  %8v (%s)\nAssets: %8v (%s)\nFeeds:  %8v (%s)\nTags:   %8v\n----------------\nTOTAL %8vms\n",
		result.PageCount, byteSize(result.Bytes["pages"]),
		len(result.Assets), byteSize(result.Bytes["assets"]),
		result.FeedCount, byteSize(result.Bytes["feeds"]),
		len(result.Tags),
		result.DurationMS,
	)
}

func (build *Build) Result() BuildResult {
	result := BuildResult{
		ID:         build.ID,
		Assets:     build.Status.Assets.Map(),
		Tags:       build.Status.Tags.Map(),
		Bytes:      build.Status.Bytes.Map(),
		DurationMS: build.Status.DurationMS,
	}
	if build.URI != nil {
		result.URI = build.URI.String()
	}
	if build.Pages != nil {
		result.PageCount = len(*build.Pages)
		for _, page := range *build.Pages {
			if page != nil && page.Feed != nil {
				result.FeedCount++
			}
		}
	}
	return result
}

func (build *Build) writeBuildData(result BuildResult, w io.Writer) {
	if w == nil {
		w = os.Stdout
	}
	if build.Config.DataFile == "" {
		return
	}
	dataFile, err := build.AbsolutePath(build.Config.DataFile)
	if err != nil {
		fmt.Fprintf(w, "build data %q skipped: %v\n", build.Config.DataFile, err)
		return
	}
	fileBytes, err := json.MarshalIndent(result, "", "\t")
	if err != nil {
		fmt.Fprintf(w, "build data %q skipped: %v\n", build.Config.DataFile, err)
		return
	}
	fileBytes = append(fileBytes, '\n')
	if err := os.MkdirAll(filepath.Dir(dataFile), 0755); err != nil {
		fmt.Fprintf(w, "build data %q skipped: %v\n", build.Config.DataFile, err)
		return
	}
	if err := build.WriteFile(dataFile, fileBytes, w); err != nil {
		fmt.Fprintf(w, "build data %q skipped: %v\n", build.Config.DataFile, err)
	}
}

func (build *Build) writeDeploymentIgnore(w io.Writer) {
	if w == nil {
		w = os.Stdout
	}
	paths, err := build.deploymentIgnoreArtifactPaths()
	if err != nil {
		fmt.Fprintf(w, "deployment ignore skipped: %v\n", err)
		return
	}
	if len(paths) == 0 {
		return
	}
	if build.URI == nil || build.URI.Path == "" {
		fmt.Fprintf(w, "deployment ignore skipped: build.uri is required\n")
		return
	}

	ignorePath, err := build.AbsolutePath(".deploymentignore")
	if err != nil {
		fmt.Fprintf(w, "deployment ignore skipped: %v\n", err)
		return
	}
	root := filepath.Dir(ignorePath)
	seen := map[string]bool{}
	lines := []string{}
	if data, err := os.ReadFile(ignorePath); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			entry := normalizeDeploymentIgnoreEntry(line)
			if entry == "" || seen[entry] {
				continue
			}
			seen[entry] = true
			lines = append(lines, entry)
		}
	} else if !os.IsNotExist(err) {
		fmt.Fprintf(w, "deployment ignore skipped: %v\n", err)
		return
	}

	changed := false
	for _, path := range paths {
		entry := normalizeDeploymentIgnoreEntry(path)
		if entry == "" || seen[entry] {
			continue
		}
		seen[entry] = true
		lines = append(lines, entry)
		changed = true
	}
	if !changed {
		return
	}
	if err := os.MkdirAll(root, 0755); err != nil {
		fmt.Fprintf(w, "deployment ignore skipped: %v\n", err)
		return
	}
	if err := build.WriteFile(ignorePath, []byte(strings.Join(lines, "\n")+"\n"), w); err != nil {
		fmt.Fprintf(w, "deployment ignore skipped: %v\n", err)
	}
}

func (build *Build) deploymentIgnoreArtifactPaths() ([]string, error) {
	var paths []string
	for _, configured := range []string{build.Config.DataFile, build.Config.LogFile} {
		if configured == "" {
			continue
		}
		absPath, err := build.AbsolutePath(configured)
		if err != nil {
			return nil, err
		}
		rel, err := build.RelativePath(absPath)
		if err != nil {
			return nil, err
		}
		paths = append(paths, rel)
	}
	return paths, nil
}

func (status *BuildStatus) Reset() {
	status.Assets.Reset()
	status.Tags.Reset()
	status.Bytes.Reset()
	status.DurationMS = 0
}

func normalizeDeploymentIgnoreEntry(entry string) string {
	entry = strings.TrimSpace(entry)
	entry = strings.TrimLeft(entry, "/")
	entry = filepath.ToSlash(entry)
	return entry
}

func byteSize(bytes int) string {
	if bytes < 1_024 {
		return fmt.Sprintf("%d B", bytes)
	}
	if bytes < 1_048_576 {
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	}
	if bytes < 1_073_741_824 {
		return fmt.Sprintf("%.1f MB", float64(bytes)/1_048_576)
	}
	return fmt.Sprintf("%.3f GB", float64(bytes)/1_073_741_824)
}

// NamespacedTemplateData loads the website's static data files, deriving
// each namespace from the file's path under the data directory. Files in
// subdirectories nest into the resulting TemplateData by their path
// segments (data/social/twitter.yaml -> data.social.twitter.*).
func NamespacedTemplateData(website *Website, config *hypertemplates.HyperMarkConfig, verbose bool, w io.Writer) (hypertemplates.TemplateData, error) {
	if w == nil {
		w = os.Stdout
	}
	return loadStaticData(website, website.DataDirPath(), config, verbose, w)
}

// ThemeTemplateData loads the theme's static data files using the same
// filename-derived namespace rules as NamespacedTemplateData.
func ThemeTemplateData(website *Website, config *hypertemplates.HyperMarkConfig, verbose bool, w io.Writer) (hypertemplates.TemplateData, error) {
	if w == nil {
		w = os.Stdout
	}
	return loadStaticData(website, website.Theme.DataDirPath(), config, verbose, w)
}

func NewTemplateDocument(website *Website, page *Page) (doc *document.Document, err error) {
	var layout *html.Node
	if layout, err = website.Theme.LayoutHTML(page.LayoutRef); err != nil {
		return nil, err
	}
	return &document.Document{
		Theme:          &website.Theme,
		Layout:         layout,
		TidyMode:       website.Config.TidyMode,
		MarkdownConfig: website.Config.Markdown,
	}, nil
}

func TemplateData(build *Build, website *Website, page *Page) (data *hypertemplates.TemplateData) {
	data = &hypertemplates.TemplateData{}
	data.Set("ht", hypertemplates.TemplateData{
		"version":      LibVersion,
		"release_date": LibReleaseDate.Format(time.RFC3339),
	})
	data.Set("build", hypertemplates.TemplateData{
		"id":   build.ID,
		"date": time.Now().Format(time.RFC3339),
	})
	data.Set("env", build.Environment.TemplateData)
	data.Set("site", *website.TemplateData)
	data.Set("data", website.GlobalData)
	data.Set("theme", website.Theme.TemplateData)
	data.Set("page", page.TemplateData)
	return data
}

func RenderPage(build *Build, website *Website, page *Page, w io.Writer) (pageBytes []byte, err error) {
	if w == nil {
		w = os.Stdout
	}

	var doc *document.Document
	if err = page.Validate(); err != nil {
		return nil, err
	}
	if page.Static {
		return os.ReadFile(page.URI.Path) // return static HTML file bytes without processing template
	}
	if doc, err = NewTemplateDocument(website, page); err != nil {
		return nil, err
	}
	doc.Plugins = build.Plugins
	if pageBytes, err = directives.Process(doc, TemplateData(build, website, page), &hypertemplates.TemplateData{"page": page.TemplateData}, w); err != nil {
		return nil, err
	}
	// page.Output = gohtml.Format(page.Output) // TODO: restore HTML formatting 🤌🏽
	return pageBytes, nil
}
