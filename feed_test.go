package htxt

import (
	"encoding/xml"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/herdworks/httt"
	"github.com/herdworks/hyperfeed/generator"
	"gopkg.in/yaml.v3"
)

func TestFeedConfigMetadataUnmarshalsAndOverridesWebsiteDefaults(t *testing.T) {
	var configs FeedConfigs
	if err := yaml.Unmarshal([]byte("blog:\n  metadata:\n    title: Blog\n    description: Recent writing\n    author: Feed Author\n    email: author@example.com\n    favicon: /feed-icon.png\n"), &configs); err != nil {
		t.Fatal(err)
	}
	website := &Website{
		BaseURL:     "https://example.com/",
		Title:       "Site Title",
		Description: "Site Description",
		Byline:      Byline{Name: "Site Author", Email: "site@example.com", Favicon: "/site-icon.png"},
		Config:      WebsiteConfig{FeedPath: "/feeds"},
	}
	feed := website.Feed(UserFeed, "blog", configs["blog"])
	if got, want := feed.Metadata, (FeedMeta{Title: "Blog", Description: "Recent writing", Author: "Feed Author", Email: "author@example.com", Favicon: "/feed-icon.png"}); got != want {
		t.Fatalf("feed metadata = %#v, want %#v", got, want)
	}
	feed = website.Feed(UserFeed, "main", FeedConfig{})
	if got, want := feed.Metadata, (FeedMeta{Title: "Site Title", Description: "Site Description", Author: "Site Author", Email: "site@example.com", Favicon: "/site-icon.png"}); got != want {
		t.Fatalf("default metadata = %#v, want %#v", got, want)
	}
	website.Byline.Email = ""
	website.Contact = &Contact{Email: []ContactEmail{{Address: "contact@example.com"}}}
	feed = website.Feed(UserFeed, "contact", FeedConfig{})
	if got, want := feed.Metadata.Email, "contact@example.com"; got != want {
		t.Fatalf("contact email = %q, want %q", got, want)
	}
	feed = website.Feed(UserFeed, "override", FeedConfig{Metadata: FeedMeta{Email: "feed@example.com"}})
	if got, want := feed.Metadata.Email, "feed@example.com"; got != want {
		t.Fatalf("feed email = %q, want %q", got, want)
	}
}

func TestFeedAtomIncludesResolvedMetadata(t *testing.T) {
	website := &Website{BaseURL: "https://example.com/"}
	feed := Feed{
		Metadata: FeedMeta{Title: "Blog", Description: "Recent writing", Author: "Feed Author", Email: "author@example.com", Favicon: "/feed-icon.png"},
		URL:      url.URL{Scheme: "https", Host: "example.com", Path: "/feeds/blog.xml"},
	}
	model, err := feed.toHyperFeedFeed(website)
	if err != nil {
		t.Fatal(err)
	}
	bytes, err := generator.Atom(model)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<title>Blog</title>", "<subtitle>Recent writing</subtitle>", "<name>Feed Author</name>", "<email>author@example.com</email>"} {
		if !strings.Contains(string(bytes), want) {
			t.Errorf("Atom output missing %q:\n%s", want, bytes)
		}
	}
	if strings.Contains(string(bytes), "hypergraph:") {
		t.Fatalf("Atom output includes a HyperGraph extension:\n%s", bytes)
	}
}

