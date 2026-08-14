package htxt

import (
	"io"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/herdworks/httt"
	"github.com/herdworks/htxt/href"
	htpatch "github.com/herdworks/htxt/patch"
	htprompt "github.com/herdworks/htxt/prompt"
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

func mustHref(t *testing.T, raw string) *href.Href {
	t.Helper()
	h, err := href.Parse(raw)
	if err != nil {
		t.Fatalf("href.Parse(%q) error = %v", raw, err)
	}
	return h
}

func TestPagesFilterDraft(t *testing.T) {
	published := &Page{Title: "Published"}
	draft := &Page{Title: "Draft", Visibility: DraftVisibility}
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

func TestPagesColumnValuesIncludesVisibility(t *testing.T) {
	pages := Pages{
		&Page{Visibility: AutoVisibility},
		&Page{Visibility: DraftVisibility},
	}
	if got, want := pages.ColumnValues("visibility"), []string{"public", "draft"}; !slices.Equal(got, want) {
		t.Fatalf("ColumnValues(visibility) = %v, want %v", got, want)
	}
}

func TestPagesAddPreservesOrderAndUniqueURIs(t *testing.T) {
	first := &Page{URI: &url.URL{Path: "/content/first/index.md"}}
	duplicate := &Page{URI: &url.URL{Path: "/content/first/index.md"}}
	second := &Page{URI: &url.URL{Path: "/content/second/index.md"}}

	got := Pages{first}.Add(nil, duplicate, second, first)
	if len(got) != 2 || got[0] != first || got[1] != second {
		t.Fatalf("Pages.Add() = %#v, want first then second", got)
	}
}

func TestPageVisibilityValidate(t *testing.T) {
	for _, visibility := range SupportedVisibilityKinds {
		if err := visibility.Validate(); err != nil {
			t.Errorf("%q.Validate() error = %v", visibility, err)
		}
	}
	if err := PageVisibilityKind("private").Validate(); err == nil {
		t.Fatal("invalid visibility Validate() error = nil")
	}
}

func TestValidateTagName(t *testing.T) {
	for _, name := range []string{"foo", "foo/bar", ".foo", "foo-bar"} {
		if err := ValidateTagName(name); err != nil {
			t.Errorf("ValidateTagName(%q) error = %v", name, err)
		}
	}
	for _, name := range []string{"", ".", "..", "/foo", "foo/", "foo//bar", "foo\\bar", "foo/../bar"} {
		if err := ValidateTagName(name); err == nil {
			t.Errorf("ValidateTagName(%q) error = nil", name)
		}
	}
}

func TestFeedDataValidateRejectsInvalidTagName(t *testing.T) {
	data := FeedData{Tag: "foo//bar"}
	if _, err := data.Validate(&Website{}); err == nil {
		t.Fatal("FeedData.Validate() accepted an invalid tag name")
	}
}

func TestPageVisibilityAliases(t *testing.T) {
	for _, visibility := range []PageVisibilityKind{
		StatusVisibility, FeedVisibility, StoryVisibility, ChatVisibility, MessageVisibility, ResponseVisibility, ReplyVisibility,
	} {
		if !visibility.IsStatus() {
			t.Errorf("%q.IsStatus() = false, want true", visibility)
		}
	}
	if !PreviewVisibility.IsDraft() {
		t.Fatal("preview visibility is not a draft alias")
	}
}

func TestLoadPageValidatesVisibilityAndAppliesStatusLayout(t *testing.T) {
	website, _ := testWebsite(t)
	website.Config.StatusLayout = "status"
	sourcePath := filepath.Join(website.ContentDirPath(), "status", "index.md")
	source := "---\nlayout: article\nvisibility: story\n---\n\nstatus"
	writePageTestFile(t, sourcePath, source)

	page := &Page{URI: &url.URL{Path: sourcePath}}
	if err := LoadPage(website, page, false, io.Discard); err != nil {
		t.Fatalf("LoadPage() error = %v", err)
	}
	if page.Visibility != StoryVisibility {
		t.Fatalf("page.Visibility = %q, want authored story alias", page.Visibility)
	}
	if page.LayoutRef != "status" {
		t.Fatalf("page.LayoutRef = %q, want status", page.LayoutRef)
	}
	page.ResolveTemplateData(website)
	if got := page.TemplateData.Get("layout"); got != "status" {
		t.Fatalf("page template layout = %#v, want status", got)
	}
	bytes, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(bytes) != source {
		t.Fatalf("status source changed:\n%s", bytes)
	}

	invalidPath := filepath.Join(website.ContentDirPath(), "invalid", "index.md")
	writePageTestFile(t, invalidPath, "---\nvisibility: private\n---\n\ninvalid")
	if err := LoadPage(website, &Page{URI: &url.URL{Path: invalidPath}}, false, io.Discard); err == nil {
		t.Fatal("LoadPage() accepted invalid visibility")
	}
}

func TestLoadPageRejectsInvalidTagName(t *testing.T) {
	website, _ := testWebsite(t)
	sourcePath := filepath.Join(website.ContentDirPath(), "invalid-tag", "index.md")
	writePageTestFile(t, sourcePath, "---\ntags:\n  - foo//bar\n---\n\ninvalid")

	page := &Page{URI: &url.URL{Path: sourcePath}}
	if err := LoadPage(website, page, false, io.Discard); err == nil {
		t.Fatal("LoadPage() accepted an invalid tag name")
	}
}

func TestContentTypeResolvePageURIUsesContentDirForDraftPages(t *testing.T) {
	website, _ := testWebsite(t)
	contenttype := &ContentType{Metadata: ContentTypeMetadata{Name: "Test"}, Spec: ContentTypeSpecification{
		Layout:      "default",
		URIResolver: ManualURIResolver,
		Format:      "markdown",
		Path:        "posts",
	}}
	page := &Page{Visibility: DraftVisibility, Slug: "draft-post"}

	uri, err := contenttype.ResolvePageURI(website, page, nil)
	if err != nil {
		t.Fatalf("ContentType.ResolvePageURI() error = %v", err)
	}
	got := uri.Path
	want := filepath.Join(website.ContentDirPath(), "posts", "draft-post", "index.md")
	if got != want {
		t.Fatalf("ContentType.ResolvePageURI() = %q, want %q", got, want)
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

func TestPageAtomEntryUsesCanonicalURLForIDAndAlternateLink(t *testing.T) {
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
	if got, want := links["alternate"], "https://example.com/about/"; got != want {
		t.Fatalf("alternate link = %q, want %q", got, want)
	}
	if _, ok := links["related"]; ok {
		t.Fatalf("unexpected related link = %q", links["related"])
	}
	if _, ok := links["via"]; ok {
		t.Fatalf("unexpected via link = %q", links["via"])
	}
}

func TestPageAtomEntryLinksRepresentAttachments(t *testing.T) {
	website, _ := testWebsite(t)
	pageDir := filepath.Join(website.ContentDirPath(), "s", "123")
	writeAssetTestFile(t, filepath.Join(pageDir, "photo.png"))
	writeAssetTestFile(t, filepath.Join(website.ContentDirPath(), "docs", "guide.pdf"))
	page := Page{
		URI:  &url.URL{Path: filepath.Join(pageDir, "index.md")},
		Path: "/s/123/",
		Attachments: Attachments{
			{Kind: AttachmentKindLink, Href: mustHref(t, "https://example.org/article"), Data: map[string]any{"hypergraph": map[string]any{"title": "Example Article"}}},
			{Kind: AttachmentKindImage, Href: mustHref(t, "photo.png")},
			{Kind: AttachmentKindLink, Href: mustHref(t, "https://example.net/second")},
			{Kind: AttachmentKindDocument, Href: mustHref(t, "/docs/guide.pdf")},
		},
	}

	entry := page.AtomEntry(website)
	if entry == nil {
		t.Fatal("AtomEntry() returned nil")
	}
	related := []string{}
	enclosures := map[string]struct {
		mimetype string
		length   uint
	}{}
	for _, link := range entry.Links {
		switch link.Rel {
		case "alternate":
			if got, want := link.Href, "https://example.com/s/123/"; got != want {
				t.Fatalf("alternate link = %q, want %q", got, want)
			}
		case "via":
			t.Fatalf("unexpected via link: %#v", link)
		case "related":
			related = append(related, link.Href)
			if link.Href == "https://example.org/article" && link.Title != "Example Article" {
				t.Fatalf("related title = %q, want Example Article", link.Title)
			}
		case "enclosure":
			enclosures[link.Href] = struct {
				mimetype string
				length   uint
			}{mimetype: link.Type, length: link.Length}
		}
	}
	if got, want := strings.Join(related, ","), "https://example.org/article,https://example.net/second"; got != want {
		t.Fatalf("related links = %q, want %q", got, want)
	}
	if got, ok := enclosures["https://example.com/s/123/photo.png"]; !ok {
		t.Fatal("missing photo enclosure")
	} else if got.mimetype != "image/png" || got.length != 5 {
		t.Fatalf("photo enclosure = %#v, want image/png length 5", got)
	}
	if got, ok := enclosures["https://example.com/docs/guide.pdf"]; !ok {
		t.Fatal("missing document enclosure")
	} else if got.mimetype != "application/pdf" || got.length != 0 {
		t.Fatalf("document enclosure = %#v, want application/pdf length 0", got)
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

func TestPageRenderDataRendersMarkdownFromClonedPage(t *testing.T) {
	website, _ := testWebsite(t)
	sourcePath := filepath.Join(website.ContentDirPath(), "preview", "index.md")
	page := &Page{
		URI:        &url.URL{Path: sourcePath},
		CreatedAt:  "2026-07-03T00:00:00Z",
		LayoutRef:  "default",
		Title:      "Preview Page",
		Content:    "Preview body.",
		Visibility: DraftVisibility,
		TemplateData: httt.TemplateData{
			"custom": "value",
		},
	}

	bytes, err := page.Clone().RenderData(website, "markdown")
	if err != nil {
		t.Fatalf("RenderData() error = %v", err)
	}
	contents := string(bytes)
	for _, want := range []string{"title: Preview Page", "visibility: draft", "custom: value", "Preview body."} {
		if !strings.Contains(contents, want) {
			t.Fatalf("rendered page missing %q:\n%s", want, contents)
		}
	}
	if got := page.TemplateData.Get("title"); got != nil {
		t.Fatalf("source template title = %v, want unset", got)
	}
	if got := page.TemplateData.Get("visibility"); got != nil {
		t.Fatalf("source template draft = %v, want unset", got)
	}
	if got := page.TemplateData.Get("custom"); got != "value" {
		t.Fatalf("source template custom = %v, want value", got)
	}
	if _, err := os.Stat(sourcePath); !os.IsNotExist(err) {
		t.Fatalf("RenderData wrote source path or unexpected stat error = %v", err)
	}
}

func TestContentTypePreviewPageDataAppliesOverlayWithoutWriting(t *testing.T) {
	website, _ := testWebsite(t)
	sourcePath := filepath.Join(website.ContentDirPath(), "drafts", "working", "index.md")
	contenttype := ContentType{
		Metadata: ContentTypeMetadata{Name: "Neighborhood Post"},
		Spec: ContentTypeSpecification{
			Layout:      "post.html",
			URIResolver: TitleKabobURIResolver,
			Format:      "markdown",
			Visibility:  UnlistedVisibility,
			TemplateData: httt.TemplateData{
				"neighborhood": "",
			},
			Prompts: htprompt.Prompts{
				{
					ID:   "neighborhood",
					Type: htprompt.TypeDropDown,
					Options: htprompt.Options{
						{Label: "Downtown", Value: "downtown"},
						{Label: "North Beach", Value: "north-beach"},
					},
				},
			},
			Patches: htpatch.Patches{
				{Op: htpatch.OperationReplace, Path: "page.neighborhood", ValueRef: "neighborhood"},
			},
		},
		State: ContentTypeState{UserData: map[string]any{"neighborhood": "north-beach"}},
	}
	page := &Page{
		URI:        &url.URL{Path: sourcePath},
		CreatedAt:  "2026-07-03T00:00:00Z",
		Title:      "Neighborhood Notes",
		Content:    "A few notes from the neighborhood.",
		Visibility: DraftVisibility,
	}

	bytes, err := contenttype.PreviewPageData(website, page)
	if err != nil {
		t.Fatalf("PreviewPageData() error = %v", err)
	}
	contents := string(bytes)
	for _, want := range []string{
		"layout: post.html",
		"title: Neighborhood Notes",
		"visibility: unlisted",
		"neighborhood: north-beach",
		"A few notes from the neighborhood.",
	} {
		if !strings.Contains(contents, want) {
			t.Fatalf("preview page data missing %q:\n%s", want, contents)
		}
	}
	if page.LayoutRef != "" {
		t.Fatalf("source LayoutRef = %q, want unchanged empty layout", page.LayoutRef)
	}
	if got := page.TemplateData.Get("neighborhood"); got != nil {
		t.Fatalf("source template neighborhood = %v, want unset", got)
	}
	if _, err := os.Stat(sourcePath); !os.IsNotExist(err) {
		t.Fatalf("PreviewPageData wrote source path or unexpected stat error = %v", err)
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

func TestCreatePageAppliesContentTypeUnlistedDefault(t *testing.T) {
	website, _ := testWebsite(t)
	contenttype := testPageContentType("markdown", "posts")
	contenttype.Spec.Visibility = UnlistedVisibility
	page := &Page{Visibility: DraftVisibility, Content: "draft content"}

	if err := CreatePage(website, page, contenttype, io.Discard); err != nil {
		t.Fatalf("CreatePage() error = %v", err)
	}
	if page.Visibility != UnlistedVisibility {
		t.Fatalf("page.Visibility = %q, want unlisted", page.Visibility)
	}
	bytes, err := os.ReadFile(page.URI.Path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(bytes), "visibility: unlisted") {
		t.Fatalf("created page missing visibility frontmatter:\n%s", bytes)
	}
}

func TestUpdatePageWithTemplatedContentTypePathUsesExistingCreatedAt(t *testing.T) {
	website, _ := testWebsite(t)
	sourcePath := filepath.Join(website.ContentDirPath(), "drafts", "working", "index.md")
	writePageTestFile(t, sourcePath, "---\ncreated_at: 2026-05-20T00:00:00Z\nlayout: default\nvisibility: draft\n---\n\nold")
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

func TestUpdatePageAppliesContentTypeUnlistedDefault(t *testing.T) {
	website, _ := testWebsite(t)
	sourcePath := filepath.Join(website.ContentDirPath(), "posts", "draft-fixed", "index.md")
	writePageTestFile(t, sourcePath, "---\ncreated_at: 2026-05-20T00:00:00Z\nlayout: default\nvisibility: draft\n---\n\nold")
	contenttype := testPageContentType("markdown", "posts")
	contenttype.Spec.Visibility = UnlistedVisibility
	page := &Page{URI: &url.URL{Path: sourcePath}, Visibility: DraftVisibility, Content: "updated draft"}

	if err := UpdatePage(website, page, contenttype, io.Discard); err != nil {
		t.Fatalf("UpdatePage() error = %v", err)
	}
	if page.Visibility != UnlistedVisibility {
		t.Fatalf("page.Visibility = %q, want unlisted", page.Visibility)
	}
	bytes, err := os.ReadFile(page.URI.Path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(bytes), "visibility: unlisted") {
		t.Fatalf("updated page missing visibility frontmatter:\n%s", bytes)
	}
}

func TestCreateDraftPageWithEmptySlugUsesTemporaryURI(t *testing.T) {
	website, _ := testWebsite(t)
	contenttype := testPageContentTypeWithURIResolver("markdown", "posts", TitleKabobURIResolver)
	page := &Page{Visibility: DraftVisibility, Content: "draft content"}

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
	page := &Page{Visibility: DraftVisibility, Slug: "custom-draft", Content: "draft content"}

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
	writePageTestFile(t, sourcePath, "---\ncreated_at: 2026-05-20T00:00:00Z\nlayout: default\nvisibility: draft\n---\n\nold")
	contenttype := testPageContentTypeWithURIResolver("json", "notes", TitleKabobURIResolver)
	page := &Page{URI: &url.URL{Path: sourcePath}, Visibility: DraftVisibility, Content: "updated"}

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
	writePageTestFile(t, sourcePath, "---\ncreated_at: 2026-05-20T00:00:00Z\nlayout: default\nvisibility: draft\n---\n\nold")
	contenttype := testPageContentTypeWithURIResolver("json", "notes", TitleKabobURIResolver)
	page := &Page{URI: &url.URL{Path: sourcePath}, Visibility: DraftVisibility, Content: "updated"}

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
	writePageTestFile(t, sourcePath, "---\ncreated_at: 2026-05-20T00:00:00Z\nlayout: default\nvisibility: draft\n---\n\nold")
	contenttype := testPageContentTypeWithURIResolver("markdown", "posts", TitleKabobURIResolver)
	page := &Page{URI: &url.URL{Path: sourcePath}, Visibility: DraftVisibility, Title: "Final Draft", Content: "updated"}

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
	writePageTestFile(t, sourcePath, "---\ncreated_at: 2026-05-20T00:00:00Z\nlayout: default\nvisibility: draft\n---\n\nold")
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
			writePageTestFile(t, sourcePath, "---\ncreated_at: 2026-05-20T00:00:00Z\nlayout: default\nvisibility: draft\n---\n\nold")
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
		writePageTestFile(t, sourcePath, "---\ncreated_at: 2026-05-20T00:00:00Z\nlayout: default\nvisibility: draft\n---\n\nold")
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
		writePageTestFile(t, sourcePath, "---\ncreated_at: 2026-05-20T00:00:00Z\nlayout: default\nvisibility: draft\n---\n\nold")
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
		URI:        &url.URL{Path: sourcePath},
		Slug:       "updated",
		CreatedAt:  "2026-05-20T00:00:00Z",
		LayoutRef:  "default",
		Title:      "Updated",
		Content:    "updated content",
		Visibility: DraftVisibility,
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
		URI:        &url.URL{Path: sourcePath},
		Slug:       "home",
		CreatedAt:  "2026-05-20T00:00:00Z",
		LayoutRef:  "default",
		Title:      "Home",
		Content:    "home content",
		Visibility: DraftVisibility,
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
	page := &Page{URI: &url.URL{Path: sourcePath}, Slug: "taken", CreatedAt: "2026-05-20T00:00:00Z", Content: "new", Visibility: DraftVisibility}

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
	page := &Page{URI: &url.URL{Path: sourcePath}, Slug: "updated", CreatedAt: "2026-05-20T00:00:00Z", Content: "new", Visibility: DraftVisibility}

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
	page := &Page{URI: &url.URL{Path: sourcePath}, Slug: "updated", CreatedAt: "2026-05-20T00:00:00Z", Title: "Updated", Content: "json content", Visibility: DraftVisibility}

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

func TestCreatePageDoesNotMaterializeContentTypeTemplateOrPatches(t *testing.T) {
	website, _ := testWebsite(t)
	contenttype := &ContentType{
		Metadata: ContentTypeMetadata{Name: "Draft"},
		Spec: ContentTypeSpecification{
			Layout:      "default",
			Path:        "posts",
			URIResolver: TitleKabobURIResolver,
			Format:      "markdown",
			TemplateData: httt.TemplateData{
				"summary": "Spec summary",
			},
			Prompts: htprompt.Prompts{
				{ID: "title", Type: htprompt.TypeText},
				{ID: "featured", Type: htprompt.TypeBoolean},
			},
			Patches: htpatch.Patches{
				{Op: "add", Path: "page.title", ValueRef: "title"},
				{Op: "add", Path: "page.featured", ValueRef: "featured"},
			},
		},
		State: ContentTypeState{UserData: map[string]any{"title": "Patched Title", "featured": true}},
	}
	page := &Page{Visibility: DraftVisibility, Content: "draft content"}

	if err := CreatePage(website, page, contenttype, io.Discard); err != nil {
		t.Fatalf("CreatePage() error = %v", err)
	}
	if page.URI == nil {
		t.Fatal("page.URI is nil")
	}
	pageDir := filepath.Dir(page.URI.Path)
	draftSegment := filepath.Base(pageDir)
	if filepath.Dir(pageDir) != filepath.Join(website.ContentDirPath(), "posts") {
		t.Fatalf("page.URI = %s, want autosave under posts", page.URI.Path)
	}
	assertTimestampDraftSegment(t, draftSegment)
	bytes, err := os.ReadFile(page.URI.Path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	contents := string(bytes)
	for _, unexpected := range []string{"Patched Title", "Spec summary", "featured: true"} {
		if strings.Contains(contents, unexpected) {
			t.Fatalf("autosave materialized %q:\n%s", unexpected, contents)
		}
	}
	if !strings.Contains(contents, "draft content") {
		t.Fatalf("page file missing draft content:\n%s", contents)
	}
}

func TestUpdatePageDoesNotMaterializeContentTypeTemplateOrPatches(t *testing.T) {
	website, _ := testWebsite(t)
	sourcePath := filepath.Join(website.ContentDirPath(), "posts", "draft-fixed", "index.md")
	writePageTestFile(t, sourcePath, "---\ncreated_at: 2026-05-20T00:00:00Z\nlayout: default\nvisibility: draft\n---\n\nold")
	contenttype := &ContentType{
		Metadata: ContentTypeMetadata{Name: "Draft"},
		Spec: ContentTypeSpecification{
			Layout:      "default",
			Path:        "notes",
			URIResolver: TitleKabobURIResolver,
			Format:      "json",
			TemplateData: httt.TemplateData{
				"summary": "Spec summary",
			},
			Prompts: htprompt.Prompts{
				{ID: "title", Type: htprompt.TypeText},
				{ID: "featured", Type: htprompt.TypeBoolean},
			},
			Patches: htpatch.Patches{
				{Op: "add", Path: "page.title", ValueRef: "title"},
				{Op: "add", Path: "page.featured", ValueRef: "featured"},
			},
		},
		State: ContentTypeState{UserData: map[string]any{"title": "Patched Title", "featured": true}},
	}
	page := &Page{URI: &url.URL{Path: sourcePath}, Visibility: DraftVisibility, Content: "updated draft"}

	if err := UpdatePage(website, page, contenttype, io.Discard); err != nil {
		t.Fatalf("UpdatePage() error = %v", err)
	}
	wantPath := filepath.Join(website.ContentDirPath(), "notes", "draft-fixed", "index.json")
	if page.URI == nil || page.URI.Path != wantPath {
		t.Fatalf("page.URI = %#v, want %s", page.URI, wantPath)
	}
	bytes, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	contents := string(bytes)
	for _, unexpected := range []string{"Patched Title", "Spec summary", "featured"} {
		if strings.Contains(contents, unexpected) {
			t.Fatalf("update materialized %q:\n%s", unexpected, contents)
		}
	}
	if !strings.Contains(contents, "updated draft") {
		t.Fatalf("page file missing updated draft content:\n%s", contents)
	}
}

func TestFinalizePageMaterializesContentTypeOverlayAndClearsDraft(t *testing.T) {
	website, _ := testWebsite(t)
	sourcePath := filepath.Join(website.ContentDirPath(), "posts", "draft-fixed", "index.md")
	writePageTestFile(t, sourcePath, "---\ncreated_at: 2026-05-20T00:00:00Z\nlayout: default\nvisibility: draft\n---\n\nold")
	contenttype := &ContentType{
		Metadata: ContentTypeMetadata{Name: "Post"},
		Spec: ContentTypeSpecification{
			Layout:      "default",
			Path:        "posts",
			URIResolver: TitleKabobURIResolver,
			Format:      "markdown",
			Visibility:  UnlistedVisibility,
			TemplateData: httt.TemplateData{
				"summary": "Spec summary",
				"tags":    []string{"spec"},
			},
			Prompts: htprompt.Prompts{
				{ID: "title", Type: htprompt.TypeText},
				{ID: "tag", Type: htprompt.TypeText},
				{ID: "featured", Type: htprompt.TypeBoolean},
			},
			Patches: htpatch.Patches{
				{Op: "add", Path: "page.title", ValueRef: "title"},
				{Op: "append", Path: "page.tags", ValueRef: "tag"},
				{Op: "add", Path: "page.featured", ValueRef: "featured"},
			},
		},
		State: ContentTypeState{UserData: map[string]any{
			"title":    "Final Title",
			"tag":      "prompt-tag",
			"featured": true,
		}},
	}
	page := &Page{
		URI:          &url.URL{Path: sourcePath},
		Visibility:   DraftVisibility,
		Content:      "final content",
		TemplateData: httt.TemplateData{"visibility": "draft"},
	}

	if err := FinalizePage(website, page, contenttype, io.Discard); err != nil {
		t.Fatalf("FinalizePage() error = %v", err)
	}
	wantPath := filepath.Join(website.ContentDirPath(), "posts", "final-title", "index.md")
	if page.URI == nil || page.URI.Path != wantPath {
		t.Fatalf("page.URI = %#v, want %s", page.URI, wantPath)
	}
	if page.Visibility != UnlistedVisibility {
		t.Fatalf("page.Visibility = %q, want unlisted", page.Visibility)
	}
	if _, err := os.Stat(sourcePath); !os.IsNotExist(err) {
		t.Fatalf("source page still exists or unexpected error = %v", err)
	}
	bytes, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	contents := string(bytes)
	for _, want := range []string{"title: Final Title", "summary: Spec summary", "featured: true", "visibility: unlisted", "prompt-tag", "final content"} {
		if !strings.Contains(contents, want) {
			t.Fatalf("final page missing %q:\n%s", want, contents)
		}
	}
	if strings.Contains(contents, "visibility: draft") || strings.Contains(contents, "content_type") {
		t.Fatalf("final page retained composer-only state:\n%s", contents)
	}
}

func TestFinalizePageWithoutContentTypeFinalizesInPlaceAndClearsDraft(t *testing.T) {
	website, _ := testWebsite(t)
	sourcePath := filepath.Join(website.ContentDirPath(), "freeform", "draft-fixed", "index.md")
	writePageTestFile(t, sourcePath, "---\ncreated_at: 2026-05-20T00:00:00Z\nlayout: default\nvisibility: draft\n---\n\nold")
	page := &Page{
		URI:          &url.URL{Path: sourcePath},
		Visibility:   DraftVisibility,
		LayoutRef:    "default",
		Title:        "Freeform",
		Content:      "freeform content",
		TemplateData: httt.TemplateData{"visibility": "draft", "title": "Freeform"},
	}

	if err := FinalizePage(website, page, nil, io.Discard); err != nil {
		t.Fatalf("FinalizePage() error = %v", err)
	}
	if page.URI == nil || page.URI.Path != sourcePath {
		t.Fatalf("page.URI = %#v, want unchanged source path", page.URI)
	}
	if page.Visibility != PublicVisibility {
		t.Fatalf("page.Visibility = %q, want public", page.Visibility)
	}
	bytes, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	contents := string(bytes)
	if strings.Contains(contents, "visibility: draft") {
		t.Fatalf("finalized freeform page retained draft state:\n%s", contents)
	}
	for _, want := range []string{"title: Freeform", "freeform content"} {
		if !strings.Contains(contents, want) {
			t.Fatalf("finalized freeform page missing %q:\n%s", want, contents)
		}
	}
}

func TestBuildPathWithRelativeWebsiteRoot(t *testing.T) {
	sourcePath, err := filepath.Abs(filepath.Join("testdata", "site", "content", "about", "index.html"))
	if err != nil {
		t.Fatalf("Abs() error = %v", err)
	}
	website := &Website{URI: &href.Href{URL: url.URL{Path: filepath.Join("testdata", "site", "site.yaml")}}}
	build := &Build{
		Config:  BuildConfig{URI: &url.URL{Path: filepath.Join("testdata", "site", ".builds", "test")}},
		Website: website,
	}
	page := &Page{URI: &url.URL{Path: sourcePath}, Path: "/about/"}

	got := build.BuildPath(page)
	want, err := filepath.Abs(filepath.Join("testdata", "site", ".builds", "test", "about", "index.html"))
	if err != nil {
		t.Fatalf("Abs() error = %v", err)
	}
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
			want: filepath.Join(build.Config.URI.Path, "about", "index.html"),
		},
		{
			name: "ugly url",
			path: "/404.html",
			want: filepath.Join(build.Config.URI.Path, "404.html"),
		},
		{
			name: "nested ugly url",
			path: "/posts/hello.html",
			want: filepath.Join(build.Config.URI.Path, "posts", "hello.html"),
		},
		{
			name: "root path",
			path: "/",
			want: filepath.Join(build.Config.URI.Path, "index.html"),
		},
		{
			name: "path without trailing slash",
			path: "/tags/go",
			want: filepath.Join(build.Config.URI.Path, "tags", "go", "index.html"),
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
