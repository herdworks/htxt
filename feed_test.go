package hypertexting

import (
	"net/url"
	"slices"
	"strings"
	"testing"

	hypertemplates "github.com/herdworks/hypertemplates-go"
	"gopkg.in/yaml.v3"
)

func TestFeedDatasetUnmarshalsFlatAliases(t *testing.T) {
	var dataset FeedDataset
	if err := yaml.Unmarshal([]byte("latest:\n  source: blog\ntasty:\n  tag: tasty\n"), &dataset); err != nil {
		t.Fatal(err)
	}
	if dataset[UserFeed]["latest"].Source != "blog" || dataset[TagFeed]["tasty"].Tag != "tasty" {
		t.Fatalf("dataset = %#v", dataset)
	}
}

func TestFeedDatasetMarshalsFlatAliasesWithoutRuntimeData(t *testing.T) {
	dataset := FeedDataset{UserFeed: {"latest": {Source: "blog", URL: url.URL{Path: "/feeds/blog.atom.xml"}, Items: []hypertemplates.TemplateData{{"title": "one"}}}}, TagFeed: {"tasty": {Tag: "tasty"}}}
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
	page := &Page{Path: "/blog/post/", Tags: []string{"featured"}}
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
		page := &Page{URI: &url.URL{Path: "/content/index.md"}, TemplateData: hypertemplates.TemplateData{}}
		data := hypertemplates.TemplateData{}
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
		URI:          &url.URL{Path: "/content/index.md"},
		TemplateData: hypertemplates.TemplateData{"existing": "value"},
	}
	for _, data := range []hypertemplates.TemplateData{{"draft": "custom"}, {"unlisted": map[string]any{"custom": true}}} {
		if err := page.Parse(data); err != nil {
			t.Fatalf("Page.Parse(%#v) error = %v", data, err)
		}
	}
	if err := page.Parse(hypertemplates.TemplateData{"draft": true}); err == nil {
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
	page := &Page{TemplateData: hypertemplates.TemplateData{"title": "Post", "feeds": hypertemplates.TemplateData{"blog": "recursive"}}}
	items := feedItemViews(Pages{page})
	if items[0].Get("feeds") != nil || page.TemplateData.Get("feeds") == nil {
		t.Fatalf("items = %#v; source = %#v", items, page.TemplateData)
	}
}

func TestFeedDataHydratesLimitedSharedItems(t *testing.T) {
	website := &Website{BaseURL: "https://example.com/", Feeds: FeedConfigs{"blog": {}}}
	website.feeds = Feeds{UserFeed: {"blog": {URL: url.URL{Path: "/feeds/blog.atom.xml"}, Items: []hypertemplates.TemplateData{{"title": "one"}, {"title": "two"}}}}, TagFeed: {}}
	page := &Page{Feeds: FeedDataset{UserFeed: {"latest": {Source: "blog", Limit: 1}}}}
	if err := page.HydrateFeeds(website); err != nil {
		t.Fatal(err)
	}
	view := page.Feeds[UserFeed]["latest"]
	if len(view.Items) != 1 || view.URL.Path != "/feeds/blog.atom.xml" {
		t.Fatalf("view = %#v", view)
	}
}

func TestWebsiteSelectsFeedsAndRenderersFromCompleteCatalog(t *testing.T) {
	website := &Website{
		BaseURL: "https://example.com/",
		Config:  WebsiteConfig{FeedPath: "/feeds", TagPath: "/tags"},
		Feeds:   FeedConfigs{"blog": {}},
	}
	source := &Page{URI: &url.URL{Path: "/content/post/index.md"}, Path: "/post/", Tags: []string{"news"}, TemplateData: hypertemplates.TemplateData{"title": "Post"}}
	renderer := &Page{
		URI:          &url.URL{Path: "/content/blog/index.md"},
		Path:         "/blog/",
		TemplateData: hypertemplates.TemplateData{},
		Feeds: FeedDataset{
			UserFeed: {"latest": {Source: "blog"}},
			TagFeed:  {"headlines": {Tag: "news"}},
		},
	}
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
