package htxt

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/herdworks/httt"
	"github.com/herdworks/htxt/href"
)

type buildSelectionFixture struct {
	root     string
	sitePath string
	pages    []string
}

func newBuildSelectionFixture(t *testing.T) buildSelectionFixture {
	t.Helper()
	root := t.TempDir()
	writeBuildSelectionFile(t, filepath.Join(root, "site.yaml"), `base_url: https://example.com/
title: Test Site
description: Test description
byline:
  name: Test Author
config:
  theme: theme.json
`)
	writeBuildSelectionFile(t, filepath.Join(root, "theme.json"), "{}")
	pages := []string{
		filepath.Join(root, "content", "one", "index.md"),
		filepath.Join(root, "content", "two", "index.md"),
	}
	writeBuildSelectionFile(t, pages[0], "---\nlayout: default\ntitle: One\n---\n\none")
	writeBuildSelectionFile(t, pages[1], "---\nlayout: default\ntitle: Two\n---\n\ntwo")
	return buildSelectionFixture{root: root, sitePath: filepath.Join(root, "site.yaml"), pages: pages}
}

type buildExecutionFixture struct {
	root      string
	sitePath  string
	buildPath string
}

func newBuildExecutionFixture(t *testing.T) buildExecutionFixture {
	t.Helper()
	root := t.TempDir()
	writeBuildSelectionFile(t, filepath.Join(root, "site.yaml"), `base_url: https://example.com/
title: Test Site
description: Test description
byline:
  name: Test Author
config:
  theme: theme.json
`)
	writeBuildSelectionFile(t, filepath.Join(root, "theme.json"), "{}")
	writeBuildSelectionFile(t, filepath.Join(root, "content", "index.html"), "<h1>Home</h1>")
	writeBuildSelectionFile(t, filepath.Join(root, "custom-content", "index.html"), "<h1>Custom</h1>")
	return buildExecutionFixture{
		root:      root,
		sitePath:  filepath.Join(root, "site.yaml"),
		buildPath: filepath.Join(root, "builds", "test"),
	}
}

