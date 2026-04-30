package hypertexting

import (
	"net/url"
	"os"
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

func TestPageResolvePath(t *testing.T) {
	website, _ := testWebsite(t)
	tests := []struct {
		name    string
		source  string
		uglyURL bool
		want    string
	}{
		{
			name:   "default index file path",
			source: filepath.Join(website.ContentDirPath(), "about", "index.md"),
			want:   "/about/",
		},
		{
			name:    "ugly url",
			source:  filepath.Join(website.ContentDirPath(), "404", "index.md"),
			uglyURL: true,
			want:    "/404.html",
		},
		{
			name:    "nested ugly url",
			source:  filepath.Join(website.ContentDirPath(), "posts", "hello", "index.md"),
			uglyURL: true,
			want:    "/posts/hello.html",
		},
		{
			name:    "root page stays index path",
			source:  filepath.Join(website.ContentDirPath(), "index.md"),
			uglyURL: true,
			want:    "/",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			page := &Page{URI: &url.URL{Path: tc.source}, UglyURL: tc.uglyURL}
			page.ResolvePath(website)
			if page.Path != tc.want {
				t.Fatalf("ResolvePath() path = %q, want %q", page.Path, tc.want)
			}
		})
	}
}

func TestLoadPageResolvesUglyURLPathAfterParsing(t *testing.T) {
	website, _ := testWebsite(t)
	sourcePath := filepath.Join(website.ContentDirPath(), "404", "index.md")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	content := []byte("---\nugly_url: true\nlayout: default\n---\n\nnot found")
	if err := os.WriteFile(sourcePath, content, 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	page := &Page{URI: &url.URL{Path: sourcePath}, Path: PagePath(website, &url.URL{Path: sourcePath})}

	if err := LoadPage(website, page, false); err != nil {
		t.Fatalf("LoadPage() error = %v", err)
	}
	if !page.UglyURL {
		t.Fatal("LoadPage() did not parse ugly_url")
	}
	if page.Path != "/404.html" {
		t.Fatalf("LoadPage() path = %q, want %q", page.Path, "/404.html")
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

func TestBuildPathWithPagePath(t *testing.T) {
	_, build := testWebsite(t)
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "index file path",
			path: "/about/",
			want: filepath.Join(build.URI.Path, "about", "index.html"),
		},
		{
			name: "ugly url",
			path: "/404.html",
			want: filepath.Join(build.URI.Path, "404.html"),
		},
		{
			name: "nested ugly url",
			path: "/posts/hello.html",
			want: filepath.Join(build.URI.Path, "posts", "hello.html"),
		},
		{
			name: "root path",
			path: "/",
			want: filepath.Join(build.URI.Path, "index.html"),
		},
		{
			name: "path without trailing slash",
			path: "/tags/go",
			want: filepath.Join(build.URI.Path, "tags", "go", "index.html"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := build.BuildPath(&Page{Path: tc.path})
			if got != tc.want {
				t.Fatalf("BuildPath() = %q, want %q", got, tc.want)
			}
		})
	}
}
