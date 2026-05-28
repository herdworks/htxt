package hypertexting

import (
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/herdworks/hypertexting-go/href"
)

func assertTimestampDraftSegment(t *testing.T, draftSegment string) {
	t.Helper()
	if len(draftSegment) != 12 || !strings.HasPrefix(draftSegment, "d") {
		t.Fatalf("draft URI segment = %q, want d followed by 11 hex digits", draftSegment)
	}
	hex := draftSegment[1:]
	if strings.ToLower(hex) != hex {
		t.Fatalf("draft URI segment = %q, want lowercase hex", draftSegment)
	}
	if _, err := strconv.ParseInt(hex, 16, 64); err != nil {
		t.Fatalf("draft URI segment = %q, want valid hex: %v", draftSegment, err)
	}
}

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

func TestResolvePageURIUsesContentDirForDraftPages(t *testing.T) {
	website, _ := testWebsite(t)
	contenttype := &ContentType{Metadata: ContentTypeMetadata{Name: "Test"}, Spec: ContentTypeSpecification{
		Layout:      "default",
		URIResolver: ManualURIResolver,
		Format:      "markdown",
		Path:        "posts",
	}}
	page := &Page{Draft: true, Slug: "draft-post"}

	uri, err := ResolvePageURI(website, page, contenttype, nil)
	if err != nil {
		t.Fatalf("ResolvePageURI() error = %v", err)
	}
	got := uri.Path
	want := filepath.Join(website.ContentDirPath(), "posts", "draft-post", "index.md")
	if got != want {
		t.Fatalf("ResolvePageURI() = %q, want %q", got, want)
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

func TestPageCanonicalURL(t *testing.T) {
	website, _ := testWebsite(t)
	page := &Page{Path: "/about/"}

	canonicalURL, err := page.CanonicalURL(website)
	if err != nil {
		t.Fatalf("CanonicalURL() error = %v", err)
	}
	if got, want := canonicalURL.String(), "https://example.com/about/"; got != want {
		t.Fatalf("CanonicalURL() = %q, want %q", got, want)
	}
}

func TestPageResolveTemplateDataSetsCanonicalURL(t *testing.T) {
	website, _ := testWebsite(t)
	page := &Page{Path: "/about/"}

	page.ResolveTemplateData(website)
	if got, want := page.TemplateData.Get("canonical_url"), "https://example.com/about/"; got != want {
		t.Fatalf("canonical_url = %q, want %q", got, want)
	}
}

func TestPageAtomEntryUsesCanonicalURLForIDAndRelatedLink(t *testing.T) {
	website, _ := testWebsite(t)
	page := Page{
		Path:     "/about/",
		Redirect: "https://elsewhere.example/post/",
	}

	entry := page.AtomEntry(website)
	if entry == nil {
		t.Fatal("AtomEntry() returned nil")
	}
	if got, want := entry.ID, "https://example.com/about/"; got != want {
		t.Fatalf("entry ID = %q, want %q", got, want)
	}

	links := map[string]string{}
	for _, link := range entry.Links {
		links[link.Rel] = link.Href
	}
	if got, want := links["alternate"], "https://elsewhere.example/post/"; got != want {
		t.Fatalf("alternate link = %q, want %q", got, want)
	}
	if got, want := links["related"], "https://example.com/about/"; got != want {
		t.Fatalf("related link = %q, want %q", got, want)
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

	if err := LoadPage(website, page, false, io.Discard); err != nil {
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
			if err := WritePage(website, first, tc.format, io.Discard); err != nil {
				t.Fatalf("first WritePage() error = %v", err)
			}

			second := &Page{
				URI:       uri,
				CreatedAt: "2026-05-19T00:00:00Z",
				LayoutRef: "default",
				Title:     "Short Page",
				Content:   "ok",
			}
			if err := WritePage(website, second, tc.format, io.Discard); err != nil {
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
			if err := LoadPage(website, loaded, false, io.Discard); err != nil {
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
			Layout:      "default",
			URIResolver: TimestampURIResolver,
			Format:      format,
			Path:        contentPath,
		},
	}
}

func testPageContentTypeWithURIResolver(format string, contentPath string, resolver PageURIResolverName) *ContentType {
	contenttype := testPageContentType(format, contentPath)
	contenttype.Spec.URIResolver = resolver
	return contenttype
}

func writePageTestFile(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", path, err)
	}
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

func TestCreatePageRequiresEmptyURI(t *testing.T) {
	website, _ := testWebsite(t)
	contenttype := testPageContentType("markdown", "posts")
	page := &Page{URI: &url.URL{Path: filepath.Join(website.ContentDirPath(), "posts", "hello", "index.md")}}

	err := CreatePage(website, page, contenttype, io.Discard)
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

	err := CreatePage(website, page, contenttype, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "page.create conflict") {
		t.Fatalf("CreatePage() error = %v, want conflict", err)
	}
}

func TestCreatePageWithNilContentTypeUsesDefaultTimestampPath(t *testing.T) {
	website, _ := testWebsite(t)
	page := &Page{Content: "default content"}

	if err := CreatePage(website, page, nil, io.Discard); err != nil {
		t.Fatalf("CreatePage() error = %v", err)
	}
	if page.URI == nil {
		t.Fatal("page.URI is nil")
	}
	pageDir := filepath.Dir(page.URI.Path)
	timestampSegment := filepath.Base(pageDir)
	if filepath.Dir(pageDir) != filepath.Join(website.ContentDirPath(), "-") {
		t.Fatalf("page.URI = %s, want under default '-' content path", page.URI.Path)
	}
	if _, err := strconv.ParseInt(timestampSegment, 10, 64); err != nil {
		t.Fatalf("default URI segment = %q, want millisecond timestamp", timestampSegment)
	}
	if filepath.Base(page.URI.Path) != "index.md" {
		t.Fatalf("page.URI = %s, want markdown index file", page.URI.Path)
	}
	if page.Path != "/-/"+timestampSegment+"/" {
		t.Fatalf("page.Path = %q, want /-/%s/", page.Path, timestampSegment)
	}
	bytes, err := os.ReadFile(page.URI.Path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(bytes), "default content") {
		t.Fatalf("default page contents = %q, want page content", string(bytes))
	}
}

func TestCreatePageWithTemplatedContentTypePath(t *testing.T) {
	tests := []struct {
		name     string
		resolver PageURIResolverName
		wantSlug string
	}{
		{name: "title kabob", resolver: TitleKabobURIResolver, wantSlug: "hello-world"},
		{name: "title snake", resolver: TitleSnakeURIResolver, wantSlug: "hello_world"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			website, _ := testWebsite(t)
			contenttype := &ContentType{
				Metadata: ContentTypeMetadata{Name: "Blog"},
				Spec: ContentTypeSpecification{
					Layout:      "default",
					Path:        "/blog/${year}/${month}/${day}/",
					URIResolver: tc.resolver,
					Format:      "markdown",
				},
			}
			page := &Page{
				CreatedAt: "2026-05-24T13:30:00-07:00",
				Title:     "Hello World",
				Content:   "templated content type path",
			}

			if err := CreatePage(website, page, contenttype, io.Discard); err != nil {
				t.Fatalf("CreatePage() error = %v", err)
			}
			wantPath := filepath.Join(website.ContentDirPath(), "blog", "2026", "05", "24", tc.wantSlug, "index.md")
			if page.URI == nil || page.URI.Path != wantPath {
				t.Fatalf("page.URI = %#v, want %s", page.URI, wantPath)
			}
			if page.Path != "/blog/2026/05/24/"+tc.wantSlug+"/" {
				t.Fatalf("page.Path = %q, want templated date path", page.Path)
			}
		})
	}
}

func TestUpdatePageWithTemplatedContentTypePathUsesExistingCreatedAt(t *testing.T) {
	website, _ := testWebsite(t)
	sourcePath := filepath.Join(website.ContentDirPath(), "drafts", "working", "index.md")
	writePageTestFile(t, sourcePath, "---\ncreated_at: 2026-05-20T00:00:00Z\nlayout: default\ndraft: true\n---\n\nold")
	contenttype := &ContentType{
		Metadata: ContentTypeMetadata{Name: "Blog"},
		Spec: ContentTypeSpecification{
			Layout:      "default",
			Path:        "/blog/${year}/${month}/${day}/",
			URIResolver: TitleKabobURIResolver,
			Format:      "markdown",
		},
	}
	page := &Page{URI: &url.URL{Path: sourcePath}, Title: "Published Draft", Content: "published"}

	if err := UpdatePage(website, page, contenttype, io.Discard); err != nil {
		t.Fatalf("UpdatePage() error = %v", err)
	}
	destinationPath := filepath.Join(website.ContentDirPath(), "blog", "2026", "05", "20", "published-draft", "index.md")
	if page.URI == nil || page.URI.Path != destinationPath {
		t.Fatalf("page.URI = %#v, want %s", page.URI, destinationPath)
	}
	if page.Path != "/blog/2026/05/20/published-draft/" {
		t.Fatalf("page.Path = %q, want existing created_at date path", page.Path)
	}
	if _, err := os.Stat(destinationPath); err != nil {
		t.Fatalf("destination page missing: %v", err)
	}
	if _, err := os.Stat(sourcePath); !os.IsNotExist(err) {
		t.Fatalf("source page still exists or unexpected error = %v", err)
	}
}

func TestCreateDraftPageWithEmptySlugUsesTemporaryURI(t *testing.T) {
	website, _ := testWebsite(t)
	contenttype := testPageContentTypeWithURIResolver("markdown", "posts", TitleKabobURIResolver)
	page := &Page{Draft: true, Content: "draft content"}

	if err := CreatePage(website, page, contenttype, io.Discard); err != nil {
		t.Fatalf("CreatePage() error = %v", err)
	}
	if page.Slug != "" {
		t.Fatalf("page.Slug = %q, want empty", page.Slug)
	}
	if page.URI == nil {
		t.Fatal("page.URI is nil")
	}
	pageDir := filepath.Dir(page.URI.Path)
	draftSegment := filepath.Base(pageDir)
	if filepath.Dir(pageDir) != filepath.Join(website.ContentDirPath(), "posts") {
		t.Fatalf("page.URI = %s, want under posts draft directory", page.URI.Path)
	}
	assertTimestampDraftSegment(t, draftSegment)
	if page.Path != "/posts/"+draftSegment+"/" {
		t.Fatalf("page.Path = %q, want computed draft path", page.Path)
	}
	bytes, err := os.ReadFile(page.URI.Path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if strings.Contains(string(bytes), "slug:") {
		t.Fatalf("draft page serialized temporary slug:\n%s", string(bytes))
	}
}

func TestCreateDraftPageWithUserSlugPreservesSlug(t *testing.T) {
	website, _ := testWebsite(t)
	contenttype := testPageContentTypeWithURIResolver("markdown", "posts", TitleKabobURIResolver)
	page := &Page{Draft: true, Slug: "custom-draft", Content: "draft content"}

	if err := CreatePage(website, page, contenttype, io.Discard); err != nil {
		t.Fatalf("CreatePage() error = %v", err)
	}
	wantPath := filepath.Join(website.ContentDirPath(), "posts", "custom-draft", "index.md")
	if page.URI == nil || page.URI.Path != wantPath {
		t.Fatalf("page.URI = %#v, want %s", page.URI, wantPath)
	}
	if page.Slug != "custom-draft" {
		t.Fatalf("page.Slug = %q, want custom-draft", page.Slug)
	}
	if page.Path != "/posts/custom-draft/" {
		t.Fatalf("page.Path = %q, want /posts/custom-draft/", page.Path)
	}
	bytes, err := os.ReadFile(page.URI.Path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(bytes), "slug: custom-draft") {
		t.Fatalf("page did not serialize user slug:\n%s", string(bytes))
	}
	loaded := &Page{URI: page.URI}
	if err := LoadPage(website, loaded, false, io.Discard); err != nil {
		t.Fatalf("LoadPage() error = %v", err)
	}
	if loaded.Slug != "custom-draft" {
		t.Fatalf("loaded Slug = %q, want custom-draft", loaded.Slug)
	}
}

func TestTimestampDraftURISuffixUsesUnixMillisecondsHex(t *testing.T) {
	got := timestampDraftURISuffix(time.UnixMilli(0x19ac0ffee0))
	if got != "d019ac0ffee0" {
		t.Fatalf("timestampDraftURISuffix() = %q, want d019ac0ffee0", got)
	}
}

func TestUpdateDraftPageWithEmptySlugRehomesTemporaryURIForContentTypeChange(t *testing.T) {
	website, _ := testWebsite(t)
	sourcePath := filepath.Join(website.ContentDirPath(), "posts", "draft-fixed", "index.md")
	writePageTestFile(t, sourcePath, "---\ncreated_at: 2026-05-20T00:00:00Z\nlayout: default\ndraft: true\n---\n\nold")
	contenttype := testPageContentTypeWithURIResolver("json", "notes", TitleKabobURIResolver)
	page := &Page{URI: &url.URL{Path: sourcePath}, Draft: true, Content: "updated"}

	if err := UpdatePage(website, page, contenttype, io.Discard); err != nil {
		t.Fatalf("UpdatePage() error = %v", err)
	}
	destinationPath := filepath.Join(website.ContentDirPath(), "notes", "draft-fixed", "index.json")
	if page.URI == nil || page.URI.Path != destinationPath {
		t.Fatalf("page.URI = %#v, want %s", page.URI, destinationPath)
	}
	if page.Slug != "" {
		t.Fatalf("page.Slug = %q, want empty", page.Slug)
	}
	if page.Path != "/notes/draft-fixed/" {
		t.Fatalf("page.Path = %q, want /notes/draft-fixed/", page.Path)
	}
	if _, err := os.Stat(destinationPath); err != nil {
		t.Fatalf("destination page missing: %v", err)
	}
	if _, err := os.Stat(sourcePath); !os.IsNotExist(err) {
		t.Fatalf("source page still exists or unexpected error = %v", err)
	}
	bytes, err := os.ReadFile(destinationPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if strings.Contains(string(bytes), "\"slug\"") {
		t.Fatalf("draft page serialized temporary slug:\n%s", string(bytes))
	}
}

func TestUpdateDraftPageWithEmptySlugPreservesTimestampTemporaryURI(t *testing.T) {
	website, _ := testWebsite(t)
	sourcePath := filepath.Join(website.ContentDirPath(), "posts", "d019ac0ffee0", "index.md")
	writePageTestFile(t, sourcePath, "---\ncreated_at: 2026-05-20T00:00:00Z\nlayout: default\ndraft: true\n---\n\nold")
	contenttype := testPageContentTypeWithURIResolver("json", "notes", TitleKabobURIResolver)
	page := &Page{URI: &url.URL{Path: sourcePath}, Draft: true, Content: "updated"}

	if err := UpdatePage(website, page, contenttype, io.Discard); err != nil {
		t.Fatalf("UpdatePage() error = %v", err)
	}
	destinationPath := filepath.Join(website.ContentDirPath(), "notes", "d019ac0ffee0", "index.json")
	if page.URI == nil || page.URI.Path != destinationPath {
		t.Fatalf("page.URI = %#v, want %s", page.URI, destinationPath)
	}
	if page.Path != "/notes/d019ac0ffee0/" {
		t.Fatalf("page.Path = %q, want /notes/d019ac0ffee0/", page.Path)
	}
	if _, err := os.Stat(destinationPath); err != nil {
		t.Fatalf("destination page missing: %v", err)
	}
}

func TestUpdateDraftPageWithTitleRehomesToTitleSlug(t *testing.T) {
	website, _ := testWebsite(t)
	sourcePath := filepath.Join(website.ContentDirPath(), "posts", "draft-fixed", "index.md")
	writePageTestFile(t, sourcePath, "---\ncreated_at: 2026-05-20T00:00:00Z\nlayout: default\ndraft: true\n---\n\nold")
	contenttype := testPageContentTypeWithURIResolver("markdown", "posts", TitleKabobURIResolver)
	page := &Page{URI: &url.URL{Path: sourcePath}, Draft: true, Title: "Final Draft", Content: "updated"}

	if err := UpdatePage(website, page, contenttype, io.Discard); err != nil {
		t.Fatalf("UpdatePage() error = %v", err)
	}
	destinationPath := filepath.Join(website.ContentDirPath(), "posts", "final-draft", "index.md")
	if page.URI == nil || page.URI.Path != destinationPath {
		t.Fatalf("page.URI = %#v, want %s", page.URI, destinationPath)
	}
	if page.Slug != "" {
		t.Fatalf("page.Slug = %q, want empty", page.Slug)
	}
	if page.Path != "/posts/final-draft/" {
		t.Fatalf("page.Path = %q, want /posts/final-draft/", page.Path)
	}
	bytes, err := os.ReadFile(destinationPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if strings.Contains(string(bytes), "slug:") {
		t.Fatalf("title-derived draft page serialized generated slug:\n%s", string(bytes))
	}
}

func TestPublishUpdateFromTemporaryDraftToTitleSlug(t *testing.T) {
	website, _ := testWebsite(t)
	sourcePath := filepath.Join(website.ContentDirPath(), "posts", "draft-fixed", "index.md")
	writePageTestFile(t, sourcePath, "---\ncreated_at: 2026-05-20T00:00:00Z\nlayout: default\ndraft: true\n---\n\nold")
	contenttype := testPageContentTypeWithURIResolver("markdown", "posts", TitleKabobURIResolver)
	page := &Page{URI: &url.URL{Path: sourcePath}, Title: "Published Draft", Content: "published"}

	if err := UpdatePage(website, page, contenttype, io.Discard); err != nil {
		t.Fatalf("UpdatePage() error = %v", err)
	}
	destinationPath := filepath.Join(website.ContentDirPath(), "posts", "published-draft", "index.md")
	if page.URI == nil || page.URI.Path != destinationPath {
		t.Fatalf("page.URI = %#v, want %s", page.URI, destinationPath)
	}
	if page.Slug != "" {
		t.Fatalf("page.Slug = %q, want empty", page.Slug)
	}
	if page.Path != "/posts/published-draft/" {
		t.Fatalf("page.Path = %q, want /posts/published-draft/", page.Path)
	}
}

func TestPublishUpdateFailsWhenTitleSlugInputMissing(t *testing.T) {
	tests := []struct {
		name     string
		resolver PageURIResolverName
	}{
		{name: "title kabob", resolver: TitleKabobURIResolver},
		{name: "title snake", resolver: TitleSnakeURIResolver},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			website, _ := testWebsite(t)
			sourcePath := filepath.Join(website.ContentDirPath(), "posts", "draft-fixed", "index.md")
			writePageTestFile(t, sourcePath, "---\ncreated_at: 2026-05-20T00:00:00Z\nlayout: default\ndraft: true\n---\n\nold")
			contenttype := testPageContentTypeWithURIResolver("markdown", "posts", tc.resolver)
			page := &Page{URI: &url.URL{Path: sourcePath}, Content: "published"}

			err := UpdatePage(website, page, contenttype, io.Discard)
			if err == nil || !strings.Contains(err.Error(), "content-type requires page.title property") {
				t.Fatalf("UpdatePage() error = %v, want title validation", err)
			}
			if _, statErr := os.Stat(sourcePath); statErr != nil {
				t.Fatalf("source page should remain after validation failure: %v", statErr)
			}
		})
	}
}

func TestPublishUpdateForManualURIResolverValidation(t *testing.T) {
	t.Run("fails without slug", func(t *testing.T) {
		website, _ := testWebsite(t)
		sourcePath := filepath.Join(website.ContentDirPath(), "posts", "draft-fixed", "index.md")
		writePageTestFile(t, sourcePath, "---\ncreated_at: 2026-05-20T00:00:00Z\nlayout: default\ndraft: true\n---\n\nold")
		contenttype := testPageContentTypeWithURIResolver("markdown", "posts", ManualURIResolver)
		page := &Page{URI: &url.URL{Path: sourcePath}, Content: "published"}

		err := UpdatePage(website, page, contenttype, io.Discard)
		if err == nil || !strings.Contains(err.Error(), "content-type requires page.slug property") {
			t.Fatalf("UpdatePage() error = %v, want slug validation", err)
		}
		if _, statErr := os.Stat(sourcePath); statErr != nil {
			t.Fatalf("source page should remain after validation failure: %v", statErr)
		}
	})

	t.Run("succeeds with slug", func(t *testing.T) {
		website, _ := testWebsite(t)
		sourcePath := filepath.Join(website.ContentDirPath(), "posts", "draft-fixed", "index.md")
		writePageTestFile(t, sourcePath, "---\ncreated_at: 2026-05-20T00:00:00Z\nlayout: default\ndraft: true\n---\n\nold")
		contenttype := testPageContentTypeWithURIResolver("markdown", "posts", ManualURIResolver)
		page := &Page{URI: &url.URL{Path: sourcePath}, Slug: "manual-page", Content: "published"}

		if err := UpdatePage(website, page, contenttype, io.Discard); err != nil {
			t.Fatalf("UpdatePage() error = %v", err)
		}
		destinationPath := filepath.Join(website.ContentDirPath(), "posts", "manual-page", "index.md")
		if page.URI == nil || page.URI.Path != destinationPath {
			t.Fatalf("page.URI = %#v, want %s", page.URI, destinationPath)
		}
		if page.Slug != "manual-page" {
			t.Fatalf("page.Slug = %q, want manual-page", page.Slug)
		}
		bytes, err := os.ReadFile(destinationPath)
		if err != nil {
			t.Fatalf("ReadFile() error = %v", err)
		}
		if !strings.Contains(string(bytes), "slug: manual-page") {
			t.Fatalf("page did not serialize manual slug:\n%s", string(bytes))
		}
	})
}

func TestUpdatePageSameDestinationWritesInPlace(t *testing.T) {
	website, _ := testWebsite(t)
	sourcePath := filepath.Join(website.ContentDirPath(), "posts", "same", "index.md")
	writePageTestFile(t, sourcePath, "---\ncreated_at: 2026-05-20T00:00:00Z\nlayout: default\nslug: same\n---\n\nold")
	contenttype := testPageContentTypeWithURIResolver("markdown", "posts", ManualURIResolver)
	page := &Page{URI: &url.URL{Path: sourcePath}, Slug: "same", Content: "updated in place"}

	if err := UpdatePage(website, page, contenttype, io.Discard); err != nil {
		t.Fatalf("UpdatePage() error = %v", err)
	}
	if page.URI == nil || page.URI.Path != sourcePath {
		t.Fatalf("page.URI = %#v, want unchanged source path", page.URI)
	}
	if page.Path != "/posts/same/" {
		t.Fatalf("page.Path = %q, want /posts/same/", page.Path)
	}
	bytes, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(bytes), "updated in place") {
		t.Fatalf("updated page contents = %q, want updated content", string(bytes))
	}
}

func TestUpdatePageAutoContentTypeWritesMarkdownInPlace(t *testing.T) {
	website, _ := testWebsite(t)
	sourcePath := filepath.Join(website.ContentDirPath(), "posts", "draft-fixed", "index.md")
	writePageTestFile(t, sourcePath, "---\ncreated_at: 2026-05-20T00:00:00Z\nlayout: default\n---\n\nold")
	emptyContentType := &ContentType{Spec: ContentTypeSpecification{Layout: "default", URIResolver: TimestampURIResolver, Format: "markdown"}}
	page := &Page{
		URI:       &url.URL{Path: sourcePath},
		Slug:      "renamed",
		LayoutRef: "default",
		Title:     "Renamed",
		Content:   "updated in place",
	}

	if err := UpdatePage(website, page, emptyContentType, io.Discard); err != nil {
		t.Fatalf("UpdatePage() error = %v", err)
	}
	if page.URI == nil || page.URI.Path != sourcePath {
		t.Fatalf("page.URI = %#v, want unchanged source path", page.URI)
	}
	if page.Path != "/posts/draft-fixed/" {
		t.Fatalf("page.Path = %q, want /posts/draft-fixed/", page.Path)
	}
	if page.CreatedAt != "2026-05-20T00:00:00Z" {
		t.Fatalf("page.CreatedAt = %q, want preserved source created_at", page.CreatedAt)
	}
	bytes, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	contents := string(bytes)
	if !strings.Contains(contents, "updated in place") {
		t.Fatalf("updated page contents = %q, want updated content", contents)
	}
	if _, err := os.Stat(filepath.Join(website.ContentDirPath(), "renamed", "index.md")); !os.IsNotExist(err) {
		t.Fatalf("auto update rehomed by slug or unexpected error = %v", err)
	}
}

func TestUpdatePageAutoContentTypeWritesStructuredFormatsInPlace(t *testing.T) {
	tests := []struct {
		name        string
		relPath     string
		oldContents string
		want        string
	}{
		{
			name:        "json",
			relPath:     filepath.Join("notes", "same", "index.json"),
			oldContents: `{"created_at":"2026-05-20T00:00:00Z","layout":"default","content":"old"}`,
			want:        `"content":"updated json"`,
		},
		{
			name:        "yaml",
			relPath:     filepath.Join("notes", "same", "index.yaml"),
			oldContents: "created_at: 2026-05-20T00:00:00Z\nlayout: default\ncontent: old\n",
			want:        "content: updated yaml",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			website, _ := testWebsite(t)
			sourcePath := filepath.Join(website.ContentDirPath(), tc.relPath)
			writePageTestFile(t, sourcePath, tc.oldContents)
			page := &Page{
				URI:       &url.URL{Path: sourcePath},
				Slug:      "renamed",
				LayoutRef: "default",
				Content:   "updated " + tc.name,
			}

			if err := UpdatePage(website, page, nil, io.Discard); err != nil {
				t.Fatalf("UpdatePage() error = %v", err)
			}
			if page.URI == nil || page.URI.Path != sourcePath {
				t.Fatalf("page.URI = %#v, want unchanged source path", page.URI)
			}
			if page.Path != "/notes/same/" {
				t.Fatalf("page.Path = %q, want /notes/same/", page.Path)
			}
			if page.CreatedAt != "2026-05-20T00:00:00Z" {
				t.Fatalf("page.CreatedAt = %q, want preserved source created_at", page.CreatedAt)
			}
			bytes, err := os.ReadFile(sourcePath)
			if err != nil {
				t.Fatalf("ReadFile() error = %v", err)
			}
			if !strings.Contains(string(bytes), tc.want) {
				t.Fatalf("updated page contents = %q, want %q", string(bytes), tc.want)
			}
		})
	}
}

func TestUpdatePageAutoContentTypeFailsForUnsupportedSourceFormat(t *testing.T) {
	website, _ := testWebsite(t)
	sourcePath := filepath.Join(website.ContentDirPath(), "notes", "same", "index.txt")
	writePageTestFile(t, sourcePath, "old")
	page := &Page{URI: &url.URL{Path: sourcePath}, CreatedAt: "2026-05-20T00:00:00Z", Content: "updated"}

	err := UpdatePage(website, page, nil, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "auto content type does not support source file extension") {
		t.Fatalf("UpdatePage() error = %v, want unsupported extension", err)
	}
	if _, statErr := os.Stat(sourcePath); statErr != nil {
		t.Fatalf("source page should remain after unsupported extension: %v", statErr)
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

	if err := UpdatePage(website, page, contenttype, io.Discard); err != nil {
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

	if err := UpdatePage(website, page, contenttype, io.Discard); err != nil {
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

	err := UpdatePage(website, page, contenttype, io.Discard)
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

	err := UpdatePage(website, page, contenttype, io.Discard)
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

	if err := UpdatePage(website, page, contenttype, io.Discard); err != nil {
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
	build := &Build{
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
