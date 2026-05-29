package hypertexting

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/herdworks/hypertexting-go/href"
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

func writeBuildSelectionFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

func (fixture buildSelectionFixture) build(pages *Pages, incremental bool) *Build {
	return &Build{
		URI:         &url.URL{Path: filepath.Join(fixture.root, "builds", "test")},
		ID:          "test",
		Website:     &Website{URI: &href.Href{URL: url.URL{Path: fixture.sitePath}}},
		Incremental: incremental,
		Pages:       pages,
	}
}

func (fixture buildExecutionFixture) build() *Build {
	return &Build{
		URI:     &url.URL{Path: fixture.buildPath},
		ID:      "test",
		Website: &Website{URI: &href.Href{URL: url.URL{Path: fixture.sitePath}}},
	}
}

func TestBuildLoadContextFullBuildUsesDiscoveredPages(t *testing.T) {
	fixture := newBuildSelectionFixture(t)
	tests := []struct {
		name  string
		pages *Pages
	}{
		{name: "nil pages"},
		{name: "empty pages", pages: &Pages{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			build := fixture.build(tc.pages, false)
			if err := build.LoadContext(context.Background(), io.Discard); err != nil {
				t.Fatalf("LoadContext() error = %v", err)
			}
			if build.Pages == nil {
				t.Fatal("LoadContext() left build.Pages nil")
			}
			if got, want := len(*build.Pages), len(fixture.pages); got != want {
				t.Fatalf("len(build.Pages) = %d, want %d", got, want)
			}
		})
	}
}

func TestBuildLoadContextFullBuildRejectsExplicitPages(t *testing.T) {
	fixture := newBuildSelectionFixture(t)
	pages := Pages{&Page{URI: &url.URL{Path: fixture.pages[0]}}}
	build := fixture.build(&pages, false)

	err := build.LoadContext(context.Background(), io.Discard)
	if err == nil {
		t.Fatal("LoadContext() error = nil")
	}
	if !strings.Contains(err.Error(), "build.pages is only supported for incremental builds") {
		t.Fatalf("LoadContext() error = %v, want build.pages incremental error", err)
	}
}

func TestBuildLoadContextIncrementalBuildRequiresPages(t *testing.T) {
	fixture := newBuildSelectionFixture(t)
	tests := []struct {
		name  string
		pages *Pages
	}{
		{name: "nil pages"},
		{name: "empty pages", pages: &Pages{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			build := fixture.build(tc.pages, true)
			err := build.LoadContext(context.Background(), io.Discard)
			if err == nil {
				t.Fatal("LoadContext() error = nil")
			}
			if !strings.Contains(err.Error(), "build.pages is required for incremental builds") {
				t.Fatalf("LoadContext() error = %v, want required pages error", err)
			}
		})
	}
}

