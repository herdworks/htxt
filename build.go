package htxt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/net/html"
	"golang.org/x/sync/errgroup"

	"github.com/herdworks/httt"
	"github.com/herdworks/httt/directives"
	"github.com/herdworks/httt/document"
	htfiles "github.com/herdworks/httt/files"
	htvariable "github.com/herdworks/httt/variable"
	"github.com/herdworks/htup"
	"github.com/herdworks/htxt/href"
	"github.com/herdworks/hyperfeed/generator"
)

type Build struct {
	// declarative config
	Config BuildConfig

	// state
	Environment Environment
	Website     *Website
	Plugins     htvariable.Plugins
	Page        *Page
	Pages       *Pages
	Feeds       Feeds
	Status      BuildStatus
}

type BuildConfig struct {
	WebsiteConfig `json:",inline" yaml:",inline"`

	URI         *url.URL `json:"-" yaml:"-"`
	ID          string   `json:"id,omitempty" yaml:"id,omitempty"`
	Incremental bool     `json:"incremental,omitempty" yaml:"incremental,omitempty"`
	Page        string   `json:"page,omitempty" yaml:"page,omitempty"`
	Preview     bool     `json:"preview,omitempty" yaml:"preview,omitempty"`
	BaseURL     string   `json:"base_url,omitempty" yaml:"base_url,omitempty"`
	DataFile    string   `json:"data_file,omitempty" yaml:"data_file,omitempty"`
	LogFile     string   `json:"log_file,omitempty" yaml:"log_file,omitempty"`
	Verbose     bool     `json:"verbose,omitempty" yaml:"verbose,omitempty"`

	// DraftsDirectory is an optional website-root-relative source directory
	// layered over content_dir for preview builds. It is deliberately not a
	// WebsiteConfig setting: draft ownership belongs to the calling app.
	DraftsDirectory string `json:"drafts_dir,omitempty" yaml:"drafts_dir,omitempty"`
}

type BuildStatus struct {
	Pages      Counter `json:"-" yaml:"-"`
	Feeds      Counter `json:"-" yaml:"-"`
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

// GeneratedBuildAsset is a build-owned file written after regular assets.
type GeneratedBuildAsset struct {
	Path  string
	Bytes []byte
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
	if build == nil {
		return fmt.Errorf("build is required")
	}
	if build.Config.ID == "" {
		build.Config.ID = uuid.NewString()
	}
	if build.Config.URI, err = build.ResolveURI(); err != nil {
		return err
	}

	// load an io.Writer for stdout & optional file logging
	w, wc, err := build.logger()
	if err != nil {
		return err
	}
	defer wc() // close the log writer
	defer func() {
		// ensure post-logger errors are written to w before returning
		if err != nil {
			_, _ = fmt.Fprintf(w, "build %q failed: %v\n", build.Config.ID, err)
		}
	}()
	fmt.Fprintf(w, "build %q starting\n", build.Config.ID)

	build.Website = &Website{URI: &href.Href{URL: url.URL{Path: build.Config.WebsiteConfig.Path}}}
	if err = build.Website.ResolveURI(); err != nil {
		return err
	}

	// measure build execution time from here
	start := time.Now()

	// load the build context
	if err = build.LoadContext(ctx, w); err != nil {
		return err
	}

	// load plugins
	build.Plugins = htvariable.MergePlugins(build.Website.Theme.Plugins, build.Website.Plugins)

	// validate build
	if err = build.Validate(); err != nil {
		return err
	}
	if err = contextCancellationCheckpoint(ctx); err != nil {
		return err
	}

	// perform build
	writegroup, writeContext := errgroup.WithContext(ctx)
	writegroup.Go(func() error { return build.WriteAssets(writeContext, w) })
	writegroup.Go(func() error { return build.WriteFeeds(writeContext, w) })
	writegroup.Go(func() error { return build.WritePages(writeContext, w) })
	if err = writegroup.Wait(); err != nil {
		return err
	}
	if err = contextCancellationCheckpoint(ctx); err != nil {
		return err
	}
	if err = build.writeGeneratedAssets(ctx, w); err != nil {
		return err
	}

	// finalize build
	build.Status.DurationMS = int(time.Since(start).Milliseconds())
	result := build.Result()
	build.writeBuildData(result, w)
	build.writeDeploymentInstructions(w)
	fmt.Fprintf(w, "build %q completed\n", build.Config.ID)
	build.logResult(result, w)
	return nil
}

// ResolveURI does the minimum possible work to resolve a build URI for the
// build logger.
func (build *Build) ResolveURI() (*url.URL, error) {
	if build == nil {
		return nil, fmt.Errorf("build is required")
	}

	// early return the configured build.Config.URI if already set
	if build.Config.URI != nil && build.Config.URI.Path != "" {
		return build.Config.URI, nil
	}

	// resolve build URI from the provided WebsiteConfig.Path
	var website = &Website{URI: &href.Href{URL: url.URL{Path: build.Config.WebsiteConfig.Path}}}
	if err := website.ResolveURI(); err != nil {
		return nil, err
	}
	if err := website.LoadConfig(&build.Config.WebsiteConfig); err != nil {
		return nil, err
	}
	if build.Config.ID == "" {
		return nil, fmt.Errorf("a build ID is required to auto-generate a build URI")
	}
	return &url.URL{Path: filepath.Join(website.BuildsDirPath(), build.Config.ID)}, nil
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
		// The configured artifact is the durable log sink. Every caller can
		// request it, while GUI hosts may not provide stdout. io.MultiWriter
		// stops at its first failed write, so the file must be first.
		w = io.MultiWriter(file, w)
	}
	return &synchronizedWriter{w: w}, wc, nil
}

