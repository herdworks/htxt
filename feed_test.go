package htxt

import (
	"encoding/xml"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/herdworks/httt"
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
}

func TestFeedAtomIncludesResolvedMetadata(t *testing.T) {
	website := &Website{BaseURL: "https://example.com/"}
	feed := Feed{
		Metadata: FeedMeta{Title: "Blog", Description: "Recent writing", Author: "Feed Author", Email: "author@example.com", Favicon: "/feed-icon.png"},
		URL:      url.URL{Scheme: "https", Host: "example.com", Path: "/feeds/blog.xml"},
	}
	bytes, err := xml.Marshal(feed.Atom(website))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<title>Blog</title>", "<subtitle>Recent writing</subtitle>", "<author><name>Feed Author</name><hypergraph:favicon>https://example.com/feed-icon.png</hypergraph:favicon><email>author@example.com</email></author>"} {
		if !strings.Contains(string(bytes), want) {
			t.Errorf("Atom output missing %q:\n%s", want, bytes)
		}
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
	atom := feed.Atom(website)
	if got, want := atom.Links[1].Href, "https://example.com/tags/news/"; got != want {
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
