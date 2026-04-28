package hypertexting

import (
	"net/url"
	"path/filepath"
	"testing"

	"github.com/herdworks/hypertexting-go/href"
)

func TestPagesFilterDraft(t *testing.T) {
	published := &Page{Title: "Published"}
	draft := &Page{Title: "Draft", Draft: true}
	pages := Pages{published, draft, nil}

	gotPublished := pages.FilterDraft(false)
	if len(gotPublished) != 1 || gotPublished[0] != published {
		t.Fatalf("FilterDraft(false) = %#v, want published page", gotPublished)
	}

	gotDrafts := pages.FilterDraft(true)
	if len(gotDrafts) != 1 || gotDrafts[0] != draft {
		t.Fatalf("FilterDraft(true) = %#v, want draft page", gotDrafts)
	}
}

func TestContentTypePageURIUsesContentDirForDraftPages(t *testing.T) {
	website, _ := testWebsite(t)
	contenttype := &ContentType{Spec: ContentTypeSpecification{
		Format: "markdown",
		Path:   "posts",
	}}
	page := &Page{Draft: true, Slug: "draft-post"}

	got := contenttype.PageURI(website, page).Path
	want := filepath.Join(website.ContentDirPath(), "posts", "draft-post", "index.md")
	if got != want {
		t.Fatalf("PageURI() = %q, want %q", got, want)
	}
}

func TestPagePathWithRelativeWebsiteRoot(t *testing.T) {
	sourcePath, err := filepath.Abs(filepath.Join("testdata", "site", "content", "about", "index.html"))
	if err != nil {
		t.Fatalf("Abs() error = %v", err)
	}
	website := &Website{URI: &href.Href{URL: url.URL{Path: filepath.Join("testdata", "site", "site.yaml")}}}
	pageURI := &url.URL{Path: sourcePath}

	got := PagePath(website, pageURI)
	if got != "/about/" {
		t.Fatalf("PagePath() = %q, want %q", got, "/about/")
	}
}

func TestBuildPathWithRelativeWebsiteRoot(t *testing.T) {
	sourcePath, err := filepath.Abs(filepath.Join("testdata", "site", "content", "about", "index.html"))
	if err != nil {
		t.Fatalf("Abs() error = %v", err)
	}
	website := &Website{URI: &href.Href{URL: url.URL{Path: filepath.Join("testdata", "site", "site.yaml")}}}
	build := &HyperBuild{
		URI:     &url.URL{Path: filepath.Join("testdata", "site", ".builds", "test")},
		Website: website,
	}
	page := &Page{URI: &url.URL{Path: sourcePath}, Path: "/about/"}

	got := build.BuildPath(page)
	want := filepath.Join("testdata", "site", ".builds", "test", "about", "index.html")
	if got != want {
		t.Fatalf("BuildPath() = %q, want %q", got, want)
	}
}
