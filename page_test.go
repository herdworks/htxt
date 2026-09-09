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

func TestPageSourceURIDerivesSourceState(t *testing.T) {
	page := &Page{
		Source: PageSource{
			Directory: "/website/drafts",
			Path:      "posts/hello",
			Filename:  "index.md",
		},
	}

	if got, want := page.Source.URI().Path, "/website/drafts/posts/hello/index.md"; got != want {
		t.Fatalf("Page.Source.URI().Path = %q, want %q", got, want)
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
	first := pageWithURI(&url.URL{Path: "/content/first/index.md"}, Page{})
	duplicate := pageWithURI(&url.URL{Path: "/content/first/index.md"}, Page{})
	second := pageWithURI(&url.URL{Path: "/content/second/index.md"}, Page{})

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

	page := pageWithURI(&url.URL{Path: sourcePath}, Page{})
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
	if err := LoadPage(website, pageWithURI(&url.URL{Path: invalidPath}, Page{}), false, io.Discard); err == nil {
		t.Fatal("LoadPage() accepted invalid visibility")
	}
}

func TestLoadPageRejectsInvalidTagName(t *testing.T) {
	website, _ := testWebsite(t)
	sourcePath := filepath.Join(website.ContentDirPath(), "invalid-tag", "index.md")
	writePageTestFile(t, sourcePath, "---\ntags:\n  - foo//bar\n---\n\ninvalid")

	page := pageWithURI(&url.URL{Path: sourcePath}, Page{})
	if err := LoadPage(website, page, false, io.Discard); err == nil {
		t.Fatal("LoadPage() accepted an invalid tag name")
	}
}

func TestPageUpdateResolvesDraftPublicURLPathWithoutMutatingSource(t *testing.T) {
	website, _ := testWebsite(t)
	contenttype := &ContentType{Metadata: ContentTypeMetadata{Name: "Test"}, Spec: ContentTypeSpecification{
		Layout:       "default",
		PathResolver: ManualPathResolver,
		Format:       "markdown",
		Visibility:   PublicVisibility,
		Path:         "posts",
	}}
	page := &Page{Source: PageSource{Directory: website.ContentDirPath()}, Visibility: DraftVisibility, Slug: "draft-post"}

	if err := page.Update(website, contenttype); err != nil {
		t.Fatalf("Page.Update() error = %v", err)
	}
	if page.PublicURL.Path != "/posts/draft-post/" {
		t.Fatalf("page.Path = %q, want /posts/draft-post/", page.PublicURL.Path)
	}
	if page.Source.Path != "" || page.Source.Filename != "" {
		t.Fatalf("path resolution mutated source state: %#v", page)
	}
}

func TestPageUpdateReplacesVisibilityFromContentType(t *testing.T) {
	website, _ := testWebsite(t)
	contenttype := testPageContentType("markdown", "posts")
	contenttype.Spec.PathResolver = ManualPathResolver
	contenttype.Spec.Visibility = PublicVisibility
	page := &Page{
		Source:     PageSource{Directory: website.ContentDirPath()},
		Slug:       "published-post",
		Visibility: StatusVisibility,
	}

	if err := page.Update(website, contenttype); err != nil {
		t.Fatalf("Page.Update() error = %v", err)
	}
	if page.Visibility != PublicVisibility {
		t.Fatalf("page.Visibility = %q, want %q", page.Visibility, PublicVisibility)
	}
}

func TestLoadPagePreservesProjectedPath(t *testing.T) {
	website, _ := testWebsite(t)
	sourcePath := filepath.Join(website.ContentDirPath(), "posts", "source", "index.md")
	writePageTestFile(t, sourcePath, "---\nlayout: default\n---\n\nsource")

	page := pageWithURI(&url.URL{Path: sourcePath}, Page{PublicURL: url.URL{Path: "/projected/"}})
	if err := LoadPage(website, page, false, io.Discard); err != nil {
		t.Fatalf("LoadPage() error = %v", err)
	}
	if page.PublicURL.Path != "/projected/" {
		t.Fatalf("page.Path = %q, want caller projection", page.PublicURL.Path)
	}
}

func TestPageResolvedPath(t *testing.T) {
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
			name:    "ugly root page becomes index file",
			source:  filepath.Join(website.ContentDirPath(), "index.md"),
			uglyURL: true,
			want:    "/index.html",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			page := pageWithURI(&url.URL{Path: tc.source}, Page{UglyURL: tc.uglyURL})
			if got := page.Source.PublicURLPath(page.UglyURL); got != tc.want {
				t.Fatalf("PageSource.publicURLPath() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPageSourcePublicURLPathTrimsSlashes(t *testing.T) {
	tests := []struct {
		sourcePath string
		uglyURL    bool
		want       string
	}{
		{sourcePath: "/about/", want: "/about/"},
		{sourcePath: "//posts//hello//", want: "/posts/hello/"},
		{sourcePath: "/404/", uglyURL: true, want: "/404.html"},
	}
	for _, tc := range tests {
		if got := (PageSource{Path: tc.sourcePath}).PublicURLPath(tc.uglyURL); got != tc.want {
			t.Fatalf("PageSource.publicURLPath(%q, %t) = %q, want %q", tc.sourcePath, tc.uglyURL, got, tc.want)
		}
	}
}

func TestPageResolvePublicURLPageSourceProjectsPublicURLPath(t *testing.T) {
	tests := []struct {
		name      string
		publicURL string
		uglyURL   bool
		want      string
	}{
		{name: "pretty root", publicURL: "/", want: ""},
		{name: "pretty page", publicURL: "/posts/hello/", want: filepath.Join("posts", "hello")},
		{name: "ugly root", publicURL: "/index.html", uglyURL: true, want: ""},
		{name: "ugly page", publicURL: "/posts/hello.html", uglyURL: true, want: filepath.Join("posts", "hello")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			page := &Page{
				Source:    PageSource{Directory: "content", Filename: "index.md"},
				PublicURL: url.URL{Path: test.publicURL},
				Format:    "markdown",
				UglyURL:   test.uglyURL,
			}
			if got := page.resolvePublicURLPageSource("drafts").Path; got != test.want {
				t.Fatalf("destination source path for %q (ugly=%t) = %q, want %q", test.publicURL, test.uglyURL, got, test.want)
			}
		})
	}
}

func TestPageResolvePublicURLPageSourceMatchesAlignedSource(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		filename  string
		publicURL string
		format    string
		uglyURL   bool
	}{
		{name: "pretty root markdown", filename: "index.md", publicURL: "/", format: "markdown"},
		{name: "pretty nested json", path: filepath.Join("posts", "hello"), filename: "index.json", publicURL: "/posts/hello/", format: "json"},
		{name: "ugly root yaml", filename: "index.yaml", publicURL: "/index.html", format: "yaml", uglyURL: true},
		{name: "ugly nested yml", path: filepath.Join("posts", "hello"), filename: "index.yaml", publicURL: "/posts/hello.html", format: "yml", uglyURL: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			page := &Page{
				Source:    PageSource{Directory: "content", Path: test.path, Filename: test.filename},
				PublicURL: url.URL{Path: test.publicURL},
				Format:    test.format,
				UglyURL:   test.uglyURL,
			}
			if destination := page.resolvePublicURLPageSource(page.Source.Directory); !page.Source.matches(destination) {
				t.Fatalf("resolvePublicURLPageSource() = %#v, want source %#v", destination, page.Source)
			}
		})
	}
}

