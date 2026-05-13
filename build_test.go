package hypertexting

import (
	"context"
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

func writeBuildSelectionFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

func (fixture buildSelectionFixture) build(pages *Pages, incremental bool) *HyperBuild {
	return &HyperBuild{
		URI:         &url.URL{Path: filepath.Join(fixture.root, "builds", "test")},
		ID:          "test",
		Website:     &Website{URI: &href.Href{URL: url.URL{Path: fixture.sitePath}}},
		Incremental: incremental,
		Pages:       pages,
	}
}

func TestHyperBuildLoadContextFullBuildUsesDiscoveredPages(t *testing.T) {
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
			if err := build.LoadContext(context.Background(), nil, nil, false); err != nil {
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

func TestHyperBuildLoadContextFullBuildRejectsExplicitPages(t *testing.T) {
	fixture := newBuildSelectionFixture(t)
	pages := Pages{&Page{URI: &url.URL{Path: fixture.pages[0]}}}
	build := fixture.build(&pages, false)

	err := build.LoadContext(context.Background(), nil, nil, false)
	if err == nil {
		t.Fatal("LoadContext() error = nil")
	}
	if !strings.Contains(err.Error(), "build.pages is only supported for incremental builds") {
		t.Fatalf("LoadContext() error = %v, want build.pages incremental error", err)
	}
}

func TestHyperBuildLoadContextIncrementalBuildRequiresPages(t *testing.T) {
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
			err := build.LoadContext(context.Background(), nil, nil, false)
			if err == nil {
				t.Fatal("LoadContext() error = nil")
			}
			if !strings.Contains(err.Error(), "build.pages is required for incremental builds") {
				t.Fatalf("LoadContext() error = %v, want required pages error", err)
			}
		})
	}
}

func TestHyperBuildLoadContextIncrementalBuildPreservesRequestedPages(t *testing.T) {
	fixture := newBuildSelectionFixture(t)
	pages := Pages{&Page{URI: &url.URL{Path: fixture.pages[0]}}}
	build := fixture.build(&pages, true)

	if err := build.LoadContext(context.Background(), nil, nil, false); err != nil {
		t.Fatalf("LoadContext() error = %v", err)
	}
	if got, want := len(*build.Pages), 1; got != want {
		t.Fatalf("len(build.Pages) = %d, want %d", got, want)
	}
	if got, want := (*build.Pages)[0].URI.Path, fixture.pages[0]; got != want {
		t.Fatalf("build.Pages[0].URI.Path = %q, want %q", got, want)
	}
}

func TestHyperBuildLoadContextFullBuildCanReloadWebsitePagesAlias(t *testing.T) {
	fixture := newBuildSelectionFixture(t)
	build := fixture.build(nil, false)
	if err := build.LoadContext(context.Background(), nil, nil, false); err != nil {
		t.Fatalf("first LoadContext() error = %v", err)
	}
	if build.Pages == nil || len(*build.Pages) == 0 {
		t.Fatal("first LoadContext() did not load pages")
	}
	if err := build.LoadContext(context.Background(), nil, nil, false); err != nil {
		t.Fatalf("second LoadContext() error = %v", err)
	}
	if got, want := len(*build.Pages), len(fixture.pages); got != want {
		t.Fatalf("len(build.Pages) after reload = %d, want %d", got, want)
	}
}
