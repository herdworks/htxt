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
	htvalue "github.com/herdworks/hypertemplates-go/value"
)

type Build struct {
	URI           *url.URL
	ID            string
	Website       *Website
	Config        BuildConfig
	Environment   Environment
	Incremental   bool
	IncludeDrafts bool
	Plugins       htvalue.Plugins

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

	// load the io.Writer for stdout & optional file logging
	w, wc, err := build.logger()
	if err != nil {
		return err
	}
	defer wc() // close the log writer
	fmt.Fprintf(w, "build \"%s\" starting\n", filepath.Base(build.URI.Path))
	start := time.Now()

	// load the build context
	if err = build.loadContext(ctx, w); err != nil {
		return err
	}

	// load plugins
	build.Plugins = htvalue.Merge(build.Website.Theme.Plugins, build.Website.Plugins)

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
	if err = build.writeDeploymentIgnore(); err != nil {
		return err
	}

	// finalize build
	build.BuildStats(time.Since(start), w)
	return build.writeDataArtifact(w)
}

func (build *Build) logger() (w io.Writer, wc func(), err error) {
	w = io.Writer(os.Stdout)
	wc = func() {}
	logFile, err := build.buildArtifactPath(build.Config.LogFile)
	if err != nil {
		return nil, nil, err
	}
	if logFile != "" {
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
	if _, err := build.buildArtifactPath(build.Config.DataFile); err != nil {
		return err
	}
	if _, err := build.buildArtifactPath(build.Config.LogFile); err != nil {
		return err
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
	fmt.Fprintf(w, "copying \"%s\" to \"%s\"\n", build.Website.RelativePath(srcPath), build.Website.RelativePath(dstPath))
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
	fmt.Fprintf(w, "writing %d bytes to %s\n", len(fileBytes), build.Website.RelativePath(fileName))
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
	if publicPath == "" {
		return filepath.Join(build.URI.Path, "index.html")
	}
	if strings.HasSuffix(publicPath, "/") {
		return filepath.Join(build.URI.Path, publicPath, "index.html")
	}
	if filepath.Ext(publicPath) == ".html" {
		return filepath.Join(build.URI.Path, publicPath)
	}
	return filepath.Join(build.URI.Path, publicPath, "index.html")
}

func (build *Build) BuildStats(duration time.Duration, w io.Writer) {
	if w == nil {
		w = os.Stdout
	}

	build.Status.DurationMS = int(duration.Milliseconds())
	feedCount := 0
	for _, p := range *build.Pages {
		if p.Feed != nil {
			feedCount += 1
		}
	}
	fmt.Fprintf(w,
		"\nPages:  %8v (%s)\nAssets: %8v (%s)\nFeeds:  %8v (%s)\nTags:   %8v\n----------------\nTOTAL %8vms\n",
		len(*build.Pages), byteSize(build.Status.Bytes.Counters["pages"]),
		len(build.Status.Assets.Counters), byteSize(build.Status.Bytes.Counters["assets"]),
		feedCount, byteSize(build.Status.Bytes.Counters["feeds"]),
		len(build.Status.Tags.Counters),
		build.Status.DurationMS,
	)
}

func (build *Build) Data() BuildResult {
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
	return result
}

func (build *Build) WriteData(fileName string, w io.Writer) error {
	if w == nil {
		w = os.Stdout
	}

	fileBytes, err := json.MarshalIndent(build.Data(), "", "\t")
	if err != nil {
		return err
	}
	fileBytes = append(fileBytes, '\n')
	if err := os.MkdirAll(filepath.Dir(fileName), 0755); err != nil {
		return err
	}
	return build.WriteFile(fileName, fileBytes, w)
}

func (build *Build) writeDataArtifact(w io.Writer) error {
	dataFile, err := build.buildArtifactPath(build.Config.DataFile)
	if err != nil {
		return err
	}
	if dataFile == "" {
		return nil
	}
	return build.WriteData(dataFile, w)
}

func (build *Build) ArtifactPath(configured string) (string, error) {
	return build.buildArtifactPath(configured)
}

func (build *Build) buildArtifactPath(configured string) (string, error) {
	if configured == "" {
		return "", nil
	}
	if build.URI == nil || build.URI.Path == "" {
		return "", fmt.Errorf("build.uri is required")
	}
	if filepath.IsAbs(configured) {
		return "", fmt.Errorf("build artifact path must be relative: %s", configured)
	}
	if strings.HasSuffix(configured, "/") || strings.HasSuffix(configured, string(filepath.Separator)) {
		return "", fmt.Errorf("build artifact path must be a file: %s", configured)
	}

	rel, err := hypertemplates.CleanRelativeFilePath(configured)
	if err != nil {
		return "", err
	}
	if rel == "." {
		return "", fmt.Errorf("build artifact path must be a file: %s", configured)
	}
	return filepath.Join(build.URI.Path, rel), nil
}

func (build *Build) writeDeploymentIgnore() error {
	paths, err := build.deploymentIgnoreArtifactPaths()
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		return nil
	}
	if build.URI == nil || build.URI.Path == "" {
		return fmt.Errorf("build.uri is required")
	}

	root := build.URI.Path
	ignorePath := filepath.Join(root, ".deploymentignore")
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
		return err
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
		return nil
	}
	if err := os.MkdirAll(root, 0755); err != nil {
		return err
	}
	return os.WriteFile(ignorePath, []byte(strings.Join(lines, "\n")+"\n"), 0644)
}

func (build *Build) deploymentIgnoreArtifactPaths() ([]string, error) {
	var paths []string
	for _, configured := range []string{build.Config.DataFile, build.Config.LogFile} {
		if configured == "" {
			continue
		}
		if _, err := build.buildArtifactPath(configured); err != nil {
			return nil, err
		}
		rel, err := hypertemplates.CleanRelativeFilePath(configured)
		if err != nil {
			return nil, err
		}
		paths = append(paths, filepath.ToSlash(rel))
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

func NewTemplate(website *Website, page *Page) (template *hypertemplates.Template, err error) {
	var layout *html.Node
	if layout, err = website.Theme.LayoutHTML(page.LayoutRef); err != nil {
		return nil, err
	}
	return &hypertemplates.Template{
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

	var template *hypertemplates.Template
	if err = page.Validate(); err != nil {
		return nil, err
	}
	if page.Static {
		return os.ReadFile(page.URI.Path) // return static HTML file bytes without processing template
	}
	if template, err = NewTemplate(website, page); err != nil {
		return nil, err
	}
	template.Plugins = build.Plugins
	if pageBytes, err = template.Process(TemplateData(build, website, page), &hypertemplates.TemplateData{"page": page.TemplateData}, w); err != nil {
		return nil, err
	}
	// page.Output = gohtml.Format(page.Output) // TODO: restore HTML formatting 🤌🏽
	return pageBytes, nil
}