func TestFeedRSSNormalizesPublishingMetadata(t *testing.T) {
	explicit := true
	website := &Website{
		BaseURL:  "https://example.com/",
		CoverURL: "/site-cover.png",
		Favicon:  WebsiteFavicon{Href: "/favicon.ico"},
		Config:   WebsiteConfig{FeedFormat: RSSFeedFormat},
	}
	feed := Feed{
		Metadata: FeedMeta{Title: "Podcast", Description: "Episodes"},
		URL:      url.URL{Scheme: "https", Host: "example.com", Path: "/feeds/podcast.xml"},
		CoverURL: url.URL{Scheme: "https", Host: "example.com", Path: "/podcast-cover.png"},
		Medium:   "podcast",
		Pages: Pages{&Page{
			PublicURL: url.URL{Path: "/episodes/one/"},
			Title:     "One",
			Tags:      []string{"News"},
			CoverURL:  "/episode-cover.png",
			Explicit:  &explicit,
			Attachments: Attachments{&Attachment{
				Kind: AttachmentKindAudio, Href: mustHref(t, "episode.mp3"), Poster: "/episode-poster.png", Duration: 61, Width: 640, Height: 360,
			}},
		}},
	}
	model, err := feed.toHyperFeedFeed(website)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := model.Website.CoverURL, "https://example.com/site-cover.png"; got != want {
		t.Fatalf("website cover URL = %q, want %q", got, want)
	}
	if model.CoverURL == nil || model.CoverURL.String() != "https://example.com/podcast-cover.png" || model.Medium != "podcast" {
		t.Fatalf("feed publishing metadata = %#v", model)
	}
	item := model.Items[0]
	if item.CoverURL == nil || item.CoverURL.String() != "https://example.com/episode-cover.png" || item.Explicit == nil || !*item.Explicit || len(item.Categories) != 1 || item.Categories[0].Label != "News" {
		t.Fatalf("item publishing metadata = %#v", item)
	}
	attachment := item.Attachments[0]
	if attachment.Poster == nil || attachment.Poster.String() != "https://example.com/episode-poster.png" || attachment.Duration != 61 || attachment.Width != 640 || attachment.Height != 360 {
		t.Fatalf("attachment publishing metadata = %#v", attachment)
	}
	bytes, err := generator.RSS(model)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"xmlns:itunes=", "xmlns:media=", "<itunes:explicit>true</itunes:explicit>", "<podcast:medium>podcast</podcast:medium>", "<media:keywords>News</media:keywords>"} {
		if !strings.Contains(string(bytes), want) {
			t.Errorf("RSS output missing %q:\n%s", want, bytes)
		}
	}
}

func TestPageCoverURLResolvesAgainstCanonicalURL(t *testing.T) {
	website := &Website{BaseURL: "https://example.com/"}
	page := &Page{PublicURL: url.URL{Path: "/episodes/one/"}}
	tests := []struct {
		name     string
		coverURL string
		want     string
	}{
		{name: "page relative", coverURL: "cover.jpg", want: "https://example.com/episodes/one/cover.jpg"},
		{name: "nested page relative", coverURL: "images/cover.jpg", want: "https://example.com/episodes/one/images/cover.jpg"},
		{name: "website root relative", coverURL: "/images/cover.jpg", want: "https://example.com/images/cover.jpg"},
		{name: "absolute", coverURL: "https://cdn.example/cover.jpg", want: "https://cdn.example/cover.jpg"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			page.CoverURL = test.coverURL
			item, err := page.toHyperFeedItem(website)
			if err != nil {
				t.Fatal(err)
			}
			if item.CoverURL == nil || item.CoverURL.String() != test.want {
				t.Fatalf("cover URL = %v, want %q", item.CoverURL, test.want)
			}
		})
	}
}

func TestFeedFormatDefaultsToAtomAndRejectsUnknownValues(t *testing.T) {
	website := &Website{Config: WebsiteConfig{}}
	website.ResolveConfig(nil)
	if website.Config.FeedFormat != AtomFeedFormat {
		t.Fatalf("default feed format = %q, want atom", website.Config.FeedFormat)
	}
	website = &Website{Config: WebsiteConfig{FeedFormat: RSSFeedFormat}}
	website.ResolveConfig(&WebsiteConfig{FeedFormat: AtomFeedFormat})
	if website.Config.FeedFormat != AtomFeedFormat {
		t.Fatalf("overridden feed format = %q, want atom", website.Config.FeedFormat)
	}
	if err := (&WebsiteConfig{FeedFormat: "json"}).Validate(); err == nil {
		t.Fatal("WebsiteConfig.Validate accepted an unsupported feed format")
	}
	atomWebsite := &Website{BaseURL: "https://example.com/", Config: WebsiteConfig{FeedFormat: AtomFeedFormat}}
	feed := Feed{URL: url.URL{Scheme: "https", Host: "example.com", Path: "/feeds/main.xml"}}
	model, err := feed.toHyperFeedFeed(atomWebsite)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := model.Links[0].Mimetype, "application/atom+xml"; got != want {
		t.Fatalf("Atom self-link type = %q, want %q", got, want)
	}
	if bytes, err := generator.Atom(model); err != nil || !strings.Contains(string(bytes), `<feed xmlns="http://www.w3.org/2005/Atom"`) {
		t.Fatalf("Atom generation = %q, %v", bytes, err)
	}
}