func TestPageSourceDir(t *testing.T) {
	source := PageSource{Directory: "content", Path: filepath.Join("posts", "hello")}
	if want := filepath.Join("content", "posts", "hello"); source.Dir() != want {
		t.Fatalf("PageSource.Dir() = %q, want %q", source.Dir(), want)
	}
}

func TestPageSourceMatches(t *testing.T) {
	source := PageSource{Directory: "content", Path: filepath.Join("posts", "hello"), Filename: "index.md"}
	if !source.matches(source) {
		t.Fatal("PageSource.matches() = false, want true for identical source")
	}
	if source.matches(PageSource{Directory: "content", Path: filepath.Join("posts", "hello"), Filename: "index.yaml"}) {
		t.Fatal("PageSource.matches() = true, want false for a different data filename")
	}
	if source.matches(PageSource{Directory: "drafts", Path: filepath.Join("posts", "hello"), Filename: "index.md"}) {
		t.Fatal("PageSource.matches() = true, want false for a different source directory")
	}
	if source.matches(PageSource{Directory: "content", Path: filepath.Join("posts", "hello")}) {
		t.Fatal("PageSource.matches() = true, want false for incomplete source state")
	}
}

func TestPageCanonicalURL(t *testing.T) {
	website, _ := testWebsite(t)
	page := &Page{PublicURL: url.URL{Path: "/about/"}}

	canonicalURL, err := page.CanonicalURL(website)
	if err != nil {
		t.Fatalf("CanonicalURL() error = %v", err)
	}
	if got, want := canonicalURL.String(), "https://example.com/about/"; got != want {
		t.Fatalf("CanonicalURL() = %q, want %q", got, want)
	}
}

func TestPageCanonicalURLRejectsNonWebsiteRelativePublicURL(t *testing.T) {
	website, _ := testWebsite(t)
	tests := []struct {
		name      string
		publicURL url.URL
	}{
		{name: "scheme", publicURL: url.URL{Scheme: "https", Host: "elsewhere.example", Path: "/about/"}},
		{name: "opaque", publicURL: url.URL{Scheme: "mailto", Opaque: "hello@example.com", Path: "/about/"}},
		{name: "host", publicURL: url.URL{Host: "elsewhere.example", Path: "/about/"}},
		{name: "user info", publicURL: url.URL{User: url.User("user"), Path: "/about/"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			page := &Page{PublicURL: test.publicURL}

			if _, err := page.CanonicalURL(website); err == nil || !strings.Contains(err.Error(), "website-relative") {
				t.Fatalf("CanonicalURL() error = %v, want website-relative public URL requirement", err)
			}
		})
	}
}

func TestPageResolveTemplateDataSetsCanonicalURL(t *testing.T) {
	website, _ := testWebsite(t)
	page := &Page{PublicURL: url.URL{Path: "/about/"}}

	page.ResolveTemplateData(website)
	if got, want := page.TemplateData.Get("canonical_url"), "https://example.com/about/"; got != want {
		t.Fatalf("canonical_url = %q, want %q", got, want)
	}
}

func TestPageResolveTemplateDataOmitsEmptyFeeds(t *testing.T) {
	page := &Page{
		Feeds:        FeedDataset{},
		TemplateData: httt.TemplateData{"feeds": httt.TemplateData{}},
	}

	page.ResolveTemplateData(nil)
	if _, exists := page.TemplateData["feeds"]; exists {
		t.Fatalf("page template data retained empty feeds: %#v", page.TemplateData.Get("feeds"))
	}
	bytes, err := page.RenderData(nil, "md")
	if err != nil {
		t.Fatalf("RenderData() error = %v", err)
	}
	if strings.Contains(string(bytes), "feeds:") {
		t.Fatalf("rendered markdown includes empty feeds: %s", bytes)
	}
}