// LoadContext loads the complete build context, including:
//
//   - `site.*`:  resolved site config, discovered assets, etc
//   - `env.*`:   configured HT_* environment variables
//   - `data.*`:  static namespaced data and computed-data results
//   - `theme.*`: theme data files
//   - the complete private page and feed catalogs, including generated tag pages
//
// `ht.*` and `build.*` are supplied dynamically when template context is read.
func (build *Build) LoadContext(ctx context.Context, w io.Writer) (err error) {
	if w == nil {
		w = os.Stdout
	}
	build.Reset()
	if err := build.validateBuildScope(); err != nil {
		return err
	}
	if build.Website == nil {
		return fmt.Errorf("build.website is required")
	}
	if build.Website.TemplateData == nil {
		if err = build.Website.Load(build.Config.Verbose, &build.Config.WebsiteConfig, w); err != nil {
			return err
		}
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
	if err = build.Website.LoadPages(ctx, build.Config.Verbose, w, build.sourceDirectories()...); err != nil {
		return err
	}
	for _, page := range build.Website.pages {
		if slices.Contains(FeedVisible, page.Visibility) {
			build.Status.Tags.Increment(1, page.Tags...)
		}
	}

	// init site.pages template data
	// NOTE: the complete sitemap catalog must always be available, even during incremental builds
	sitemap := build.buildPages(build.Website.pages)
	build.Website.TemplateData.Set("pages", sitemap.TemplateData())

	// init build.Pages catalog
	if !build.Config.Incremental {
		pages := append(Pages(nil), build.Website.pages...)
		build.Pages = &pages
		build.Feeds = build.Website.feeds
	} else {
		if err = build.resolvePage(); err != nil {
			return err
		}
		page := build.Page
		if page == nil {
			return fmt.Errorf("build.config.page %q was not found in the website catalog", build.Config.Page)
		}
		selected := Pages{page}
		build.Feeds = build.Website.PageFeeds(selected...)
		selected = selected.Add(build.Website.FeedPages(build.Feeds)...)
		build.Pages = &selected
	}

	// hydrate build.Pages page.feeds FeedData
	for _, page := range *build.Pages {
		if err := page.HydrateFeeds(build.Website); err != nil {
			return err
		}
	}

	// Run computed-data scripts. Each script observes the full
	// static-data context (including site.pages) and installs its
	// result into Website.GlobalData; per-page renders read these
	// values back through TemplateData.
	return generateComputedData(ctx, build, build.Config.Verbose, w)
}

func (build *Build) resolvePage() error {
	root := filepath.Dir(build.Website.URI.Path)
	page := build.Website.pages.PageByURI(&url.URL{Path: filepath.Join(root, build.Config.Page)})
	if page == nil {
		return fmt.Errorf("build.config.page %q was not found in the website catalog", build.Config.Page)
	}
	build.Page = page
	return nil
}

func (build *Build) validateBuildScope() error {
	if build.Config.DraftsDirectory != "" {
		if !build.Config.Preview {
			return fmt.Errorf("build.config.drafts_dir is only supported for preview builds")
		}
		if _, err := htfiles.CleanRelativePath(build.Config.DraftsDirectory); err != nil {
			return fmt.Errorf("build.config.drafts_dir must be a clean website-root-relative path")
		}
	}
	if !build.Config.Incremental {
		if build.Config.Page != "" {
			return fmt.Errorf("build.config.page is only supported for incremental builds")
		}
		return nil
	}
	if build.Config.Page == "" {
		return fmt.Errorf("build.config.page is required for incremental builds")
	}
	if filepath.IsAbs(build.Config.Page) {
		return fmt.Errorf("build.config.page must be relative to the website root: %s", build.Config.Page)
	}
	if _, err := htfiles.CleanRelativePath(build.Config.Page); err != nil {
		return fmt.Errorf("build.config.page must be relative to the website root: %w", err)
	}
	return nil
}

func (build *Build) sourceDirectories() (directories []string) {
	directories = []string{build.Website.ContentDirPath()}
	if build.Config.DraftsDirectory != "" {
		directories = append(directories, filepath.Join(build.Website.RootDir(), build.Config.DraftsDirectory))
	}
	return directories
}

// Context returns the unified template context that page templates
// and computed-data scripts read during a build: ht, build, env, site,
// data, theme. Call LoadContext first to populate it; otherwise the
// entries are empty.
//
// build.date and ht.release_date resolve at call time, so each
// invocation captures a fresh timestamp.
func (build *Build) Context() httt.TemplateData {
	siteData := httt.TemplateData{}
	var globalData httt.TemplateData
	var themeData httt.TemplateData
	if build.Website != nil {
		if build.Website.TemplateData != nil {
			siteData = *build.Website.TemplateData
		}
		globalData = build.Website.GlobalData
		themeData = build.Website.Theme.TemplateData
	}
	return httt.TemplateData{
		"ht": httt.TemplateData{
			"version":      LibVersion,
			"release_date": LibReleaseDate.Format(time.RFC3339),
		},
		"build": httt.TemplateData{
			"id":   build.Config.ID,
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
	build.Pages = nil
	build.Feeds = nil
}

func (build *Build) buildPages(incoming Pages) (pages Pages) {
	var visibility []PageVisibilityKind = BuildVisible
	if build.Config.Preview {
		visibility = BuildPreviewVisible
	}
	for _, page := range incoming {
		if slices.Contains(visibility, page.Visibility) {
			pages = append(pages, page)
		}
	}
	return pages
}

func (build *Build) Validate() (err error) {
	if build.Config.URI == nil {
		return fmt.Errorf("build.uri is required")
	}
	if build.Config.URI.Path == "" {
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

	// Don't perform asset discovery if the context has already been cancelled.
	if err = contextCancellationCheckpoint(ctx); err != nil {
		return err
	}

	assetPages := build.buildPages(*build.Pages)
	assets, err := AssetCandidates(&build.Website.Theme, build.Website, assetPages)
	if err != nil {
		return err
	}
	assets = assets.Resolved()
	if build.Config.Incremental {
		assets = assets.FilterSource(PageAsset)
	}
	for _, asset := range assets {
		build.Status.Assets.Increment(1, build.AssetCounterKey(asset.RelPath))
	}
	taskgroup, taskContext := errgroup.WithContext(ctx)
	for _, asset := range assets {
		// Do not dispatch another asset after observing cancellation; join active workers first.
		if err = errgroupCancellationCheckpoint(taskContext, taskgroup); err != nil {
			return err
		}
		asset := asset
		taskgroup.Go(func() error {
			// Do not begin this copy if cancellation won the dispatch-to-worker race.
			if err := contextCancellationCheckpoint(taskContext); err != nil {
				return err
			}
			return build.copyAsset(asset, w)
		})
	}
	return taskgroup.Wait()
}

func (build *Build) CopyAsset(asset Asset, w io.Writer) error {
	if w == nil {
		w = os.Stdout
	}
	return build.copyAsset(asset, w)
}

func (build *Build) CopyWebsiteAsset(relPath string, w io.Writer) error {
	if w == nil {
		w = os.Stdout
	}

	srcPath, err := build.Website.ResourcePath(build.Website.StaticDirPath(), relPath)
	if err != nil {
		return err
	}
	return build.copyAsset(Asset{Source: WebsiteAsset, RelPath: filepath.ToSlash(relPath), SourcePath: srcPath}, w)
}

// WriteGeneratedAssets writes build-owned assets after all regular build work.
func (build *Build) WriteGeneratedAssets(w io.Writer) (err error) {
	return build.writeGeneratedAssets(context.Background(), w)
}

func (build *Build) writeGeneratedAssets(ctx context.Context, w io.Writer) (err error) {
	if w == nil {
		w = os.Stdout
	}
	assets, err := build.generatedBuildAssets(ctx)
	if err != nil {
		return err
	}
	for _, asset := range assets {
		var filePath string
		if filePath, err = build.AbsolutePath(asset.Path); err != nil {
			return err
		}
		if err = os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
			return err
		}
		if err = build.WriteFile(filePath, asset.Bytes, w); err != nil {
			return err
		}
		build.Status.Assets.Increment(1, asset.Path)
		build.Status.Bytes.Increment(len(asset.Bytes), "generated_assets")
	}
	return nil
}

func (build *Build) copyAsset(asset Asset, w io.Writer) (err error) {
	var srcFile io.ReadCloser
	var found bool
	var resourceBytes []byte
	var dstPath string
	var dstFile *os.File
	var dstFileBytesWritten int64
	if dstPath, err = build.AssetPath(asset); err != nil {
		return err
	}
	dstRelPath, err := build.RelativePath(dstPath)
	if err != nil {
		return err
	}
	sourcePath := asset.SourcePath
	switch asset.Source {
	case BuiltinAsset:
		hydrateThemeBuiltins(&build.Website.Theme)
		resourceBytes, found, err = build.Website.Theme.BuiltinResourceBytes(filepath.ToSlash(filepath.Join("static", asset.RelPath)))
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("builtin asset %q not found", asset.SourcePath)
		}
		srcFile = io.NopCloser(bytes.NewReader(resourceBytes))
	default:
		sourcePath = build.Website.RelativePath(sourcePath)
		if srcFile, err = os.Open(asset.SourcePath); err != nil {
			return err
		}
	}
	fmt.Fprintf(w, "copying \"%s\" to \"%s\"\n", sourcePath, filepath.ToSlash(filepath.Join(build.Config.ID, dstRelPath)))
	if err = os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		return err
	}
	defer srcFile.Close()
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

func (build *Build) generatedBuildAssets(ctx context.Context) (assets []GeneratedBuildAsset, err error) {
	assets = []GeneratedBuildAsset{{Path: ".well-known/hypertexting/build", Bytes: []byte(build.Config.ID + "\n")}}
	if data, err := build.Website.contactVCard(ctx); err != nil {
		return nil, err
	} else if len(data) != 0 {
		assets = append(assets, GeneratedBuildAsset{Path: contactAssetPath, Bytes: data})
	}
	return assets, nil
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
	// Stop before feed rendering when this phase has already been cancelled.
	if err = contextCancellationCheckpoint(ctx); err != nil {
		return err
	}

	taskgroup, taskContext := errgroup.WithContext(ctx)
	for _, catalog := range build.Feeds {
		for _, feed := range catalog {
			// Do not dispatch another feed after observing cancellation; join active workers first.
			if err = errgroupCancellationCheckpoint(taskContext, taskgroup); err != nil {
				return err
			}
			feed := feed
			taskgroup.Go(func() error {
				// Do not begin this feed render if cancellation won the dispatch-to-worker race.
				if err := contextCancellationCheckpoint(taskContext); err != nil {
					return err
				}
				return build.WriteFeed(feed, w)
			})
		}
	}
	return taskgroup.Wait()
}

func (build *Build) WriteFeed(feed Feed, w io.Writer) (err error) {
	if w == nil {
		w = os.Stdout
	}
	feedPath, err := build.AbsolutePath(strings.TrimPrefix(feed.URL.Path, "/"))
	if err != nil {
		return err
	}
	model, err := feed.toHyperFeedFeed(build.Website)
	if err != nil {
		return err
	}
	// TODO: Switch on the configured feed format when RSS generation is available.
	feedBytes, err := generator.Atom(model)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(feedPath), 0755); err != nil {
		return err
	}
	if err = build.WriteFile(feedPath, feedBytes, w); err != nil {
		return err
	}
	build.Status.Bytes.Increment(len(feedBytes), "feeds")
	build.Status.Feeds.Increment(1, "written")
	return nil
}

func (build *Build) WritePages(ctx context.Context, w io.Writer) (err error) {
	if w == nil {
		w = os.Stdout
	}
	// Stop before page rendering when this phase has already been cancelled.
	if err = contextCancellationCheckpoint(ctx); err != nil {
		return err
	}

	taskgroup, taskContext := errgroup.WithContext(ctx)
	for i := range *build.Pages {
		// Do not dispatch another page after observing cancellation; join active workers first.
		if err = errgroupCancellationCheckpoint(taskContext, taskgroup); err != nil {
			return err
		}
		taskgroup.Go(func() error {
			// Do not begin this page render if cancellation won the dispatch-to-worker race.
			if err := contextCancellationCheckpoint(taskContext); err != nil {
				return err
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

	var visibility []PageVisibilityKind = BuildVisible
	if build.Config.Preview {
		visibility = BuildPreviewVisible
	}
	if !slices.Contains(visibility, page.Visibility) {
		return nil
	}

	var filePath string = build.BuildPath(page)
	var fileBytes []byte
	if fileBytes, err = RenderPage(build, build.Website, page, w); err != nil {
		return fmt.Errorf("failed to generate %q with error: %w", build.Website.RelativePath(page.Source.URI().Path), err)
	}
	if err = os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return err
	}
	if err = build.WriteFile(filePath, fileBytes, w); err != nil {
		return err
	}
	build.Status.Bytes.Increment(len(fileBytes), "pages")
	build.Status.Pages.Increment(1, "written")
	return nil
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
	fmt.Fprintf(w, "writing %d bytes to %s\n", len(fileBytes), filepath.ToSlash(filepath.Join(build.Config.ID, fileRelPath)))
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
	publicPath := page.PublicURL.Path
	if publicPath == "" {
		publicPath = page.Source.PublicURLPath(page.UglyURL)
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
		if build != nil && build.Config.URI != nil {
			return filepath.Join(build.Config.URI.Path, relPath)
		}
		return relPath
	}
	return filePath
}

// AssetPath returns the build output path for asset.
func (build *Build) AssetPath(asset Asset) (string, error) {
	return build.AbsolutePath(asset.RelPath)
}

func (build *Build) AbsolutePath(relPath string) (string, error) {
	if build == nil || build.Config.URI == nil || build.Config.URI.Path == "" {
		return "", fmt.Errorf("build.uri is required")
	}
	if filepath.IsAbs(relPath) {
		return "", fmt.Errorf("build path must be relative: %s", relPath)
	}
	relPath, err := htfiles.CleanRelativePath(relPath)
	if err != nil {
		return "", err
	}
	root, err := filepath.Abs(build.Config.URI.Path)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, relPath), nil
}

func (build *Build) RelativePath(path string) (string, error) {
	if build == nil || build.Config.URI == nil || build.Config.URI.Path == "" {
		return "", fmt.Errorf("build.uri is required")
	}
	root, err := filepath.Abs(build.Config.URI.Path)
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
		ID:         build.Config.ID,
		Assets:     build.Status.Assets.Map(),
		Tags:       build.Status.Tags.Map(),
		Bytes:      build.Status.Bytes.Map(),
		DurationMS: build.Status.DurationMS,
	}
	if build.Config.URI != nil {
		result.URI = build.Config.URI.String()
	}
	result.PageCount = build.Status.Pages.Map()["written"]
	result.FeedCount = build.Status.Feeds.Map()["written"]
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

func (build *Build) writeDeploymentInstructions(w io.Writer) {
	if w == nil {
		w = os.Stdout
	}
	paths, err := build.deploymentIgnoreArtifactPaths()
	if err != nil {
		fmt.Fprintf(w, "deployment instructions skipped: %v\n", err)
		return
	}
	if len(paths) == 0 {
		return
	}
	if build.Config.URI == nil || build.Config.URI.Path == "" {
		fmt.Fprintf(w, "deployment instructions skipped: build.uri is required\n")
		return
	}

	instructionsPath, err := build.AbsolutePath(htup.DeploymentInstructionsFile)
	if err != nil {
		fmt.Fprintf(w, "deployment instructions skipped: %v\n", err)
		return
	}
	root := filepath.Dir(instructionsPath)
	instructions, err := htup.LoadDeploymentInstructions(root)
	if err != nil {
		fmt.Fprintf(w, "deployment instructions skipped: %v\n", err)
		return
	}
	seen := map[string]bool{}
	normalizedIgnore := make([]string, 0, len(instructions.Ignore)+len(paths))
	for _, configured := range instructions.Ignore {
		entry := normalizeDeploymentInstructionIgnoreEntry(configured)
		if entry == "" || seen[entry] {
			continue
		}
		seen[entry] = true
		normalizedIgnore = append(normalizedIgnore, entry)
	}
	instructions.Ignore = normalizedIgnore

	changed := false
	for _, path := range paths {
		entry := normalizeDeploymentInstructionIgnoreEntry(path)
		if entry == "" || seen[entry] {
			continue
		}
		seen[entry] = true
		instructions.Ignore = append(instructions.Ignore, entry)
		changed = true
	}
	if !changed {
		return
	}
	if err := os.MkdirAll(root, 0755); err != nil {
		fmt.Fprintf(w, "deployment instructions skipped: %v\n", err)
		return
	}
	data, err := json.Marshal(instructions)
	if err != nil {
		fmt.Fprintf(w, "deployment instructions skipped: %v\n", err)
		return
	}
	if err := build.WriteFile(instructionsPath, data, w); err != nil {
		fmt.Fprintf(w, "deployment instructions skipped: %v\n", err)
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
	status.Pages.Reset()
	status.Feeds.Reset()
	status.Assets.Reset()
	status.Tags.Reset()
	status.Bytes.Reset()
	status.DurationMS = 0
}

func normalizeDeploymentInstructionIgnoreEntry(entry string) string {
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

// contextCancellationCheckpoint returns ctx's cancellation error, if any.
//
// we don't normally like thin wrappers, but this is an exception to the rule;
// checking <some context>.Err() multiple times in a process is less legible
// than clearly demarcated "context cancellation checkpoints", thus this helper.
func contextCancellationCheckpoint(ctx context.Context) error {
	return ctx.Err()
}

// errgroupCancellationCheckpoint stops dispatch when ctx is cancelled and joins any
// work already started by taskgroup before returning.
func errgroupCancellationCheckpoint(ctx context.Context, taskgroup *errgroup.Group) error {
	if err := ctx.Err(); err != nil {
		if groupErr := taskgroup.Wait(); groupErr != nil {
			return groupErr
		}
		return err
	}
	return nil
}

// NamespacedTemplateData loads the website's static data files, deriving
// each namespace from the file's path under the data directory. Files in
// subdirectories nest into the resulting TemplateData by their path
// segments (data/social/twitter.yaml -> data.social.twitter.*).
func NamespacedTemplateData(website *Website, config *httt.HyperMarkConfig, verbose bool, w io.Writer) (httt.TemplateData, error) {
	if w == nil {
		w = os.Stdout
	}
	return loadStaticData(website, website.DataDirPath(), config, verbose, w)
}

// ThemeTemplateData loads the theme's static data files using the same
// filename-derived namespace rules as NamespacedTemplateData.
func ThemeTemplateData(website *Website, config *httt.HyperMarkConfig, verbose bool, w io.Writer) (httt.TemplateData, error) {
	if w == nil {
		w = os.Stdout
	}
	return loadStaticData(website, website.Theme.DataDirPath(), config, verbose, w)
}

func NewTemplateDocument(website *Website, page *Page) (doc *document.Document, err error) {
	hydrateThemeBuiltins(&website.Theme)
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

func TemplateData(build *Build, website *Website, page *Page) (data *httt.TemplateData) {
	data = &httt.TemplateData{}
	data.Set("ht", httt.TemplateData{
		"version":      LibVersion,
		"release_date": LibReleaseDate.Format(time.RFC3339),
	})
	data.Set("build", httt.TemplateData{
		"id":   build.Config.ID,
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
		return os.ReadFile(page.Source.URI().Path) // return static HTML file bytes without processing template
	}
	if doc, err = NewTemplateDocument(website, page); err != nil {
		return nil, err
	}
	doc.Plugins = build.Plugins
	if pageBytes, err = directives.Process(doc, TemplateData(build, website, page), &httt.TemplateData{"page": page.TemplateData}, w); err != nil {
		return nil, err
	}
	// page.Output = gohtml.Format(page.Output) // TODO: restore HTML formatting 🤌🏽
	return pageBytes, nil
}