func TestFeedURLsAndTagMetadataUseXMLAndTagPageIdentity(t *testing.T) {
	website := &Website{
		BaseURL:     "https://example.com/",
		Title:       "Site",
		Description: "Site description",
		Byline:      Byline{Name: "Site Author", Email: "site@example.com", Favicon: "/favicon.png"},
		Config:      WebsiteConfig{FeedPath: "/feeds", TagPath: "/tags"},
	}
	if got, want := website.Feed(UserFeed, "main", FeedConfig{}).URL.Path, "/feeds/main.xml"; got != want {
		t.Fatalf("user feed URL = %q, want %q", got, want)
	}
	tagFeed := website.Feed(TagFeed, "news", FeedConfig{})
	if got, want := tagFeed.URL.Path, "/feeds/tags/news.xml"; got != want {
		t.Fatalf("tag feed URL = %q, want %q", got, want)
	}
	if got, want := tagFeed.Metadata, (FeedMeta{Title: "#news", Description: "Everything tagged with #news", Author: "Site Author", Email: "site@example.com", Favicon: "/favicon.png"}); got != want {
		t.Fatalf("tag metadata = %#v, want %#v", got, want)
	}
}

func TestFeedMetadataMakesSingleTagFeedDistinct(t *testing.T) {
	website := &Website{Config: WebsiteConfig{TagPath: "/tags"}}
	configs := FeedConfigs{
		"news": {
			Include:  []FeedSelector{{Tags: []string{"news"}}},
			Metadata: FeedMeta{Title: "Newsletters"},
		},
	}
	if err := configs.Validate(website); err != nil {
		t.Fatalf("FeedConfigs.Validate() error = %v", err)
	}
}

func TestFeedDatasetUnmarshalsFlatAliases(t *testing.T) {
	var dataset FeedDataset
	if err := yaml.Unmarshal([]byte("latest:\n  source: blog\ntasty:\n  tag: tasty\n"), &dataset); err != nil {
		t.Fatal(err)
	}
	if dataset[UserFeed]["latest"].Source != "blog" || dataset[TagFeed]["tasty"].Tag != "tasty" {
		t.Fatalf("dataset = %#v", dataset)
	}
}

func TestFeedDatasetDefaultsEmptyAliasToSource(t *testing.T) {
	var dataset FeedDataset
	if err := yaml.Unmarshal([]byte("main:\n"), &dataset); err != nil {
		t.Fatal(err)
	}
	if got, want := dataset[UserFeed]["main"].Source, "main"; got != want {
		t.Fatalf("main source = %q, want %q", got, want)
	}
	if err := dataset.Validate(&Website{Feeds: FeedConfigs{"main": {}}}); err != nil {
		t.Fatalf("FeedDataset.Validate() error = %v", err)
	}
}

func TestFeedDatasetDefaultSourceMustBeConfigured(t *testing.T) {
	var dataset FeedDataset
	if err := yaml.Unmarshal([]byte("main:\n"), &dataset); err != nil {
		t.Fatal(err)
	}
	if err := dataset.Validate(&Website{}); err == nil || !strings.Contains(err.Error(), `source "main" is not configured`) {
		t.Fatalf("FeedDataset.Validate() error = %v, want unconfigured source error", err)
	}
}