func TestBuildWritePhasesDoNotStartAfterCancellation(t *testing.T) {
	fixture := newBuildExecutionFixture(t)
	build := &Build{
		Config: BuildConfig{
			WebsiteConfig: WebsiteConfig{Path: fixture.sitePath},
			URI:           &url.URL{Path: fixture.buildPath},
			ID:            "test",
		},
		Website: &Website{URI: &href.Href{URL: url.URL{Path: fixture.sitePath}}},
	}
	if err := build.Website.ResolveURI(); err != nil {
		t.Fatalf("ResolveURI() error = %v", err)
	}
	if err := build.LoadContext(context.Background(), io.Discard); err != nil {
		t.Fatalf("LoadContext() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for name, write := range map[string]func(context.Context, io.Writer) error{
		"assets": build.WriteAssets,
		"feeds":  build.WriteFeeds,
		"pages":  build.WritePages,
	} {
		if err := write(ctx, io.Discard); err != context.Canceled {
			t.Errorf("Write%s() error = %v, want context.Canceled", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(fixture.buildPath, "index.html")); !os.IsNotExist(err) {
		t.Errorf("cancelled write created page output: %v", err)
	}
}

func writeBuildSelectionFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

func TestBootstrapPackageBuildsWithCurrentDirectives(t *testing.T) {
	root := t.TempDir()
	website := &Website{
		URI:         &href.Href{URL: url.URL{Path: filepath.Join(root, "site.yaml")}},
		BaseURL:     "https://example.com/",
		Title:       "Bootstrap Site",
		Description: "Bootstrap description",
		Byline:      Byline{Name: "Bootstrap Author"},
	}
	if err := website.CreatePackage(io.Discard); err != nil {
		t.Fatalf("CreatePackage() error = %v", err)
	}
	if got, want := website.Config.ThemePath, "themes/default/theme.json"; got != want {
		t.Fatalf("created website theme = %q, want %q", got, want)
	}
	if got, want := website.Theme.URI.Path, filepath.Join(root, "themes", "default", "theme.json"); got != want {
		t.Fatalf("created website theme URI = %q, want %q", got, want)
	}
	if _, err := os.Stat(website.Theme.URI.Path); err != nil {
		t.Fatalf("created website theme file: %v", err)
	}

	build := &Build{
		Config: BuildConfig{
			WebsiteConfig: WebsiteConfig{Path: website.URI.Path},
			URI:           &url.URL{Path: filepath.Join(root, "builds", "test")},
			ID:            "test",
		},
	}
	if err := build.Execute(context.Background()); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output, err := os.ReadFile(filepath.Join(build.Config.URI.Path, "index.html"))
	if err != nil {
		t.Fatalf("ReadFile(index.html) error = %v", err)
	}
	for _, stale := range []string{"ht-template", "ht-content", "ht-param", "ht-attr=", "ht-not"} {
		if bytes.Contains(output, []byte(stale)) {
			t.Errorf("rendered bootstrap output contains stale directive %q:\n%s", stale, output)
		}
	}
	feed, err := os.ReadFile(filepath.Join(build.Config.URI.Path, "feeds", "blog.atom.xml"))
	if err != nil {
		t.Fatalf("ReadFile(blog Atom feed) error = %v", err)
	}
	for _, title := range []string{"Hello world", "Hello again"} {
		if !bytes.Contains(feed, []byte("<title>"+title+"</title>")) {
			t.Errorf("blog Atom feed is missing %q:\n%s", title, feed)
		}
	}
}

func (fixture buildSelectionFixture) build(page *Page, incremental bool) *Build {
	pagePath := ""
	if page != nil {
		pagePath, _ = filepath.Rel(fixture.root, page.URI.Path)
	}
	return &Build{
		Config: BuildConfig{
			WebsiteConfig: WebsiteConfig{Path: fixture.sitePath},
			URI:           &url.URL{Path: filepath.Join(fixture.root, "builds", "test")},
			ID:            "test",
			Incremental:   incremental,
			Page:          pagePath,
		},
		Website: &Website{URI: &href.Href{URL: url.URL{Path: fixture.sitePath}}},
	}
}

func (fixture buildExecutionFixture) build() *Build {
	return &Build{
		Config: BuildConfig{
			WebsiteConfig: WebsiteConfig{Path: fixture.sitePath},
			URI:           &url.URL{Path: fixture.buildPath},
			ID:            "test",
		},
		Website: &Website{URI: &href.Href{URL: url.URL{Path: fixture.sitePath}}},
	}
}

func TestBuildLoadContextFullBuildUsesDiscoveredPages(t *testing.T) {
	fixture := newBuildSelectionFixture(t)
	build := fixture.build(nil, false)
	build.Pages = &Pages{{URI: &url.URL{Path: "/stale-output/"}}}
	if err := build.LoadContext(context.Background(), io.Discard); err != nil {
		t.Fatalf("LoadContext() error = %v", err)
	}
	if build.Pages == nil {
		t.Fatal("LoadContext() left build.Pages nil")
	}
	if got, want := len(*build.Pages), len(fixture.pages); got != want {
		t.Fatalf("len(build.Pages) = %d, want %d", got, want)
	}
}

func TestBuildGeneratedTagPageUsesTagFeedAlias(t *testing.T) {
	fixture := newBuildSelectionFixture(t)
	writeBuildSelectionFile(t, fixture.pages[0], "---\nlayout: default\ntags:\n  - news\ntitle: One\n---\n\none")
	build := fixture.build(nil, false)
	if err := build.LoadContext(context.Background(), io.Discard); err != nil {
		t.Fatalf("LoadContext() error = %v", err)
	}
	page := build.Pages.PageByPath("/tags/news/")
	if page == nil {
		t.Fatal("generated tag page is missing")
	}
	data, ok := page.Feeds[TagFeed]["tag"]
	if !ok || data.Tag != "news" {
		t.Fatalf("generated tag feed = %#v, want alias tag for news", page.Feeds[TagFeed])
	}
	if _, exists := page.Feeds[TagFeed]["news"]; exists {
		t.Fatalf("generated tag feed retained tag-specific alias: %#v", page.Feeds[TagFeed])
	}
}

func TestBuildLoadsTagPagesIntoWebsiteCatalogBeforeSelection(t *testing.T) {
	fixture := newBuildSelectionFixture(t)
	writeBuildSelectionFile(t, fixture.pages[0], "---\nlayout: default\ntags:\n  - news\ntitle: One\n---\n\none")
	build := fixture.build(nil, false)
	if err := build.LoadContext(context.Background(), io.Discard); err != nil {
		t.Fatalf("LoadContext() error = %v", err)
	}
	if got, want := len(build.Website.pages), 3; got != want {
		t.Fatalf("len(website.pages) = %d, want %d", got, want)
	}
	if _, ok := build.Feeds.Feed(TagFeed, "news"); !ok {
		t.Fatal("Build.Feeds is missing the complete tag catalog")
	}
	if feed, _ := build.Feeds.Feed(TagFeed, "news"); len(feed.Pages) != 1 || feed.Pages[0].Path != "/one/" {
		t.Fatalf("tag feed pages = %#v, want authored source page only", feed.Pages)
	}
	if page := build.Website.pages.PageByPath("/tags/news/"); page == nil || page.TemplateData.Get("feeds.tag.items") == nil {
		t.Fatalf("generated tag page = %#v, want hydrated page.feeds.tag", page)
	}
}

func TestIncrementalBuildSelectsGeneratedButNotAuthoredTagRenderers(t *testing.T) {
	fixture := newBuildSelectionFixture(t)
	writeBuildSelectionFile(t, fixture.pages[0], "---\nlayout: default\ntags:\n  - news\ntitle: One\n---\n\none")
	requested := &Page{URI: &url.URL{Path: fixture.pages[0]}}
	build := fixture.build(requested, true)
	if err := build.LoadContext(context.Background(), io.Discard); err != nil {
		t.Fatalf("generated LoadContext() error = %v", err)
	}
	if page := build.Pages.PageByPath("/tags/news/"); page == nil {
		t.Fatal("incremental build did not select generated tag renderer")
	}

	writeBuildSelectionFile(t, filepath.Join(fixture.root, "content", "tags", "news", "index.md"), "---\nlayout: default\ntitle: Authored tag page\n---\n\nauthored")
	requested = &Page{URI: &url.URL{Path: fixture.pages[0]}}
	build = fixture.build(requested, true)
	if err := build.LoadContext(context.Background(), io.Discard); err != nil {
		t.Fatalf("authored LoadContext() error = %v", err)
	}
	if page := build.Pages.PageByPath("/tags/news/"); page != nil {
		t.Fatalf("incremental build selected authored tag page without binding: %#v", page)
	}
}

func TestBuildLoadContextFullBuildRejectsPage(t *testing.T) {
	fixture := newBuildSelectionFixture(t)
	build := fixture.build(&Page{URI: &url.URL{Path: fixture.pages[0]}}, false)

	err := build.LoadContext(context.Background(), io.Discard)
	if err == nil {
		t.Fatal("LoadContext() error = nil")
	}
	if !strings.Contains(err.Error(), "build.config.page is only supported for incremental builds") {
		t.Fatalf("LoadContext() error = %v, want build.config.page incremental error", err)
	}
}

func TestBuildLoadContextRejectsInvalidPageVisibility(t *testing.T) {
	fixture := newBuildSelectionFixture(t)
	writeBuildSelectionFile(t, fixture.pages[0], "---\nvisibility: private\n---\n\ninvalid")
	build := fixture.build(nil, false)
	if err := build.LoadContext(context.Background(), io.Discard); err == nil {
		t.Fatal("LoadContext() accepted invalid page visibility")
	}
}

func TestBuildLoadContextIncrementalBuildRequiresPage(t *testing.T) {
	fixture := newBuildSelectionFixture(t)
	build := fixture.build(nil, true)
	err := build.LoadContext(context.Background(), io.Discard)
	if err == nil {
		t.Fatal("LoadContext() error = nil")
	}
	if !strings.Contains(err.Error(), "build.config.page is required for incremental builds") {
		t.Fatalf("LoadContext() error = %v, want required page error", err)
	}
}

func TestBuildLoadContextIncrementalBuildSelectsRequestedPage(t *testing.T) {
	fixture := newBuildSelectionFixture(t)
	build := fixture.build(&Page{URI: &url.URL{Path: fixture.pages[0]}}, true)

	if err := build.LoadContext(context.Background(), io.Discard); err != nil {
		t.Fatalf("LoadContext() error = %v", err)
	}
	if got, want := len(*build.Pages), 1; got != want {
		t.Fatalf("len(build.Pages) = %d, want %d", got, want)
	}
	if got, want := (*build.Pages)[0].URI.Path, fixture.pages[0]; got != want {
		t.Fatalf("build.Pages[0].URI.Path = %q, want %q", got, want)
	}
}

func TestBuildLoadContextIncrementalBuildValidatesAllPageFeeds(t *testing.T) {
	fixture := newBuildSelectionFixture(t)
	for name, source := range map[string]string{
		"unknown source": "feeds:\n  latest:\n    source: missing\n",
		"negative limit": "feeds:\n  latest:\n    source: latest\n    limit: -1\n",
	} {
		t.Run(name, func(t *testing.T) {
			writeBuildSelectionFile(t, fixture.pages[1], "---\n"+source+"---\n\nunselected")
			build := fixture.build(&Page{URI: &url.URL{Path: fixture.pages[0]}}, true)
			if err := build.LoadContext(context.Background(), io.Discard); err == nil {
				t.Fatal("LoadContext() accepted invalid feeds on an unselected page")
			}
		})
	}
}

func TestBuildLoadContextCanRebuildFullSelection(t *testing.T) {
	fixture := newBuildSelectionFixture(t)
	build := fixture.build(nil, false)
	if err := build.LoadContext(context.Background(), io.Discard); err != nil {
		t.Fatalf("first LoadContext() error = %v", err)
	}
	if build.Pages == nil || len(*build.Pages) == 0 {
		t.Fatal("first LoadContext() did not load pages")
	}
	if err := build.LoadContext(context.Background(), io.Discard); err != nil {
		t.Fatalf("second LoadContext() error = %v", err)
	}
	if got, want := len(*build.Pages), len(fixture.pages); got != want {
		t.Fatalf("len(build.Pages) after reload = %d, want %d", got, want)
	}
}

func TestBuildLoadContextCanRebuildIncrementalSelection(t *testing.T) {
	fixture := newBuildSelectionFixture(t)
	build := fixture.build(&Page{URI: &url.URL{Path: fixture.pages[0]}}, true)
	if err := build.LoadContext(context.Background(), io.Discard); err != nil {
		t.Fatalf("first LoadContext() error = %v", err)
	}
	build.Config.Page, _ = filepath.Rel(fixture.root, fixture.pages[1])
	if err := build.LoadContext(context.Background(), io.Discard); err != nil {
		t.Fatalf("second LoadContext() error = %v", err)
	}
	if got, want := (*build.Pages)[0].URI.Path, fixture.pages[1]; got != want {
		t.Fatalf("incremental root page after reload = %q, want %q", got, want)
	}
}

func TestBuildLoadContextUsesBuildConfig(t *testing.T) {
	fixture := newBuildExecutionFixture(t)
	build := fixture.build()
	build.Config = BuildConfig{
		WebsiteConfig: WebsiteConfig{Path: build.Website.URI.Path, ContentDirectory: "custom-content"},
		URI:           &url.URL{Path: fixture.buildPath},
		ID:            "test",
		BaseURL:       "https://preview.example.com/",
		Verbose:       true,
	}

	if err := build.LoadContext(context.Background(), io.Discard); err != nil {
		t.Fatalf("LoadContext() error = %v", err)
	}
	if got := build.Website.BaseURL; got != "https://preview.example.com/" {
		t.Fatalf("Website.BaseURL = %q, want preview override", got)
	}
	if build.Pages == nil || len(*build.Pages) != 1 {
		t.Fatalf("len(Build.Pages) = %v, want 1", build.Pages)
	}
	if got := (*build.Pages)[0].URI.Path; !strings.Contains(got, "custom-content") {
		t.Fatalf("page URI = %q, want custom content directory", got)
	}
}

func TestBuildExecuteWritesConfiguredArtifacts(t *testing.T) {
	fixture := newBuildExecutionFixture(t)
	build := fixture.build()
	build.Config.DataFile = "build.json"
	build.Config.LogFile = "build.log"
	build.Config.Verbose = true

	if err := build.Execute(context.Background()); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	logBytes, err := os.ReadFile(filepath.Join(fixture.buildPath, "build.log"))
	if err != nil {
		t.Fatalf("ReadFile(build.log) error = %v", err)
	}
	if got := string(logBytes); !strings.Contains(got, `build "test" starting`) {
		t.Fatalf("build.log = %q, want build start log", got)
	}
	if got := string(logBytes); !strings.Contains(got, `website "`+fixture.sitePath+`" load`) {
		t.Fatalf("build.log = %q, want website load log", got)
	}

	dataBytes, err := os.ReadFile(filepath.Join(fixture.buildPath, "build.json"))
	if err != nil {
		t.Fatalf("ReadFile(build.json) error = %v", err)
	}
	var data BuildResult
	if err := json.Unmarshal(dataBytes, &data); err != nil {
		t.Fatalf("Unmarshal(build.json) error = %v", err)
	}
	var rawData map[string]any
	if err := json.Unmarshal(dataBytes, &rawData); err != nil {
		t.Fatalf("Unmarshal(build.json map) error = %v", err)
	}
	if _, ok := rawData["status"]; ok {
		t.Fatalf("build.json contains nested status: %s", dataBytes)
	}
	if _, ok := rawData["bytes"]; !ok {
		t.Fatalf("build.json missing top-level bytes: %s", dataBytes)
	}
	if data.ID != "test" {
		t.Fatalf("build data id = %q, want test", data.ID)
	}
	if data.PageCount != 1 {
		t.Fatalf("build data page_count = %d, want 1", data.PageCount)
	}
	if data.FeedCount != 0 {
		t.Fatalf("build data feed_count = %d, want 0", data.FeedCount)
	}
	if data.DurationMS < 0 {
		t.Fatalf("duration_ms = %d, want non-negative", data.DurationMS)
	}
	if data.Bytes["pages"] != len("<h1>Home</h1>") {
		t.Fatalf("bytes pages = %d, want %d", data.Bytes["pages"], len("<h1>Home</h1>"))
	}

	instructionsBytes, err := os.ReadFile(filepath.Join(fixture.buildPath, ".deployment.json"))
	if err != nil {
		t.Fatalf("ReadFile(.deployment.json) error = %v", err)
	}
	var instructions struct {
		Ignore []string `json:"ignore"`
	}
	if err := json.Unmarshal(instructionsBytes, &instructions); err != nil {
		t.Fatalf("Unmarshal(.deployment.json) error = %v", err)
	}
	for _, want := range []string{"build.json", "build.log"} {
		if !slices.Contains(instructions.Ignore, want) {
			t.Fatalf(".deployment.json ignore = %#v, want %q", instructions.Ignore, want)
		}
	}
}

func TestBuildExecuteInitializesDeclarativeConfig(t *testing.T) {
	fixture := newBuildExecutionFixture(t)
	build := &Build{
		Config: BuildConfig{
			WebsiteConfig: WebsiteConfig{Path: fixture.sitePath},
		},
	}

	if err := build.Execute(t.Context()); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if build.Config.ID == "" {
		t.Fatal("Execute() did not generate a build ID")
	}
	if build.Config.URI == nil {
		t.Fatal("Execute() did not generate a build URI")
	}
	wantRoot := filepath.Join(fixture.root, "builds", build.Config.ID)
	if build.Config.URI.Path != wantRoot {
		t.Fatalf("build URI = %q, want %q", build.Config.URI.Path, wantRoot)
	}
	if _, err := os.Stat(filepath.Join(wantRoot, "index.html")); err != nil {
		t.Fatalf("default output missing: %v", err)
	}
}

func TestBuildResolveURILeavesWebsiteUnset(t *testing.T) {
	fixture := newBuildExecutionFixture(t)
	build := &Build{Config: BuildConfig{
		WebsiteConfig: WebsiteConfig{Path: fixture.sitePath},
		ID:            "build-1",
	}}

	uri, err := build.ResolveURI()
	if err != nil {
		t.Fatalf("ResolveURI() error = %v", err)
	}
	if build.Website != nil {
		t.Fatalf("ResolveURI() website = %#v, want nil", build.Website)
	}
	if build.Config.URI != nil {
		t.Fatalf("ResolveURI() build URI = %#v, want nil", build.Config.URI)
	}
	want := filepath.Join(fixture.root, "builds", "build-1")
	if uri.Path != want {
		t.Fatalf("ResolveURI() URI = %q, want %q", uri.Path, want)
	}
}

func TestBuildExecuteResolvesWebsiteDirectory(t *testing.T) {
	fixture := newBuildExecutionFixture(t)
	build := &Build{
		Config: BuildConfig{
			WebsiteConfig: WebsiteConfig{Path: fixture.root},
			ID:            "directory",
		},
	}

	if err := build.Execute(t.Context()); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got, want := build.Website.URI.Path, fixture.sitePath; got != want {
		t.Fatalf("website URI = %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(fixture.root, "builds", "directory", "index.html")); err != nil {
		t.Fatalf("directory build output missing: %v", err)
	}
}

func TestBuildExecutePreservesExplicitOutputURI(t *testing.T) {
	fixture := newBuildExecutionFixture(t)
	output := filepath.Join(t.TempDir(), "external-build")
	build := &Build{
		Config: BuildConfig{
			WebsiteConfig: WebsiteConfig{Path: fixture.sitePath},
			ID:            "external",
			URI:           &url.URL{Path: output},
		},
	}

	if err := build.Execute(t.Context()); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if build.Config.URI.Path != output {
		t.Fatalf("build URI = %q, want explicit %q", build.Config.URI.Path, output)
	}
	if _, err := os.Stat(filepath.Join(output, "index.html")); err != nil {
		t.Fatalf("explicit output missing: %v", err)
	}
}

func TestBuildRejectsAbsolutePageExpression(t *testing.T) {
	fixture := newBuildExecutionFixture(t)
	build := &Build{
		Config: BuildConfig{
			WebsiteConfig: WebsiteConfig{Path: fixture.sitePath},
			Incremental:   true,
			Page:          fixture.sitePath,
			LogFile:       "build.log",
		},
	}

	err := build.Execute(t.Context())
	if err == nil || !strings.Contains(err.Error(), "must be relative to the website root") {
		t.Fatalf("Execute() error = %v, want relative page error", err)
	}
	logBytes, logErr := os.ReadFile(filepath.Join(build.Config.URI.Path, "build.log"))
	if logErr != nil {
		t.Fatalf("ReadFile(build.log) error = %v", logErr)
	}
	if got := string(logBytes); !strings.Contains(got, `build "`+build.Config.ID+`" failed: `+err.Error()) {
		t.Fatalf("build.log = %q, want logged build error", got)
	}
}

func TestBuildWritersEnforceVisibilityOnSelectedDependencies(t *testing.T) {
	root := t.TempDir()
	page := &Page{
		URI:          &url.URL{Path: filepath.Join(root, "content", "feed", "index.html")},
		Path:         "/feed/",
		Visibility:   NoVisibility,
		Static:       true,
		TemplateData: map[string]any{},
	}
	pages := Pages{page}
	build := &Build{
		Config:  BuildConfig{URI: &url.URL{Path: filepath.Join(root, "builds", "test")}, Incremental: true},
		Website: &Website{BaseURL: "https://example.com/"},
		Pages:   &pages,
	}
	if pages := build.buildPages(pages); len(pages) != 0 {
		t.Fatalf("buildPages() = %#v, want no hidden dependency", pages)
	}
	if err := build.WritePage(page, io.Discard); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(build.Config.URI.Path, "feed", "index.html")); !os.IsNotExist(err) {
		t.Fatalf("hidden page was written, stat error = %v", err)
	}
	if got := build.Result().PageCount; got != 0 {
		t.Fatalf("BuildResult.PageCount = %d, want 0 emitted pages", got)
	}
}

func TestBuildWritesStatusLandingPage(t *testing.T) {
	fixture := newBuildSelectionFixture(t)
	writeBuildSelectionFile(t, fixture.sitePath, `base_url: https://example.com/
title: Test Site
description: Test description
byline:
  name: Test Author
config:
  theme: theme.json
  status_layout: builtin/status.html
  status_redirect: /following/
feeds:
  updates:
    include:
      - path: "**"
`)
	statusSource := "---\nlayout: article\nvisibility: feed\ntitle: Feed-only update\n---\n\nstatus"
	writeBuildSelectionFile(t, fixture.pages[0], statusSource)
	writeBuildSelectionFile(t, fixture.pages[1], "---\nvisibility: none\n---\n\nhidden")
	build := fixture.build(nil, false)
	if err := build.LoadContext(context.Background(), io.Discard); err != nil {
		t.Fatalf("LoadContext() error = %v", err)
	}
	status := build.Website.pages.PageByURI(&url.URL{Path: fixture.pages[0]})
	if status == nil || status.Visibility != FeedVisibility || status.LayoutRef != "builtin/status.html" {
		t.Fatalf("loaded status = %#v", status)
	}
	if got := build.Website.TemplateData.Get("config.status_redirect"); got != "/following/" {
		t.Fatalf("site.config.status_redirect = %#v, want /following/", got)
	}
	pages, ok := build.Website.TemplateData.Get("pages").([]httt.TemplateData)
	if !ok || !slices.ContainsFunc(pages, func(page httt.TemplateData) bool {
		return page.Get("title") == status.Title
	}) {
		t.Fatal("status page is absent from site.pages")
	}
	feed, ok := build.Feeds.Feed(UserFeed, "updates")
	if !ok || !feed.Pages.Contains(status) {
		t.Fatalf("status page is absent from updates feed: %#v", feed)
	}
	if links, err := status.AtomEntryLinks(build.Website); err != nil || len(links) != 0 {
		t.Fatalf("status Atom links = %#v, %v; want none", links, err)
	}
	if err := build.Execute(context.Background()); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	output, err := os.ReadFile(filepath.Join(fixture.root, "builds", "test", "one", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(output), "url=/following/") {
		t.Fatalf("status output missing configured redirect:\n%s", output)
	}
	bytes, err := os.ReadFile(fixture.pages[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(bytes) != statusSource {
		t.Fatalf("status source changed:\n%s", bytes)
	}
}

func TestBuildExecuteUnsetArtifactFilesWritesNoArtifacts(t *testing.T) {
	fixture := newBuildExecutionFixture(t)
	build := fixture.build()

	if err := build.Execute(context.Background()); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, rel := range []string{"build.json", "build.log", ".deployment.json"} {
		if _, err := os.Stat(filepath.Join(fixture.buildPath, rel)); !os.IsNotExist(err) {
			t.Fatalf("%s exists or stat error = %v, want not exist", rel, err)
		}
	}
}

func TestBuildWriteFileLogsBuildRelativePathForExternalBuildRoot(t *testing.T) {
	fixture := newBuildExecutionFixture(t)
	build := fixture.build()
	build.Config.ID = "build-1"
	build.Config.URI = &url.URL{Path: filepath.Join(t.TempDir(), "remote-output", build.Config.ID)}
	if err := os.MkdirAll(build.Config.URI.Path, 0755); err != nil {
		t.Fatalf("MkdirAll(build.Config.URI.Path) error = %v", err)
	}

	var log bytes.Buffer
	if err := build.WriteFile(filepath.Join(build.Config.URI.Path, "index.html"), []byte("home"), &log); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if got, want := log.String(), "writing 4 bytes to build-1/index.html\n"; got != want {
		t.Fatalf("log = %q, want %q", got, want)
	}
}

func TestBuildCopyAssetLogsBuildRelativePathForExternalBuildRoot(t *testing.T) {
	fixture := newBuildExecutionFixture(t)
	build := fixture.build()
	build.Config.ID = "build-1"
	build.Config.URI = &url.URL{Path: filepath.Join(t.TempDir(), "remote-output", build.Config.ID)}
	writeBuildSelectionFile(t, filepath.Join(fixture.root, "static", "favicon.png"), "icon")

	var log bytes.Buffer
	if err := build.CopyWebsiteAsset("favicon.png", &log); err != nil {
		t.Fatalf("CopyWebsiteAsset() error = %v", err)
	}
	if got, want := log.String(), "copying \"static/favicon.png\" to \"build-1/favicon.png\"\n"; got != want {
		t.Fatalf("log = %q, want %q", got, want)
	}
}

func TestBuildAbsolutePathUsesBuildRelativePaths(t *testing.T) {
	root := t.TempDir()
	build := &Build{Config: BuildConfig{URI: &url.URL{Path: filepath.Join(root, "build")}}}
	want := filepath.Join(root, "build", "logs", "build.log")

	got, err := build.AbsolutePath("logs/build.log")
	if err != nil {
		t.Fatalf("AbsolutePath() error = %v", err)
	}
	if got != want {
		t.Fatalf("AbsolutePath() = %q, want %q", got, want)
	}
}

func TestBuildPathHelpersUseAbsoluteBuildRoot(t *testing.T) {
	build := &Build{Config: BuildConfig{URI: &url.URL{Path: filepath.Join("testdata", "site", ".builds", "test")}}}
	absRoot, err := filepath.Abs(build.Config.URI.Path)
	if err != nil {
		t.Fatalf("Abs(build.Config.URI.Path) error = %v", err)
	}

	absPath, err := build.AbsolutePath("posts/hello/index.html")
	if err != nil {
		t.Fatalf("AbsolutePath() error = %v", err)
	}
	if want := filepath.Join(absRoot, "posts", "hello", "index.html"); absPath != want {
		t.Fatalf("AbsolutePath() = %q, want %q", absPath, want)
	}

	relPath, err := build.RelativePath(filepath.Join(build.Config.URI.Path, "posts", "hello", "index.html"))
	if err != nil {
		t.Fatalf("RelativePath() error = %v", err)
	}
	if relPath != "posts/hello/index.html" {
		t.Fatalf("RelativePath() = %q, want posts/hello/index.html", relPath)
	}
}

func TestBuildValidateRejectsAbsoluteArtifactPaths(t *testing.T) {
	tests := []struct {
		name   string
		config BuildConfig
	}{
		{name: "data file", config: BuildConfig{DataFile: filepath.Join(t.TempDir(), "build.json")}},
		{name: "log file", config: BuildConfig{LogFile: filepath.Join(t.TempDir(), "build.log")}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pages := Pages{&Page{}}
			tc.config.URI = &url.URL{Path: t.TempDir()}
			build := &Build{Config: tc.config, Pages: &pages}
			err := build.Validate()
			if err == nil {
				t.Fatal("Validate() error = nil, want absolute artifact path error")
			}
			if !strings.Contains(err.Error(), "build artifact path must be relative") {
				t.Fatalf("Validate() error = %v, want relative artifact path error", err)
			}
		})
	}
}

func TestBuildExecuteRejectsInvalidDataFileBeforeWritingOutput(t *testing.T) {
	fixture := newBuildExecutionFixture(t)
	build := fixture.build()
	build.Config.DataFile = filepath.Join(t.TempDir(), "build.json")

	err := build.Execute(context.Background())
	if err == nil {
		t.Fatal("Execute() error = nil, want invalid data file error")
	}
	if !strings.Contains(err.Error(), "build artifact path must be relative") {
		t.Fatalf("Execute() error = %v, want relative artifact path error", err)
	}
	if _, statErr := os.Stat(filepath.Join(fixture.buildPath, "index.html")); !os.IsNotExist(statErr) {
		t.Fatalf("build output exists or stat error = %v, want not exist", statErr)
	}
}

func TestBuildExecuteContinuesWhenDeploymentInstructionsFail(t *testing.T) {
	fixture := newBuildExecutionFixture(t)
	build := fixture.build()
	build.Config.LogFile = "build.log"
	if err := os.MkdirAll(filepath.Join(fixture.buildPath, ".deployment.json"), 0755); err != nil {
		t.Fatalf("MkdirAll(.deployment.json) error = %v", err)
	}

	if err := build.Execute(context.Background()); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(fixture.buildPath, "index.html")); err != nil {
		t.Fatalf("build output missing: %v", err)
	}
	logBytes, err := os.ReadFile(filepath.Join(fixture.buildPath, "build.log"))
	if err != nil {
		t.Fatalf("ReadFile(build.log) error = %v", err)
	}
	if got := string(logBytes); !strings.Contains(got, "deployment instructions skipped:") {
		t.Fatalf("build.log = %q, want deployment instructions warning", got)
	}
}

func TestBuildExecuteContinuesWhenBuildDataFails(t *testing.T) {
	fixture := newBuildExecutionFixture(t)
	build := fixture.build()
	build.Config.DataFile = "reports/build.json"
	build.Config.LogFile = "build.log"
	if err := os.MkdirAll(fixture.buildPath, 0755); err != nil {
		t.Fatalf("MkdirAll(buildPath) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(fixture.buildPath, "reports"), []byte("not a directory"), 0644); err != nil {
		t.Fatalf("WriteFile(reports) error = %v", err)
	}

	if err := build.Execute(context.Background()); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(fixture.buildPath, "index.html")); err != nil {
		t.Fatalf("build output missing: %v", err)
	}
	logBytes, err := os.ReadFile(filepath.Join(fixture.buildPath, "build.log"))
	if err != nil {
		t.Fatalf("ReadFile(build.log) error = %v", err)
	}
	if got := string(logBytes); !strings.Contains(got, `build data "reports/build.json" skipped:`) {
		t.Fatalf("build.log = %q, want build data warning", got)
	}
}

func TestBuildWriteDeploymentInstructionsLogsBuildRelativePath(t *testing.T) {
	fixture := newBuildExecutionFixture(t)
	build := fixture.build()
	build.Config.DataFile = filepath.Join("artifacts", "build.json")

	var log bytes.Buffer
	build.writeDeploymentInstructions(&log)

	if got, want := log.String(), "writing 35 bytes to test/.deployment.json\n"; got != want {
		t.Fatalf("log = %q, want %q", got, want)
	}
}

func TestBuildExecuteAddsNestedArtifactToDeploymentInstructions(t *testing.T) {
	fixture := newBuildExecutionFixture(t)
	build := fixture.build()
	build.Config.DataFile = filepath.Join("artifacts", "build.json")

	if err := build.Execute(context.Background()); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	instructionsBytes, err := os.ReadFile(filepath.Join(fixture.buildPath, ".deployment.json"))
	if err != nil {
		t.Fatalf("ReadFile(.deployment.json) error = %v", err)
	}
	if !strings.Contains(string(instructionsBytes), "artifacts/build.json") {
		t.Fatalf(".deployment.json = %q, want artifacts/build.json", instructionsBytes)
	}
}
