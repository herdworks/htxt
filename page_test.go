package hypertexting

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
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

func TestWritePageTruncatesExistingFile(t *testing.T) {
	website, _ := testWebsite(t)
	tests := []struct {
		name   string
		format string
		ext    string
	}{
		{name: "json", format: "json", ext: ".json"},
		{name: "yaml", format: "yaml", ext: ".yaml"},
		{name: "markdown", format: "markdown", ext: ".md"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(website.ContentDirPath(), "shorter-"+tc.name, "index"+tc.ext)
			uri := &url.URL{Path: path}
			first := &Page{
				URI:         uri,
				CreatedAt:   "2026-05-19T00:00:00Z",
				LayoutRef:   "default",
				Title:       "Long Page",
				Description: "stale description should disappear after shorter save",
				Content:     "stale body should disappear after shorter save",
			}
			if err := WritePage(website, first, tc.format); err != nil {
				t.Fatalf("first WritePage() error = %v", err)
			}

			second := &Page{
				URI:       uri,
				CreatedAt: "2026-05-19T00:00:00Z",
				LayoutRef: "default",
				Title:     "Short Page",
				Content:   "ok",
			}
			if err := WritePage(website, second, tc.format); err != nil {
				t.Fatalf("second WritePage() error = %v", err)
			}

			bytes, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("ReadFile() error = %v", err)
			}
			contents := string(bytes)
			for _, stale := range []string{"stale description", "stale body"} {
				if strings.Contains(contents, stale) {
					t.Fatalf("saved page still contains stale data %q:\n%s", stale, contents)
				}
			}

			loaded := &Page{URI: uri}
			if err := LoadPage(website, loaded, false); err != nil {
				t.Fatalf("LoadPage() error = %v", err)
			}
			if loaded.Title != "Short Page" {
				t.Fatalf("loaded Title = %q, want %q", loaded.Title, "Short Page")
			}
			if tc.format == "markdown" && !strings.Contains(loaded.Content, "ok") {
				t.Fatalf("loaded Content = %q, want content containing %q", loaded.Content, "ok")
			}
			if tc.format != "markdown" && loaded.Content != "ok" {
				t.Fatalf("loaded Content = %q, want %q", loaded.Content, "ok")
			}
		})
	}
}

func testPageContentType(format string, contentPath string) *ContentType {
	return &ContentType{
		Metadata: ContentTypeMetadata{Name: "Test"},
		Spec: ContentTypeSpecification{
			Layout: "default",
			Format: format,
			Path:   contentPath,
		},
	}
}

func TestCreatePageRequiresEmptyURI(t *testing.T) {
	website, _ := testWebsite(t)
	contenttype := testPageContentType("markdown", "posts")
	page := &Page{URI: &url.URL{Path: filepath.Join(website.ContentDirPath(), "posts", "hello", "index.md")}}

	err := CreatePage(website, page, contenttype)
	if err == nil || !strings.Contains(err.Error(), "page.create requires page.uri to be empty") {
		t.Fatalf("CreatePage() error = %v, want page.uri guard", err)
	}
}

func TestCreatePageFailsIfResolvedPathExists(t *testing.T) {
	website, _ := testWebsite(t)
	contenttype := testPageContentType("markdown", "posts")
	destinationPath := filepath.Join(website.ContentDirPath(), "posts", "duplicate", "index.md")
	if err := os.MkdirAll(filepath.Dir(destinationPath), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(destinationPath, []byte("existing"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	page := &Page{Slug: "duplicate", Title: "Duplicate", Content: "new content"}

	err := CreatePage(website, page, contenttype)
	if err == nil || !strings.Contains(err.Error(), "page.create conflict") {
		t.Fatalf("CreatePage() error = %v, want conflict", err)
	}
}

func TestUpdatePageRehomesRegularFilesAndPreservesNestedPages(t *testing.T) {
	website, _ := testWebsite(t)
	sourceDir := filepath.Join(website.ContentDirPath(), "drafts", "working")
	sourcePath := filepath.Join(sourceDir, "index.md")
	assetPath := filepath.Join(sourceDir, "photo.txt")
	nestedPagePath := filepath.Join(sourceDir, "child", "index.md")
	for path, contents := range map[string]string{
		sourcePath:     "old page",
		assetPath:      "asset",
		nestedPagePath: "nested page",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", path, err)
		}
		if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", path, err)
		}
	}
	contenttype := testPageContentType("markdown", "posts")
	page := &Page{
		URI:       &url.URL{Path: sourcePath},
		Slug:      "updated",
		CreatedAt: "2026-05-20T00:00:00Z",
		LayoutRef: "default",
		Title:     "Updated",
		Content:   "updated content",
		Draft:     true,
	}

	if err := UpdatePage(website, page, contenttype); err != nil {
		t.Fatalf("UpdatePage() error = %v", err)
	}

	destinationDir := filepath.Join(website.ContentDirPath(), "posts", "updated")
	destinationPath := filepath.Join(destinationDir, "index.md")
	if page.URI == nil || page.URI.Path != destinationPath {
		t.Fatalf("page.URI = %#v, want %s", page.URI, destinationPath)
	}
	if page.Path != "/posts/updated/" {
		t.Fatalf("page.Path = %q, want /posts/updated/", page.Path)
	}
	if _, err := os.Stat(destinationPath); err != nil {
		t.Fatalf("destination page missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destinationDir, "photo.txt")); err != nil {
		t.Fatalf("destination asset missing: %v", err)
	}
	if _, err := os.Stat(nestedPagePath); err != nil {
		t.Fatalf("nested page was not preserved: %v", err)
	}
	if _, err := os.Stat(sourcePath); !os.IsNotExist(err) {
		t.Fatalf("source page still exists or unexpected error = %v", err)
	}
	if _, err := os.Stat(assetPath); !os.IsNotExist(err) {
		t.Fatalf("source asset still exists or unexpected error = %v", err)
	}
	bytes, err := os.ReadFile(destinationPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(bytes), "updated content") {
		t.Fatalf("updated page contents = %q, want updated content", string(bytes))
	}
}

func TestUpdatePageRehomesRootRegularFiles(t *testing.T) {
	website, _ := testWebsite(t)
	sourcePath := filepath.Join(website.ContentDirPath(), "index.md")
	robotsPath := filepath.Join(website.ContentDirPath(), "robots.txt")
	nestedPagePath := filepath.Join(website.ContentDirPath(), "posts", "hello", "index.md")
	for path, contents := range map[string]string{
		sourcePath:     "root page",
		robotsPath:     "User-agent: *",
		nestedPagePath: "nested page",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", path, err)
		}
		if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", path, err)
		}
	}
	contenttype := testPageContentType("markdown", "pages")
	page := &Page{
		URI:       &url.URL{Path: sourcePath},
		Slug:      "home",
		CreatedAt: "2026-05-20T00:00:00Z",
		LayoutRef: "default",
		Title:     "Home",
		Content:   "home content",
		Draft:     true,
	}

	if err := UpdatePage(website, page, contenttype); err != nil {
		t.Fatalf("UpdatePage() error = %v", err)
	}

	destinationDir := filepath.Join(website.ContentDirPath(), "pages", "home")
	if _, err := os.Stat(filepath.Join(destinationDir, "robots.txt")); err != nil {
		t.Fatalf("root-owned file was not moved: %v", err)
	}
	if _, err := os.Stat(nestedPagePath); err != nil {
		t.Fatalf("nested page was not preserved: %v", err)
	}
	if _, err := os.Stat(robotsPath); !os.IsNotExist(err) {
		t.Fatalf("source root-owned file still exists or unexpected error = %v", err)
	}
}