func TestFeedDatasetMarshalsFlatAliasesWithoutRuntimeData(t *testing.T) {
	dataset := FeedDataset{UserFeed: {"latest": {Source: "blog", URL: url.URL{Path: "/feeds/blog.xml"}, Items: []httt.TemplateData{{"title": "one"}}}}, TagFeed: {"tasty": {Tag: "tasty"}}}
	bytes, err := yaml.Marshal(dataset)
	if err != nil {
		t.Fatal(err)
	}
	contents := string(bytes)
	for _, unexpected := range []string{"user:", "items:", "url:"} {
		if strings.Contains(contents, unexpected) {
			t.Fatalf("serialized feeds contain %q:\n%s", unexpected, contents)
		}
	}
	for _, expected := range []string{"latest:", "source: blog", "tasty:", "tag: tasty"} {
		if !strings.Contains(contents, expected) {
			t.Fatalf("serialized feeds missing %q:\n%s", expected, contents)
		}
	}
}

func TestFeedSelectorUsesPathAndTagsAsAlternatives(t *testing.T) {
	page := &Page{PublicURL: url.URL{Path: "/blog/post/"}, Tags: []string{"featured"}}
	for _, selector := range []FeedSelector{{Path: "/blog/**"}, {Tags: []string{"featured"}}} {
		if !selector.Matches(page) {
			t.Fatalf("selector %#v did not match page", selector)
		}
	}
}

func TestVisibilityControlsWebsiteFeedsAndOutput(t *testing.T) {
	tests := []struct {
		visibility            PageVisibilityKind
		website, feed, output bool
	}{
		{AutoVisibility, true, true, true}, {DefaultVisibility, true, true, true}, {PublicVisibility, true, true, true},
		{UnlistedVisibility, true, false, true}, {WebsiteVisibility, true, false, true},
		{StatusVisibility, false, true, true}, {FeedVisibility, false, true, true}, {StoryVisibility, false, true, true},
		{ChatVisibility, false, true, true}, {MessageVisibility, false, true, true}, {ResponseVisibility, false, true, true}, {ReplyVisibility, false, true, true},
		{DraftVisibility, false, false, false}, {PreviewVisibility, false, false, false}, {NoVisibility, false, false, false},
	}
	for _, test := range tests {
		page := &Page{Visibility: test.visibility}
		if slices.Contains(WebsiteVisible, page.Visibility) != test.website || slices.Contains(FeedVisible, page.Visibility) != test.feed || slices.Contains(BuildVisible, page.Visibility) != test.output {
			t.Fatalf("visibility %q has wrong selection", test.visibility)
		}
	}
	if !slices.Contains(BuildPreviewVisible, DraftVisibility) || !slices.Contains(BuildPreviewVisible, PreviewVisibility) {
		t.Fatal("preview draft selection is disabled")
	}
}

func TestPageParseRejectsLegacyFeedAndVisibilityFields(t *testing.T) {
	for _, key := range []string{"draft: true", "unlisted: true", "feed: {}"} {
		page := pageWithURI(&url.URL{Path: "/content/index.md"}, Page{TemplateData: httt.TemplateData{}})
		data := httt.TemplateData{}
		if err := yaml.Unmarshal([]byte(key), &data); err != nil {
			t.Fatal(err)
		}
		if err := page.Parse(data); err == nil {
			t.Fatalf("Page.Parse accepted %q", key)
		}
	}
}

func TestPageScreenAllowsCustomNonBooleanLegacyNamesWithoutMutationOnReject(t *testing.T) {
	page := &Page{
		TemplateData: httt.TemplateData{"existing": "value"},
	}
	page.Source.Directory = "/content"
	page.Source.Filename = "index.md"
	for _, data := range []httt.TemplateData{{"draft": "custom"}, {"unlisted": map[string]any{"custom": true}}} {
		if err := page.Parse(data); err != nil {
			t.Fatalf("Page.Parse(%#v) error = %v", data, err)
		}
	}
	if err := page.Parse(httt.TemplateData{"draft": true}); err == nil {
		t.Fatal("Page.Parse accepted legacy draft")
	}
	if got := page.TemplateData.Get("unlisted"); got == nil {
		t.Fatalf("rejected parse changed TemplateData: %#v", page.TemplateData)
	}
}