func TestPageAtomEntryUsesCanonicalURLForIDAndAlternateLink(t *testing.T) {
	website, _ := testWebsite(t)
	page := Page{
		PublicURL: url.URL{Path: "/about/"},
		Redirect:  "https://elsewhere.example/post/",
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
		Source: PageSource{
			Directory: filepath.Dir(filepath.Join(pageDir, "index.md")),
			Filename:  filepath.Base(filepath.Join(pageDir, "index.md")),
		},
		PublicURL: url.URL{Path: "/s/123/"},
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
	page := pageWithURI(&url.URL{Path: sourcePath}, Page{})

	if err := LoadPage(website, page, false, io.Discard); err != nil {
		t.Fatalf("LoadPage() error = %v", err)
	}
	if !page.UglyURL {
		t.Fatal("LoadPage() did not parse ugly_url")
	}
	if page.PublicURL.Path != "/404.html" {
		t.Fatalf("LoadPage() path = %q, want %q", page.PublicURL.Path, "/404.html")
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
				Source: PageSource{
					Directory: filepath.Dir(uri.Path),
					Filename:  filepath.Base(uri.Path),
				},
				CreatedAt:   "2026-05-19T00:00:00Z",
				LayoutRef:   "default",
				Title:       "Long Page",
				Description: "stale description should disappear after shorter save",
				Content:     "stale body should disappear after shorter save",
			}
			if err := writePage(website, first, io.Discard); err != nil {
				t.Fatalf("first writePage() error = %v", err)
			}

			second := &Page{
				Source: PageSource{
					Directory: filepath.Dir(uri.Path),
					Filename:  filepath.Base(uri.Path),
				},
				CreatedAt: "2026-05-19T00:00:00Z",
				LayoutRef: "default",
				Title:     "Short Page",
				Content:   "ok",
			}
			if err := writePage(website, second, io.Discard); err != nil {
				t.Fatalf("second writePage() error = %v", err)
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

			loaded := pageWithURI(uri, Page{})
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
		Source: PageSource{
			Path:     filepath.Join("posts", "draft"),
			Filename: filepath.Base(sourcePath),
		},
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
			Layout:       "post.html",
			PathResolver: TitleKabobPathResolver,
			Format:       "markdown",
			Visibility:   UnlistedVisibility,
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
		Source: PageSource{
			Directory: website.ContentDirPath(),
			Path:      filepath.Join("posts", "draft-fixed"),
			Filename:  filepath.Base(sourcePath),
		},
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

func TestContentTypePreviewPageDataValidatesDefinition(t *testing.T) {
	website, _ := testWebsite(t)
	contenttype := ContentType{
		Metadata: ContentTypeMetadata{Name: "Invalid Preview"},
		Spec: ContentTypeSpecification{
			Format:     "markdown",
			Visibility: PublicVisibility,
		},
	}

	_, err := contenttype.PreviewPageData(website, &Page{})
	if err == nil || !strings.Contains(err.Error(), "content-type spec.path_resolver is required") {
		t.Fatalf("PreviewPageData() error = %v, want path_resolver requirement", err)
	}
}

func testPageContentType(format string, contentPath string) *ContentType {
	return &ContentType{
		Metadata: ContentTypeMetadata{Name: "Test"},
		Spec: ContentTypeSpecification{
			Layout:       "default",
			PathResolver: TimestampPathResolver,
			Format:       format,
			Path:         contentPath,
			Visibility:   DraftVisibility,
		},
	}
}

func testPageContentTypeWithPathResolver(format string, contentPath string, resolver PublicURLPathResolverKind) *ContentType {
	contenttype := testPageContentType(format, contentPath)
	contenttype.Spec.PathResolver = resolver
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

func autosaveTestPage(website *Website, page *Page, contenttype *ContentType, w io.Writer) error {
	if page.Source.Directory == "" {
		page.Source.Directory = website.ContentDirPath()
	}
	if err := updatePage(website, contenttype, page); err != nil {
		return err
	}
	return savePage(website, page, w)
}

func TestPageSaveRejectsSourcesOutsideWebsiteRoot(t *testing.T) {
	website, _ := testWebsite(t)
	outsideDirectory := t.TempDir()
	outsideSourcePath := filepath.Join(outsideDirectory, "index.md")
	insideSourcePath := filepath.Join(website.ContentDirPath(), "draft", "index.md")
	writePageTestFile(t, outsideSourcePath, "outside")
	writePageTestFile(t, insideSourcePath, "inside")

	tests := []struct {
		name string
		page *Page
		path string
	}{
		{
			name: "current source",
			page: &Page{
				Source:    PageSource{Directory: outsideDirectory, Filename: "index.md"},
				PublicURL: url.URL{Path: "/posts/safe/"},
				Format:    "markdown",
			},
			path: outsideSourcePath,
		},
		{
			name: "projected destination",
			page: &Page{
				Source:    PageSource{Directory: website.ContentDirPath(), Path: "draft", Filename: "index.md"},
				PublicURL: url.URL{Path: "/../../outside/"},
				Format:    "markdown",
			},
			path: insideSourcePath,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.page.Save(website, io.Discard)
			if err == nil || !strings.Contains(err.Error(), "escapes website root") {
				t.Fatalf("Page.Save() error = %v, want website-root escape error", err)
			}
			if _, err := os.Stat(test.path); err != nil {
				t.Fatalf("source path missing after rejected save: %v", err)
			}
		})
	}
}

func TestPageFinalizeRejectsSourceOutsideWebsiteRoot(t *testing.T) {
	website, _ := testWebsite(t)
	draftsDirectory := t.TempDir()
	sourcePath := filepath.Join(draftsDirectory, "posts", "draft", "index.md")
	writePageTestFile(t, sourcePath, "draft")
	page := &Page{
		Source: PageSource{
			Directory: draftsDirectory,
			Path:      filepath.Join("posts", "draft"),
			Filename:  "index.md",
		},
		Visibility: DraftVisibility,
		Title:      "Published Draft",
		Content:    "draft",
	}
	contenttype := testPageContentTypeWithPathResolver("markdown", "posts", TitleKabobPathResolver)

	err := page.Finalize(website, contenttype, "", io.Discard)
	if err == nil || !strings.Contains(err.Error(), "escapes website root") {
		t.Fatalf("Page.Finalize() error = %v, want website-root escape error", err)
	}
	if _, err := os.Stat(sourcePath); err != nil {
		t.Fatalf("source path missing after rejected finalization: %v", err)
	}
}

func TestCreatePageRequiresSourceDirectory(t *testing.T) {
	website, _ := testWebsite(t)
	contenttype := testPageContentTypeWithPathResolver("markdown", "posts", TitleKabobPathResolver)

	if _, err := website.CreatePage(contenttype, ""); err == nil || !strings.Contains(err.Error(), "source directory is required to create a page") {
		t.Fatalf("Website.CreatePage() error = %v, want source directory requirement", err)
	}
}

func TestCreatePageRejectsEmptyContentType(t *testing.T) {
	website, _ := testWebsite(t)

	if _, err := website.CreatePage(&ContentType{Spec: ContentTypeSpecification{Visibility: PublicVisibility}}, website.ContentDirPath()); err == nil || !strings.Contains(err.Error(), "content-type spec.format is required") {
		t.Fatalf("Website.CreatePage() error = %v, want invalid content type", err)
	}
}

func TestCreatePageInitializesUnsavedPage(t *testing.T) {
	website, _ := testWebsite(t)
	sourceDirectory := filepath.Join(website.RootDir(), "drafts")
	contenttype := testPageContentType("markdown", "posts")

	page, err := website.CreatePage(contenttype, sourceDirectory)
	if err != nil {
		t.Fatalf("Website.CreatePage() error = %v", err)
	}
	if page.Source.Directory != sourceDirectory {
		t.Fatalf("page.Source.Directory = %q, want %q", page.Source.Directory, sourceDirectory)
	}
	if page.Source.URI() != nil {
		t.Fatalf("page.Source.URI() = %#v, want nil before persistence", page.Source.URI())
	}
	if page.Format != "markdown" || page.LayoutRef != "default" {
		t.Fatalf("page direct content-type state = format %q, layout %q", page.Format, page.LayoutRef)
	}
	if page.UpdatedAt == "" || page.PublicURL.Path == "" {
		t.Fatalf("page transition state = updated_at %q, public URL %q", page.UpdatedAt, page.PublicURL.Path)
	}
	if _, err := os.Stat(sourceDirectory); !os.IsNotExist(err) {
		t.Fatalf("source directory exists after object-only Website.CreatePage: %v", err)
	}
}

func TestUpdatePageRejectsInvalidContentTypeWithoutUpdatingTimestamp(t *testing.T) {
	website, _ := testWebsite(t)
	updatedAt := "2026-05-20T00:00:00Z"
	page := pageWithURI(&url.URL{Path: filepath.Join(website.ContentDirPath(), "draft", "index.md")}, Page{UpdatedAt: updatedAt})

	err := website.UpdatePage(&ContentType{Spec: ContentTypeSpecification{Visibility: PublicVisibility}}, page)
	if err == nil || !strings.Contains(err.Error(), "content-type spec.format is required") {
		t.Fatalf("Website.UpdatePage() error = %v, want invalid content type", err)
	}
	if page.UpdatedAt != updatedAt {
		t.Fatalf("page.UpdatedAt = %q, want unchanged %q", page.UpdatedAt, updatedAt)
	}
}

func TestPageValidateAllowsEmptyLayout(t *testing.T) {
	website, _ := testWebsite(t)
	page := &Page{
		Source: PageSource{
			Directory: website.ContentDirPath(),
			Path:      "draft",
			Filename:  "index.md",
		},
		UpdatedAt: "2026-05-20T00:00:00Z",
	}

	if err := page.Validate(); err != nil {
		t.Fatalf("Page.Validate() error = %v, want empty layout accepted", err)
	}
}

func TestPageFinalizeAllowsEmptyLayout(t *testing.T) {
	website, _ := testWebsite(t)
	sourcePath := filepath.Join(website.ContentDirPath(), "draft", "index.md")
	writePageTestFile(t, sourcePath, "draft")
	page := pageWithURI(&url.URL{Path: sourcePath}, Page{
		Content: "finalized without an explicit layout",
	})

	if err := page.Finalize(website, nil, "", io.Discard); err != nil {
		t.Fatalf("Page.Finalize() error = %v, want empty layout accepted", err)
	}
	if page.LayoutRef != "" {
		t.Fatalf("page.LayoutRef = %q, want empty layout preserved", page.LayoutRef)
	}
}

func TestCreatePageWithTemplatedContentTypePath(t *testing.T) {
	tests := []struct {
		name     string
		resolver PublicURLPathResolverKind
		wantSlug string
	}{
		{name: "title kabob", resolver: TitleKabobPathResolver, wantSlug: "hello-world"},
		{name: "title snake", resolver: TitleSnakePathResolver, wantSlug: "hello_world"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			website, _ := testWebsite(t)
			contenttype := &ContentType{
				Metadata: ContentTypeMetadata{Name: "Blog"},
				Spec: ContentTypeSpecification{
					Layout:       "default",
					Path:         "/blog/${year}/${month}/${day}/",
					PathResolver: tc.resolver,
					Format:       "markdown",
					Visibility:   PublicVisibility,
				},
			}
			page := &Page{
				CreatedAt: "2026-05-24T13:30:00-07:00",
				Title:     "Hello World",
				Content:   "templated content type path",
			}

			if err := autosaveTestPage(website, page, contenttype, io.Discard); err != nil {
				t.Fatalf("autosaveTestPage() error = %v", err)
			}
			wantPath := filepath.Join(website.ContentDirPath(), "blog", "2026", "05", "24", tc.wantSlug, "index.md")
			if page.Source.URI() == nil || page.Source.URI().Path != wantPath {
				t.Fatalf("page.URI = %#v, want %s", page.Source.URI(), wantPath)
			}
			if page.PublicURL.Path != "/blog/2026/05/24/"+tc.wantSlug+"/" {
				t.Fatalf("page.Path = %q, want templated date path", page.PublicURL.Path)
			}
		})
	}
}

func TestCreateDraftPageAppliesContentTypeVisibility(t *testing.T) {
	website, _ := testWebsite(t)
	contenttype := testPageContentType("markdown", "posts")
	contenttype.Spec.Visibility = UnlistedVisibility
	page := &Page{Visibility: DraftVisibility, Content: "draft content"}

	if err := autosaveTestPage(website, page, contenttype, io.Discard); err != nil {
		t.Fatalf("autosaveTestPage() error = %v", err)
	}
	if page.Visibility != UnlistedVisibility {
		t.Fatalf("page.Visibility = %q, want unlisted", page.Visibility)
	}
	bytes, err := os.ReadFile(page.Source.URI().Path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(bytes), "visibility: unlisted") {
		t.Fatalf("created page missing visibility frontmatter:\n%s", bytes)
	}
}

func TestUpdatePageWithTemplatedContentTypePathUsesUpdatedAtBeforeFinalization(t *testing.T) {
	website, _ := testWebsite(t)
	sourcePath := filepath.Join(website.ContentDirPath(), "drafts", "working", "index.md")
	writePageTestFile(t, sourcePath, "---\ncreated_at: 2026-05-20T00:00:00Z\nlayout: default\nvisibility: draft\n---\n\nold")
	contenttype := &ContentType{
		Metadata: ContentTypeMetadata{Name: "Blog"},
		Spec: ContentTypeSpecification{
			Layout:       "default",
			Path:         "/blog/${year}/${month}/${day}/",
			PathResolver: TitleKabobPathResolver,
			Format:       "markdown",
			Visibility:   PublicVisibility,
		},
	}
	page := pageWithURI(&url.URL{Path: sourcePath}, Page{
		UpdatedAt: "2026-05-20T00:00:00Z",
		Title:     "Published Draft",
		Content:   "published",
	})

	if err := autosaveTestPage(website, page, contenttype, io.Discard); err != nil {
		t.Fatalf("autosaveTestPage() error = %v", err)
	}
	updatedAt, err := time.Parse(time.RFC3339, page.UpdatedAt)
	if err != nil {
		t.Fatalf("page.UpdatedAt = %q, want RFC3339 timestamp: %v", page.UpdatedAt, err)
	}
	destinationPath := filepath.Join(
		website.ContentDirPath(),
		"blog",
		updatedAt.Format("2006"),
		updatedAt.Format("01"),
		updatedAt.Format("02"),
		"published-draft",
		"index.md",
	)
	if page.Source.URI() == nil || page.Source.URI().Path != destinationPath {
		t.Fatalf("page.URI = %#v, want %s", page.Source.URI(), destinationPath)
	}
	if page.PublicURL.Path != "/blog/"+updatedAt.Format("2006/01/02")+"/published-draft/" {
		t.Fatalf("page.Path = %q, want updated_at date path", page.PublicURL.Path)
	}
	if _, err := os.Stat(destinationPath); err != nil {
		t.Fatalf("destination page missing: %v", err)
	}
	if _, err := os.Stat(sourcePath); !os.IsNotExist(err) {
		t.Fatalf("source page still exists or unexpected error = %v", err)
	}
}

func TestUpdateDraftPageAppliesContentTypeVisibility(t *testing.T) {
	website, _ := testWebsite(t)
	sourcePath := filepath.Join(website.ContentDirPath(), "posts", "draft-fixed", "index.md")
	writePageTestFile(t, sourcePath, "---\ncreated_at: 2026-05-20T00:00:00Z\nlayout: default\nvisibility: draft\n---\n\nold")
	contenttype := testPageContentType("markdown", "posts")
	contenttype.Spec.Visibility = UnlistedVisibility
	page := pageWithURI(&url.URL{Path: sourcePath}, Page{Visibility: DraftVisibility, Content: "updated draft"})

	if err := autosaveTestPage(website, page, contenttype, io.Discard); err != nil {
		t.Fatalf("autosaveTestPage() error = %v", err)
	}
	if page.Visibility != UnlistedVisibility {
		t.Fatalf("page.Visibility = %q, want unlisted", page.Visibility)
	}
	bytes, err := os.ReadFile(page.Source.URI().Path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(bytes), "visibility: unlisted") {
		t.Fatalf("updated page missing visibility frontmatter:\n%s", bytes)
	}
}

func TestCreateDraftPageWithEmptySlugUsesTemporaryURI(t *testing.T) {
	website, _ := testWebsite(t)
	contenttype := testPageContentTypeWithPathResolver("markdown", "posts", TitleKabobPathResolver)
	page := &Page{Visibility: DraftVisibility, Content: "draft content"}

	if err := autosaveTestPage(website, page, contenttype, io.Discard); err != nil {
		t.Fatalf("autosaveTestPage() error = %v", err)
	}
	if page.Slug != "" {
		t.Fatalf("page.Slug = %q, want empty", page.Slug)
	}
	if page.Source.URI() == nil {
		t.Fatal("page.URI is nil")
	}
	pageDir := filepath.Dir(page.Source.URI().Path)
	draftSegment := filepath.Base(pageDir)
	if filepath.Dir(pageDir) != filepath.Join(website.ContentDirPath(), "posts") {
		t.Fatalf("page.URI = %s, want under posts draft directory", page.Source.URI().Path)
	}
	assertTimestampDraftSegment(t, draftSegment)
	if page.PublicURL.Path != "/posts/"+draftSegment+"/" {
		t.Fatalf("page.Path = %q, want computed draft path", page.PublicURL.Path)
	}
	bytes, err := os.ReadFile(page.Source.URI().Path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if strings.Contains(string(bytes), "slug:") {
		t.Fatalf("draft page serialized temporary slug:\n%s", string(bytes))
	}
}

func TestCreateDraftPageWithUserSlugPreservesSlug(t *testing.T) {
	website, _ := testWebsite(t)
	contenttype := testPageContentTypeWithPathResolver("markdown", "posts", TitleKabobPathResolver)
	page := &Page{Visibility: DraftVisibility, Slug: "custom-draft", Content: "draft content"}

	if err := autosaveTestPage(website, page, contenttype, io.Discard); err != nil {
		t.Fatalf("autosaveTestPage() error = %v", err)
	}
	wantPath := filepath.Join(website.ContentDirPath(), "posts", "custom-draft", "index.md")
	if page.Source.URI() == nil || page.Source.URI().Path != wantPath {
		t.Fatalf("page.URI = %#v, want %s", page.Source.URI(), wantPath)
	}
	if page.Slug != "custom-draft" {
		t.Fatalf("page.Slug = %q, want custom-draft", page.Slug)
	}
	if page.PublicURL.Path != "/posts/custom-draft/" {
		t.Fatalf("page.Path = %q, want /posts/custom-draft/", page.PublicURL.Path)
	}
	bytes, err := os.ReadFile(page.Source.URI().Path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(bytes), "slug: custom-draft") {
		t.Fatalf("page did not serialize user slug:\n%s", string(bytes))
	}
	loaded := pageWithURI(page.Source.URI(), Page{})
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
	contenttype := testPageContentTypeWithPathResolver("json", "notes", TitleKabobPathResolver)
	page := pageWithURI(&url.URL{Path: sourcePath}, Page{Visibility: DraftVisibility, Content: "updated"})

	if err := autosaveTestPage(website, page, contenttype, io.Discard); err != nil {
		t.Fatalf("autosaveTestPage() error = %v", err)
	}
	destinationPath := filepath.Join(website.ContentDirPath(), "notes", "draft-fixed", "index.json")
	if page.Source.URI() == nil || page.Source.URI().Path != destinationPath {
		t.Fatalf("page.URI = %#v, want %s", page.Source.URI(), destinationPath)
	}
	if page.Slug != "" {
		t.Fatalf("page.Slug = %q, want empty", page.Slug)
	}
	if page.PublicURL.Path != "/notes/draft-fixed/" {
		t.Fatalf("page.Path = %q, want /notes/draft-fixed/", page.PublicURL.Path)
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
	contenttype := testPageContentTypeWithPathResolver("json", "notes", TitleKabobPathResolver)
	page := pageWithURI(&url.URL{Path: sourcePath}, Page{Visibility: DraftVisibility, Content: "updated"})

	if err := autosaveTestPage(website, page, contenttype, io.Discard); err != nil {
		t.Fatalf("autosaveTestPage() error = %v", err)
	}
	destinationPath := filepath.Join(website.ContentDirPath(), "notes", "d019ac0ffee0", "index.json")
	if page.Source.URI() == nil || page.Source.URI().Path != destinationPath {
		t.Fatalf("page.URI = %#v, want %s", page.Source.URI(), destinationPath)
	}
	if page.PublicURL.Path != "/notes/d019ac0ffee0/" {
		t.Fatalf("page.Path = %q, want /notes/d019ac0ffee0/", page.PublicURL.Path)
	}
	if _, err := os.Stat(destinationPath); err != nil {
		t.Fatalf("destination page missing: %v", err)
	}
}

func TestUpdateDraftPageWithTitleRehomesToTitleSlug(t *testing.T) {
	website, _ := testWebsite(t)
	sourcePath := filepath.Join(website.ContentDirPath(), "posts", "draft-fixed", "index.md")
	writePageTestFile(t, sourcePath, "---\ncreated_at: 2026-05-20T00:00:00Z\nlayout: default\nvisibility: draft\n---\n\nold")
	contenttype := testPageContentTypeWithPathResolver("markdown", "posts", TitleKabobPathResolver)
	page := pageWithURI(&url.URL{Path: sourcePath}, Page{Visibility: DraftVisibility, Title: "Final Draft", Content: "updated"})

	if err := autosaveTestPage(website, page, contenttype, io.Discard); err != nil {
		t.Fatalf("autosaveTestPage() error = %v", err)
	}
	destinationPath := filepath.Join(website.ContentDirPath(), "posts", "final-draft", "index.md")
	if page.Source.URI() == nil || page.Source.URI().Path != destinationPath {
		t.Fatalf("page.URI = %#v, want %s", page.Source.URI(), destinationPath)
	}
	if page.Slug != "" {
		t.Fatalf("page.Slug = %q, want empty", page.Slug)
	}
	if page.PublicURL.Path != "/posts/final-draft/" {
		t.Fatalf("page.Path = %q, want /posts/final-draft/", page.PublicURL.Path)
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
	contenttype := testPageContentTypeWithPathResolver("markdown", "posts", TitleKabobPathResolver)
	page := pageWithURI(&url.URL{Path: sourcePath}, Page{Title: "Published Draft", Content: "published"})

	if err := autosaveTestPage(website, page, contenttype, io.Discard); err != nil {
		t.Fatalf("autosaveTestPage() error = %v", err)
	}
	destinationPath := filepath.Join(website.ContentDirPath(), "posts", "published-draft", "index.md")
	if page.Source.URI() == nil || page.Source.URI().Path != destinationPath {
		t.Fatalf("page.URI = %#v, want %s", page.Source.URI(), destinationPath)
	}
	if page.Slug != "" {
		t.Fatalf("page.Slug = %q, want empty", page.Slug)
	}
	if page.PublicURL.Path != "/posts/published-draft/" {
		t.Fatalf("page.Path = %q, want /posts/published-draft/", page.PublicURL.Path)
	}
}

func TestPublishUpdateFailsWhenTitleSlugInputMissing(t *testing.T) {
	tests := []struct {
		name     string
		resolver PublicURLPathResolverKind
	}{
		{name: "title kabob", resolver: TitleKabobPathResolver},
		{name: "title snake", resolver: TitleSnakePathResolver},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			website, _ := testWebsite(t)
			sourcePath := filepath.Join(website.ContentDirPath(), "posts", "draft-fixed", "index.md")
			writePageTestFile(t, sourcePath, "---\ncreated_at: 2026-05-20T00:00:00Z\nlayout: default\nvisibility: draft\n---\n\nold")
			contenttype := testPageContentTypeWithPathResolver("markdown", "posts", tc.resolver)
			contenttype.Spec.Visibility = PublicVisibility
			page := pageWithURI(&url.URL{Path: sourcePath}, Page{Content: "published"})

			err := autosaveTestPage(website, page, contenttype, io.Discard)
			if err == nil || !strings.Contains(err.Error(), "content-type requires page.title property") {
				t.Fatalf("autosaveTestPage() error = %v, want title validation", err)
			}
			if _, statErr := os.Stat(sourcePath); statErr != nil {
				t.Fatalf("source page should remain after validation failure: %v", statErr)
			}
		})
	}
}

func TestPublishUpdateForManualPathResolverValidation(t *testing.T) {
	t.Run("fails without slug", func(t *testing.T) {
		website, _ := testWebsite(t)
		sourcePath := filepath.Join(website.ContentDirPath(), "posts", "draft-fixed", "index.md")
		writePageTestFile(t, sourcePath, "---\ncreated_at: 2026-05-20T00:00:00Z\nlayout: default\nvisibility: draft\n---\n\nold")
		contenttype := testPageContentTypeWithPathResolver("markdown", "posts", ManualPathResolver)
		contenttype.Spec.Visibility = PublicVisibility
		page := pageWithURI(&url.URL{Path: sourcePath}, Page{Content: "published"})

		err := autosaveTestPage(website, page, contenttype, io.Discard)
		if err == nil || !strings.Contains(err.Error(), "content-type requires page.slug property") {
			t.Fatalf("autosaveTestPage() error = %v, want slug validation", err)
		}
		if _, statErr := os.Stat(sourcePath); statErr != nil {
			t.Fatalf("source page should remain after validation failure: %v", statErr)
		}
	})

	t.Run("succeeds with slug", func(t *testing.T) {
		website, _ := testWebsite(t)
		sourcePath := filepath.Join(website.ContentDirPath(), "posts", "draft-fixed", "index.md")
		writePageTestFile(t, sourcePath, "---\ncreated_at: 2026-05-20T00:00:00Z\nlayout: default\nvisibility: draft\n---\n\nold")
		contenttype := testPageContentTypeWithPathResolver("markdown", "posts", ManualPathResolver)
		page := pageWithURI(&url.URL{Path: sourcePath}, Page{Slug: "manual-page", Content: "published"})

		if err := autosaveTestPage(website, page, contenttype, io.Discard); err != nil {
			t.Fatalf("autosaveTestPage() error = %v", err)
		}
		destinationPath := filepath.Join(website.ContentDirPath(), "posts", "manual-page", "index.md")
		if page.Source.URI() == nil || page.Source.URI().Path != destinationPath {
			t.Fatalf("page.URI = %#v, want %s", page.Source.URI(), destinationPath)
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
	contenttype := testPageContentTypeWithPathResolver("markdown", "posts", ManualPathResolver)
	page := pageWithURI(&url.URL{Path: sourcePath}, Page{Slug: "same", Content: "updated in place"})

	if err := autosaveTestPage(website, page, contenttype, io.Discard); err != nil {
		t.Fatalf("autosaveTestPage() error = %v", err)
	}
	if page.Source.URI() == nil || page.Source.URI().Path != sourcePath {
		t.Fatalf("page.URI = %#v, want unchanged source path", page.Source.URI())
	}
	if page.PublicURL.Path != "/posts/same/" {
		t.Fatalf("page.Path = %q, want /posts/same/", page.PublicURL.Path)
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
	page := &Page{
		Source: PageSource{
			Directory: website.ContentDirPath(),
			Path:      filepath.Join("posts", "draft-fixed"),
			Filename:  filepath.Base(sourcePath),
		},
		Slug:      "renamed",
		LayoutRef: "default",
		Title:     "Renamed",
		Content:   "updated in place",
	}

	if err := autosaveTestPage(website, page, nil, io.Discard); err != nil {
		t.Fatalf("autosaveTestPage() error = %v", err)
	}
	if page.Source.URI() == nil || page.Source.URI().Path != sourcePath {
		t.Fatalf("page.URI = %#v, want unchanged source path", page.Source.URI())
	}
	if page.PublicURL.Path != "/posts/draft-fixed/" {
		t.Fatalf("page.Path = %q, want /posts/draft-fixed/", page.PublicURL.Path)
	}
	if page.CreatedAt != "" {
		t.Fatalf("page.CreatedAt = %q, want empty draft creation timestamp", page.CreatedAt)
	}
	if page.UpdatedAt == "" {
		t.Fatal("page.UpdatedAt missing")
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
				Source: PageSource{
					Directory: website.ContentDirPath(),
					Path:      filepath.Dir(tc.relPath),
					Filename:  filepath.Base(sourcePath),
				},
				Slug:      "renamed",
				LayoutRef: "default",
				Content:   "updated " + tc.name,
			}

			if err := autosaveTestPage(website, page, nil, io.Discard); err != nil {
				t.Fatalf("autosaveTestPage() error = %v", err)
			}
			if page.Source.URI() == nil || page.Source.URI().Path != sourcePath {
				t.Fatalf("page.URI = %#v, want unchanged source path", page.Source.URI())
			}
			if page.PublicURL.Path != "/notes/same/" {
				t.Fatalf("page.Path = %q, want /notes/same/", page.PublicURL.Path)
			}
			if page.CreatedAt != "" {
				t.Fatalf("page.CreatedAt = %q, want empty draft creation timestamp", page.CreatedAt)
			}
			if page.UpdatedAt == "" {
				t.Fatal("page.UpdatedAt missing")
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
	page := pageWithURI(&url.URL{Path: sourcePath}, Page{CreatedAt: "2026-05-20T00:00:00Z", Content: "updated"})

	err := autosaveTestPage(website, page, nil, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "unsupported content type format") {
		t.Fatalf("autosaveTestPage() error = %v, want unsupported extension", err)
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
		Source: PageSource{
			Directory: website.ContentDirPath(),
			Path:      filepath.Join("drafts", "working"),
			Filename:  filepath.Base(sourcePath),
		},
		Slug:       "updated",
		CreatedAt:  "2026-05-20T00:00:00Z",
		LayoutRef:  "default",
		Title:      "Updated",
		Content:    "updated content",
		Visibility: DraftVisibility,
	}

	if err := autosaveTestPage(website, page, contenttype, io.Discard); err != nil {
		t.Fatalf("autosaveTestPage() error = %v", err)
	}

	destinationDir := filepath.Join(website.ContentDirPath(), "posts", "updated")
	destinationPath := filepath.Join(destinationDir, "index.md")
	if page.Source.URI() == nil || page.Source.URI().Path != destinationPath {
		t.Fatalf("page.URI = %#v, want %s", page.Source.URI(), destinationPath)
	}
	if page.PublicURL.Path != "/posts/updated/" {
		t.Fatalf("page.Path = %q, want /posts/updated/", page.PublicURL.Path)
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
	if _, err := os.Stat(sourceDir); err != nil {
		t.Fatalf("source directory with nested page was removed: %v", err)
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

func TestSavePageRehomesDraftRootPayload(t *testing.T) {
	website, _ := testWebsite(t)
	draftsDirectory := filepath.Join(website.RootDir(), "drafts")
	sourceDirectory := filepath.Join(draftsDirectory, "posts", "working")
	sourcePath := filepath.Join(sourceDirectory, "index.md")
	for path, contents := range map[string]string{
		sourcePath: "old page",
		filepath.Join(sourceDirectory, "photo.jpg"): "photo",
		filepath.Join(sourceDirectory, ".composer"): "composer",
	} {
		writePageTestFile(t, path, contents)
	}
	page := &Page{
		Source: PageSource{
			Directory: draftsDirectory,
			Path:      filepath.Join("posts", "working"),
			Filename:  "index.md",
		},
		CreatedAt:  "2026-05-20T00:00:00Z",
		LayoutRef:  "default",
		Title:      "Moved Draft",
		Content:    "updated draft",
		Visibility: DraftVisibility,
	}
	contenttype := testPageContentTypeWithPathResolver("markdown", "posts", TitleKabobPathResolver)
	if err := page.Update(website, contenttype); err != nil {
		t.Fatalf("Page.Update() error = %v", err)
	}
	if err := page.Save(website, io.Discard); err != nil {
		t.Fatalf("Page.Save() error = %v", err)
	}
	destinationDirectory := filepath.Join(draftsDirectory, "posts", "moved-draft")
	if got, want := page.Source.Directory, draftsDirectory; got != want {
		t.Fatalf("page.Source.Directory = %q, want %q", got, want)
	}
	for _, name := range []string{"index.md", "photo.jpg", ".composer"} {
		if _, err := os.Stat(filepath.Join(destinationDirectory, name)); err != nil {
			t.Fatalf("saved draft %s missing: %v", name, err)
		}
	}
	if _, err := os.Stat(sourcePath); !os.IsNotExist(err) {
		t.Fatalf("original draft page still exists: %v", err)
	}
	if _, err := os.Stat(sourceDirectory); !os.IsNotExist(err) {
		t.Fatalf("empty source draft directory remains or unexpected error = %v", err)
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
		Source: PageSource{
			Directory: filepath.Dir(sourcePath),
			Filename:  filepath.Base(sourcePath),
		},
		Slug:       "home",
		CreatedAt:  "2026-05-20T00:00:00Z",
		LayoutRef:  "default",
		Title:      "Home",
		Content:    "home content",
		Visibility: DraftVisibility,
	}

	if err := autosaveTestPage(website, page, contenttype, io.Discard); err != nil {
		t.Fatalf("autosaveTestPage() error = %v", err)
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
	page := pageWithURI(&url.URL{Path: sourcePath}, Page{Slug: "taken", CreatedAt: "2026-05-20T00:00:00Z", Content: "new", Visibility: DraftVisibility})

	err := autosaveTestPage(website, page, contenttype, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "page destination unavailable") {
		t.Fatalf("autosaveTestPage() error = %v, want conflict", err)
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
	page := pageWithURI(&url.URL{Path: sourcePath}, Page{Slug: "updated", CreatedAt: "2026-05-20T00:00:00Z", Content: "new", Visibility: DraftVisibility})

	err := autosaveTestPage(website, page, contenttype, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "page destination unavailable") {
		t.Fatalf("autosaveTestPage() error = %v, want regular file conflict", err)
	}
	if _, statErr := os.Stat(sourceAssetPath); statErr != nil {
		t.Fatalf("source asset should remain after conflict: %v", statErr)
	}
}

func TestPageSourceAvailableRequiresEmptyDirectoryUnlessSiblingsAllowed(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "destination")
	source := PageSource{Directory: directory, Filename: "index.md"}
	writePageTestFile(t, filepath.Join(directory, "cover.jpg"), "cover")

	available, err := source.Available(false)
	if err == nil || available {
		t.Fatalf("Available(false) = %t, %v; want false destination conflict", available, err)
	}
	available, err = source.Available(true)
	if err != nil || !available {
		t.Fatalf("Available(true) = %t, %v; want true, nil", available, err)
	}
	writePageTestFile(t, filepath.Join(directory, "index.md"), "page")
	available, err = source.Available(true)
	if err == nil || available {
		t.Fatalf("Available(true) with page file = %t, %v; want false destination conflict", available, err)
	}
}

func TestUpdatePageFormatChangeRemovesOldPageFile(t *testing.T) {
	website, _ := testWebsite(t)
	sourcePath := filepath.Join(website.ContentDirPath(), "posts", "updated", "index.md")
	assetPath := filepath.Join(filepath.Dir(sourcePath), "cover.jpg")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("old markdown"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.WriteFile(assetPath, []byte("cover"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	contenttype := testPageContentType("json", "posts")
	page := pageWithURI(&url.URL{Path: sourcePath}, Page{Slug: "updated", CreatedAt: "2026-05-20T00:00:00Z", Title: "Updated", Content: "json content", Visibility: DraftVisibility})

	if err := autosaveTestPage(website, page, contenttype, io.Discard); err != nil {
		t.Fatalf("autosaveTestPage() error = %v", err)
	}
	destinationPath := filepath.Join(website.ContentDirPath(), "posts", "updated", "index.json")
	if _, err := os.Stat(destinationPath); err != nil {
		t.Fatalf("destination json page missing: %v", err)
	}
	if _, err := os.Stat(sourcePath); !os.IsNotExist(err) {
		t.Fatalf("source markdown page still exists or unexpected error = %v", err)
	}
	if bytes, err := os.ReadFile(assetPath); err != nil || string(bytes) != "cover" {
		t.Fatalf("page asset = %q, %v; want unchanged asset", bytes, err)
	}
}

func TestUpdatePageDefersSourceCoordinateMutationUntilSave(t *testing.T) {
	website, _ := testWebsite(t)
	page := &Page{
		Source: PageSource{
			Directory: website.ContentDirPath(),
			Path:      filepath.Join("drafts", "working"),
			Filename:  "index.md",
		},
		Format:     "markdown",
		Visibility: DraftVisibility,
		Title:      "Updated Draft",
	}
	contenttype := testPageContentTypeWithPathResolver("json", "posts", TitleKabobPathResolver)

	if err := updatePage(website, contenttype, page); err != nil {
		t.Fatalf("updatePage() error = %v", err)
	}
	if page.PublicURL.Path != "/posts/updated-draft/" {
		t.Fatalf("page.Path = %q, want /posts/updated-draft/", page.PublicURL.Path)
	}
	if page.Format != "json" {
		t.Fatalf("page.SourceFormat = %q, want json", page.Format)
	}
	if page.Source.Directory != website.ContentDirPath() || page.Source.Path != filepath.Join("drafts", "working") || page.Source.Filename != "index.md" {
		t.Fatalf("updatePage() mutated persisted source state: %#v", page)
	}
}

func TestCreatePageDoesNotMaterializeContentTypeTemplateOrPatches(t *testing.T) {
	website, _ := testWebsite(t)
	contenttype := &ContentType{
		Metadata: ContentTypeMetadata{Name: "Draft"},
		Spec: ContentTypeSpecification{
			Layout:       "default",
			Path:         "posts",
			PathResolver: TitleKabobPathResolver,
			Format:       "markdown",
			Visibility:   DraftVisibility,
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

	if err := autosaveTestPage(website, page, contenttype, io.Discard); err != nil {
		t.Fatalf("autosaveTestPage() error = %v", err)
	}
	if page.Source.URI() == nil {
		t.Fatal("page.URI is nil")
	}
	pageDir := filepath.Dir(page.Source.URI().Path)
	draftSegment := filepath.Base(pageDir)
	if filepath.Dir(pageDir) != filepath.Join(website.ContentDirPath(), "posts") {
		t.Fatalf("page.URI = %s, want autosave under posts", page.Source.URI().Path)
	}
	assertTimestampDraftSegment(t, draftSegment)
	bytes, err := os.ReadFile(page.Source.URI().Path)
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
			Layout:       "default",
			Path:         "notes",
			PathResolver: TitleKabobPathResolver,
			Format:       "json",
			Visibility:   DraftVisibility,
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
	page := pageWithURI(&url.URL{Path: sourcePath}, Page{Visibility: DraftVisibility, Content: "updated draft"})

	if err := autosaveTestPage(website, page, contenttype, io.Discard); err != nil {
		t.Fatalf("autosaveTestPage() error = %v", err)
	}
	wantPath := filepath.Join(website.ContentDirPath(), "notes", "draft-fixed", "index.json")
	if page.Source.URI() == nil || page.Source.URI().Path != wantPath {
		t.Fatalf("page.URI = %#v, want %s", page.Source.URI(), wantPath)
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
			Layout:       "default",
			Path:         "posts",
			PathResolver: TitleKabobPathResolver,
			Format:       "markdown",
			Visibility:   UnlistedVisibility,
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
		Source: PageSource{
			Directory: filepath.Dir(sourcePath),
			Filename:  filepath.Base(sourcePath),
		},
		Visibility:   DraftVisibility,
		Content:      "final content",
		TemplateData: httt.TemplateData{"visibility": "draft"},
	}

	if err := finalizePage(website, contenttype, page, page.CreatedAt, io.Discard); err != nil {
		t.Fatalf("finalizePage() error = %v", err)
	}
	if page.CreatedAt == "" {
		t.Fatal("page.CreatedAt missing after finalization")
	}
	wantPath := filepath.Join(website.ContentDirPath(), "posts", "final-title", "index.md")
	if page.Source.URI() == nil || page.Source.URI().Path != wantPath {
		t.Fatalf("page.URI = %#v, want %s", page.Source.URI(), wantPath)
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

func TestFinalizePageMovesDraftPayloadIntoContentAndRemovesComposerSidecar(t *testing.T) {
	website, _ := testWebsite(t)
	draftsDirectory := filepath.Join(website.RootDir(), "drafts")
	sourcePath := filepath.Join(draftsDirectory, "posts", "draft", "index.md")
	writePageTestFile(t, sourcePath, "---\nlayout: default\nvisibility: draft\n---\n\ndraft")
	writePageTestFile(t, filepath.Join(filepath.Dir(sourcePath), "photo.jpg"), "image")
	writePageTestFile(t, filepath.Join(filepath.Dir(sourcePath), ".composer"), "sidecar")
	contenttype := testPageContentTypeWithPathResolver("markdown", "posts", TitleKabobPathResolver)
	page := &Page{
		Source: PageSource{
			Directory: draftsDirectory,
			Path:      filepath.Join("posts", "draft"),
			Filename:  filepath.Base(sourcePath),
		},
		Visibility: DraftVisibility,
		Title:      "Published Draft",
		Content:    "draft",
	}
	if err := finalizePage(website, contenttype, page, page.CreatedAt, io.Discard); err != nil {
		t.Fatalf("finalizePage() error = %v", err)
	}
	destinationDirectory := filepath.Join(website.ContentDirPath(), "posts", "published-draft")
	if page.Source.Directory != website.ContentDirPath() {
		t.Fatalf("page.Source.Directory = %q, want content root", page.Source.Directory)
	}
	for _, name := range []string{"index.md", "photo.jpg"} {
		if _, err := os.Stat(filepath.Join(destinationDirectory, name)); err != nil {
			t.Fatalf("finalized %s missing: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(destinationDirectory, ".composer")); !os.IsNotExist(err) {
		t.Fatalf("composer sidecar was copied to content: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(sourcePath)); !os.IsNotExist(err) {
		t.Fatalf("empty source draft directory remains or unexpected error = %v", err)
	}
}

func TestFinalizePageConflictPreservesComposerSidecar(t *testing.T) {
	website, _ := testWebsite(t)
	draftsDirectory := filepath.Join(website.RootDir(), "drafts")
	sourcePath := filepath.Join(draftsDirectory, "posts", "draft", "index.md")
	sidecarPath := filepath.Join(filepath.Dir(sourcePath), ".composer")
	destinationPath := filepath.Join(website.ContentDirPath(), "posts", "taken", "index.md")
	writePageTestFile(t, sourcePath, "---\nlayout: default\nvisibility: draft\n---\n\ndraft")
	writePageTestFile(t, sidecarPath, "sidecar")
	writePageTestFile(t, destinationPath, "published")
	contenttype := testPageContentTypeWithPathResolver("markdown", "posts", TitleKabobPathResolver)
	page := &Page{
		Source: PageSource{
			Directory: draftsDirectory,
			Path:      filepath.Join("posts", "draft"),
			Filename:  filepath.Base(sourcePath),
		},
		Visibility: DraftVisibility,
		Title:      "Taken",
		Content:    "draft",
	}

	err := finalizePage(website, contenttype, page, page.CreatedAt, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "page destination unavailable") {
		t.Fatalf("finalizePage() error = %v, want destination conflict", err)
	}
	if _, err := os.Stat(sidecarPath); err != nil {
		t.Fatalf("composer sidecar missing after finalization conflict: %v", err)
	}
}

func TestFinalizePageWithoutContentTypeFinalizesInPlaceAndClearsDraft(t *testing.T) {
	website, _ := testWebsite(t)
	sourcePath := filepath.Join(website.ContentDirPath(), "freeform", "draft-fixed", "index.md")
	writePageTestFile(t, sourcePath, "---\ncreated_at: 2026-05-20T00:00:00Z\nlayout: default\nvisibility: draft\n---\n\nold")
	page := &Page{
		Source: PageSource{
			Directory: website.ContentDirPath(),
			Path:      filepath.Join("freeform", "draft-fixed"),
			Filename:  filepath.Base(sourcePath),
		},
		Visibility:   DraftVisibility,
		LayoutRef:    "default",
		Title:        "Freeform",
		Content:      "freeform content",
		TemplateData: httt.TemplateData{"visibility": "draft", "title": "Freeform"},
	}

	createdAt := "2026-05-20T00:00:00Z"
	if err := finalizePage(website, nil, page, createdAt, io.Discard); err != nil {
		t.Fatalf("finalizePage() error = %v", err)
	}
	if page.CreatedAt != createdAt {
		t.Fatalf("page.CreatedAt = %q, want provided timestamp %q", page.CreatedAt, createdAt)
	}
	if page.Source.URI() == nil || page.Source.URI().Path != sourcePath {
		t.Fatalf("page.URI = %#v, want unchanged source path", page.Source.URI())
	}
	if page.Visibility != AutoVisibility {
		t.Fatalf("page.Visibility = %q, want unset", page.Visibility)
	}
	bytes, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	contents := string(bytes)
	if strings.Contains(contents, "visibility:") {
		t.Fatalf("finalized freeform page retained serialized visibility:\n%s", contents)
	}
	for _, want := range []string{"title: Freeform", "freeform content"} {
		if !strings.Contains(contents, want) {
			t.Fatalf("finalized freeform page missing %q:\n%s", want, contents)
		}
	}
}

func TestFinalizePageRejectsEmptyContentType(t *testing.T) {
	website, _ := testWebsite(t)
	sourcePath := filepath.Join(website.ContentDirPath(), "freeform", "draft-fixed", "index.md")
	writePageTestFile(t, sourcePath, "---\nlayout: default\nvisibility: draft\n---\n\ndraft")
	page := &Page{
		Source: PageSource{
			Directory: website.ContentDirPath(),
			Path:      filepath.Join("freeform", "draft-fixed"),
			Filename:  filepath.Base(sourcePath),
		},
		Visibility: DraftVisibility,
		LayoutRef:  "default",
		Title:      "Draft",
	}

	err := finalizePage(website, &ContentType{Spec: ContentTypeSpecification{Visibility: PublicVisibility}}, page, "", io.Discard)
	if err == nil || !strings.Contains(err.Error(), "content-type spec.format is required") {
		t.Fatalf("finalizePage() error = %v, want invalid content type", err)
	}
	if page.CreatedAt != "" || page.Visibility != DraftVisibility {
		t.Fatalf("finalizePage() mutated source page after invalid content type: %#v", page)
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
	page := pageWithURI(&url.URL{Path: sourcePath}, Page{PublicURL: url.URL{Path: "/about/"}})

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
			got := build.BuildPath(&Page{PublicURL: url.URL{Path: tc.path}})
			if got != tc.want {
				t.Fatalf("BuildPath() = %q, want %q", got, tc.want)
			}
		})
	}
}

func pageWithURI(uri *url.URL, page Page) *Page {
	page.Source.Directory = filepath.Dir(uri.Path)
	for directory := page.Source.Directory; directory != filepath.Dir(directory); directory = filepath.Dir(directory) {
		if filepath.Base(directory) != "content" {
			continue
		}
		relative, err := filepath.Rel(directory, uri.Path)
		if err != nil {
			break
		}
		page.Source.Directory = directory
		page.Source.Path = filepath.Dir(relative)
		if page.Source.Path == "." {
			page.Source.Path = ""
		}
		break
	}
	page.Source.Filename = filepath.Base(uri.Path)
	return &page
}