func TestBuildLoadContextIncrementalBuildPreservesRequestedPages(t *testing.T) {
	fixture := newBuildSelectionFixture(t)
	pages := Pages{&Page{URI: &url.URL{Path: fixture.pages[0]}}}
	build := fixture.build(&pages, true)

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

func TestBuildLoadContextFullBuildCanReloadWebsitePagesAlias(t *testing.T) {
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

func TestBuildLoadContextUsesBuildConfig(t *testing.T) {
	fixture := newBuildExecutionFixture(t)
	build := fixture.build()
	build.Config = BuildConfig{
		WebsiteConfig: WebsiteConfig{ContentDirectory: "custom-content"},
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
	build.Config = BuildConfig{DataFile: "build.json", LogFile: "build.log", Verbose: true}

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

	ignoreBytes, err := os.ReadFile(filepath.Join(fixture.buildPath, ".deploymentignore"))
	if err != nil {
		t.Fatalf("ReadFile(.deploymentignore) error = %v", err)
	}
	ignore := string(ignoreBytes)
	for _, want := range []string{"build.json", "build.log"} {
		if !strings.Contains(ignore, want) {
			t.Fatalf(".deploymentignore = %q, want %q", ignore, want)
		}
	}
}

func TestBuildExecuteUnsetArtifactFilesWritesNoArtifacts(t *testing.T) {
	fixture := newBuildExecutionFixture(t)
	build := fixture.build()

	if err := build.Execute(context.Background()); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, rel := range []string{"build.json", "build.log", ".deploymentignore"} {
		if _, err := os.Stat(filepath.Join(fixture.buildPath, rel)); !os.IsNotExist(err) {
			t.Fatalf("%s exists or stat error = %v, want not exist", rel, err)
		}
	}
}

func TestBuildWriteFileLogsBuildRelativePathForExternalBuildRoot(t *testing.T) {
	fixture := newBuildExecutionFixture(t)
	build := fixture.build()
	build.ID = "build-1"
	build.URI = &url.URL{Path: filepath.Join(t.TempDir(), "remote-output", build.ID)}
	if err := os.MkdirAll(build.URI.Path, 0755); err != nil {
		t.Fatalf("MkdirAll(build.URI.Path) error = %v", err)
	}

	var log bytes.Buffer
	if err := build.WriteFile(filepath.Join(build.URI.Path, "index.html"), []byte("home"), &log); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if got, want := log.String(), "writing 4 bytes to build-1/index.html\n"; got != want {
		t.Fatalf("log = %q, want %q", got, want)
	}
}

func TestBuildCopyAssetLogsBuildRelativePathForExternalBuildRoot(t *testing.T) {
	fixture := newBuildExecutionFixture(t)
	build := fixture.build()
	build.ID = "build-1"
	build.URI = &url.URL{Path: filepath.Join(t.TempDir(), "remote-output", build.ID)}
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
	build := &Build{URI: &url.URL{Path: filepath.Join(root, "build")}}
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
	build := &Build{URI: &url.URL{Path: filepath.Join("testdata", "site", ".builds", "test")}}
	absRoot, err := filepath.Abs(build.URI.Path)
	if err != nil {
		t.Fatalf("Abs(build.URI.Path) error = %v", err)
	}

	absPath, err := build.AbsolutePath("posts/hello/index.html")
	if err != nil {
		t.Fatalf("AbsolutePath() error = %v", err)
	}
	if want := filepath.Join(absRoot, "posts", "hello", "index.html"); absPath != want {
		t.Fatalf("AbsolutePath() = %q, want %q", absPath, want)
	}

	relPath, err := build.RelativePath(filepath.Join(build.URI.Path, "posts", "hello", "index.html"))
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
			build := &Build{URI: &url.URL{Path: t.TempDir()}, Config: tc.config, Pages: &pages}
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

func TestBuildExecuteContinuesWhenDeploymentIgnoreFails(t *testing.T) {
	fixture := newBuildExecutionFixture(t)
	build := fixture.build()
	build.Config.LogFile = "build.log"
	if err := os.MkdirAll(filepath.Join(fixture.buildPath, ".deploymentignore"), 0755); err != nil {
		t.Fatalf("MkdirAll(.deploymentignore) error = %v", err)
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
	if got := string(logBytes); !strings.Contains(got, "deployment ignore skipped:") {
		t.Fatalf("build.log = %q, want deployment ignore warning", got)
	}
}

func TestBuildExecuteContinuesWhenBuildDataFails(t *testing.T) {
	fixture := newBuildExecutionFixture(t)
	build := fixture.build()
	build.Config = BuildConfig{DataFile: "reports/build.json", LogFile: "build.log"}
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

func TestBuildWriteDeploymentIgnoreLogsBuildRelativePath(t *testing.T) {
	fixture := newBuildExecutionFixture(t)
	build := fixture.build()
	build.Config.DataFile = filepath.Join("artifacts", "build.json")

	var log bytes.Buffer
	build.writeDeploymentIgnore(&log)

	if got, want := log.String(), "writing 21 bytes to test/.deploymentignore\n"; got != want {
		t.Fatalf("log = %q, want %q", got, want)
	}
}

func TestBuildExecuteAddsNestedArtifactToDeploymentIgnore(t *testing.T) {
	fixture := newBuildExecutionFixture(t)
	build := fixture.build()
	build.Config.DataFile = filepath.Join("artifacts", "build.json")

	if err := build.Execute(context.Background()); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	ignoreBytes, err := os.ReadFile(filepath.Join(fixture.buildPath, ".deploymentignore"))
	if err != nil {
		t.Fatalf("ReadFile(.deploymentignore) error = %v", err)
	}
	if got := string(ignoreBytes); !strings.Contains(got, "artifacts/build.json") {
		t.Fatalf(".deploymentignore = %q, want artifacts/build.json", got)
	}
}