func TestTagFeedUsesTagPageAsAtomAlternate(t *testing.T) {
	website := &Website{BaseURL: "https://example.com/", Config: WebsiteConfig{FeedPath: "/feeds", TagPath: "/tags"}}
	feed := website.Feed(TagFeed, "news", FeedConfig{})
	feed.HTMLURL = website.tagPageURL("news")
	model, err := feed.toHyperFeedFeed(website)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := model.Links[1].Href.String(), "https://example.com/tags/news/"; got != want {
		t.Fatalf("tag alternate = %q, want %q", got, want)
	}
}

func TestFeedItemsCloneWithoutFeeds(t *testing.T) {
	page := &Page{TemplateData: httt.TemplateData{"title": "Post", "feeds": httt.TemplateData{"blog": "recursive"}}}
	items := feedItemViews(Pages{page})
	if items[0].Get("feeds") != nil || page.TemplateData.Get("feeds") == nil {
		t.Fatalf("items = %#v; source = %#v", items, page.TemplateData)
	}
}

func TestFeedDataHydratesLimitedSharedItems(t *testing.T) {
	website := &Website{BaseURL: "https://example.com/", Feeds: FeedConfigs{"blog": {}}}
	website.feeds = Feeds{UserFeed: {"blog": {URL: url.URL{Path: "/feeds/blog.xml"}, Items: []httt.TemplateData{{"title": "one"}, {"title": "two"}}}}, TagFeed: {}}
	page := &Page{Feeds: FeedDataset{UserFeed: {"latest": {Source: "blog", Limit: 1}}}}
	if err := page.HydrateFeeds(website); err != nil {
		t.Fatal(err)
	}
	view := page.Feeds[UserFeed]["latest"]
	if len(view.Items) != 1 || view.URL.Path != "/feeds/blog.xml" {
		t.Fatalf("view = %#v", view)
	}
}

func TestWebsiteSelectsFeedsAndRenderersFromCompleteCatalog(t *testing.T) {
	website := &Website{
		BaseURL: "https://example.com/",
		Config:  WebsiteConfig{FeedPath: "/feeds", TagPath: "/tags"},
		Feeds:   FeedConfigs{"blog": {}},
	}
	source := pageWithURI(&url.URL{Path: "/content/post/index.md"}, Page{PublicURL: url.URL{Path: "/post/"}, Tags: []string{"news"}, TemplateData: httt.TemplateData{"title": "Post"}})
	renderer := &Page{
		PublicURL:    url.URL{Path: "/blog/"},
		TemplateData: httt.TemplateData{},
		Feeds: FeedDataset{
			UserFeed: {"latest": {Source: "blog"}},
			TagFeed:  {"headlines": {Tag: "news"}},
		},
	}
	renderer.Source.Directory = "/content"
	renderer.Source.Path = "blog"
	renderer.Source.Filename = "index.md"
	website.pages = Pages{source, renderer}
	if err := website.HydrateFeeds(); err != nil {
		t.Fatal(err)
	}
	if renderer.TemplateData.Get("feeds") != nil {
		t.Fatal("catalog hydration populated an unselected page's feed template data")
	}
	selected := website.PageFeeds(source)
	if _, ok := selected.Feed(UserFeed, "blog"); !ok {
		t.Fatal("PageFeeds did not select matching user feed")
	}
	if _, ok := selected.Feed(TagFeed, "news"); !ok {
		t.Fatal("PageFeeds did not select matching tag feed")
	}
	if pages := website.FeedPages(selected); len(pages) != 1 || pages[0] != renderer {
		t.Fatalf("FeedPages = %#v, want renderer only", pages)
	}
	if err := renderer.HydrateFeeds(website); err != nil {
		t.Fatal(err)
	}
	if renderer.TemplateData.Get("feeds.latest.items") == nil || renderer.TemplateData.Get("feeds.headlines.items") == nil {
		t.Fatalf("hydrated renderer feeds = %#v", renderer.TemplateData.Get("feeds"))
	}
	if pages := website.TagPages("news"); len(pages) != 1 || pages[0] != renderer {
		t.Fatalf("TagPages = %#v, want tag-bound renderer", pages)
	}
}