func TestUpdatePageFailsIfResolvedPathExists(t *testing.T) {
	website, _ := testWebsite(t)
	sourcePath := filepath.Join(website.ContentDirPath(), "drafts", "working", "index.md")
	destinationPath := filepath.Join(website.ContentDirPath(), "posts", "taken", "index.md")
	for path, contents := range map[string]string{
		sourcePath:      "source",
		destinationPath: "existing destination",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", path, err)
		}
		if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", path, err)
		}
	}
	contenttype := testPageContentType("markdown", "posts")
	page := &Page{URI: &url.URL{Path: sourcePath}, Slug: "taken", CreatedAt: "2026-05-20T00:00:00Z", Content: "new", Draft: true}

	err := UpdatePage(website, page, contenttype)
	if err == nil || !strings.Contains(err.Error(), "page.update conflict") {
		t.Fatalf("UpdatePage() error = %v, want conflict", err)
	}
	if _, statErr := os.Stat(sourcePath); statErr != nil {
		t.Fatalf("source page should remain after conflict: %v", statErr)
	}
}

func TestUpdatePageFailsIfRegularFileWouldOverwrite(t *testing.T) {
	website, _ := testWebsite(t)
	sourceDir := filepath.Join(website.ContentDirPath(), "drafts", "working")
	destinationDir := filepath.Join(website.ContentDirPath(), "posts", "updated")
	sourcePath := filepath.Join(sourceDir, "index.md")
	sourceAssetPath := filepath.Join(sourceDir, "photo.txt")
	destinationAssetPath := filepath.Join(destinationDir, "photo.txt")
	for path, contents := range map[string]string{
		sourcePath:           "source",
		sourceAssetPath:      "source asset",
		destinationAssetPath: "destination asset",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", path, err)
		}
		if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", path, err)
		}
	}
	contenttype := testPageContentType("markdown", "posts")
	page := &Page{URI: &url.URL{Path: sourcePath}, Slug: "updated", CreatedAt: "2026-05-20T00:00:00Z", Content: "new", Draft: true}

	err := UpdatePage(website, page, contenttype)
	if err == nil || !strings.Contains(err.Error(), "regular file already exists") {
		t.Fatalf("UpdatePage() error = %v, want regular file conflict", err)
	}
	if _, statErr := os.Stat(sourceAssetPath); statErr != nil {
		t.Fatalf("source asset should remain after conflict: %v", statErr)
	}
}

func TestUpdatePageFormatChangeRemovesOldPageFile(t *testing.T) {
	website, _ := testWebsite(t)
	sourcePath := filepath.Join(website.ContentDirPath(), "posts", "updated", "index.md")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("old markdown"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	contenttype := testPageContentType("json", "posts")
	page := &Page{URI: &url.URL{Path: sourcePath}, Slug: "updated", CreatedAt: "2026-05-20T00:00:00Z", Title: "Updated", Content: "json content", Draft: true}

	if err := UpdatePage(website, page, contenttype); err != nil {
		t.Fatalf("UpdatePage() error = %v", err)
	}
	destinationPath := filepath.Join(website.ContentDirPath(), "posts", "updated", "index.json")
	if _, err := os.Stat(destinationPath); err != nil {
		t.Fatalf("destination json page missing: %v", err)
	}
	if _, err := os.Stat(sourcePath); !os.IsNotExist(err) {
		t.Fatalf("source markdown page still exists or unexpected error = %v", err)
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