func TestFeedAtomDiscussionLinks(t *testing.T) {
	website, _ := testWebsite(t)
	for _, rel := range []string{"comments", "replies"} {
		t.Run(rel, func(t *testing.T) {
			page := &Page{PublicURL: url.URL{Path: "/post/"}}
			if err := page.Parse(httt.TemplateData{
				"title": "Hello", "created_at": "2026-09-17T13:48:00-07:00",
				"links": []map[string]string{
					{"rel": rel, "href": "#comments", "title": "Discussion", "hreflang": "en"},
					{"rel": rel, "href": "https://example.net/replies.xml", "type": "application/atom+xml"},
					{"rel": rel, "href": "mailto:hello@example.com?subject=Hello"},
					{"rel": rel, "href": "mailto:hello@example.com", "type": "text/plain"},
					{"rel": rel, "href": ""},
					{"rel": "stylesheet", "href": "/style.css"},
				},
			}); err != nil {
				t.Fatal(err)
			}
			feed := website.Feed(UserFeed, "updates", FeedConfig{})
			feed.Pages = Pages{page}
			model, err := feed.toHyperFeedFeed(website)
			if err != nil {
				t.Fatal(err)
			}
			data, err := generator.Atom(model)
			if err != nil {
				t.Fatal(err)
			}
			var decoded struct {
				Entries []struct {
					Links []struct {
						Rel   string `xml:"rel,attr"`
						Href  string `xml:"href,attr"`
						Type  string `xml:"type,attr"`
						Title string `xml:"title,attr"`
						Lang  string `xml:"hreflang,attr"`
					} `xml:"link"`
				} `xml:"entry"`
			}
			if err := xml.Unmarshal(data, &decoded); err != nil {
				t.Fatal(err)
			}
			if len(decoded.Entries) != 1 || len(decoded.Entries[0].Links) != 5 {
				t.Fatalf("unexpected entries/links: %s", data)
			}
			links := decoded.Entries[0].Links
			if link := links[1]; link.Rel != "replies" || link.Href != "https://example.com/post/#comments" || link.Type != "text/html" || link.Title != "Discussion" || link.Lang != "en" {
				t.Fatalf("discussion link = %#v", link)
			}
			if link := links[2]; link.Rel != "replies" || link.Type != "application/atom+xml" || link.Href != "https://example.net/replies.xml" {
				t.Fatalf("replies feed link = %#v", link)
			}
			if link := links[3]; link.Rel != "replies" || link.Type != "text/x-url" || link.Href != "mailto:hello@example.com?subject=Hello" {
				t.Fatalf("email reply link = %#v", link)
			}
			if link := links[4]; link.Rel != "replies" || link.Type != "text/plain" || link.Href != "mailto:hello@example.com" {
				t.Fatalf("explicit email reply type = %#v", link)
			}
			if got, want := model.Items[0].CommentsURL.String(), "https://example.com/post/#comments"; got != want {
				t.Fatalf("comments URL = %q, want %q", got, want)
			}
			rssData, err := generator.RSS(model)
			if err != nil {
				t.Fatal(err)
			}
			if got := string(rssData); !strings.Contains(got, "<comments>https://example.com/post/#comments</comments>") || strings.Contains(got, `rel="replies"`) || strings.Contains(got, `rel="alternate"`) {
				t.Fatalf("RSS discussion output = %s", rssData)
			}
		})
	}
}
