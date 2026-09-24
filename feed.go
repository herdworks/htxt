package htxt

import (
	"cmp"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/herdworks/hq"
	"github.com/herdworks/hsync"
	"github.com/herdworks/httt"
	"github.com/herdworks/hyperfeed"
	"github.com/herdworks/hyperfeed/href"
)

// FeedConfig is declarative configuration for site.feeds.
type FeedConfig struct {
	Metadata FeedMeta       `json:"metadata,omitempty" yaml:"metadata,omitempty" jsonschema:"description=Feed metadata overrides."`
	Include  []FeedSelector `json:"include,omitempty" yaml:"include,omitempty" jsonschema:"description=Selectors that include pages in this feed."`
	Exclude  []FeedSelector `json:"exclude,omitempty" yaml:"exclude,omitempty" jsonschema:"description=Selectors that exclude pages from this feed."`
	Sort     []FeedSort     `json:"sort,omitempty" yaml:"sort,omitempty" jsonschema:"description=Ordered feed entry sort rules."`
	HTMLURL  string         `json:"html_url,omitempty" yaml:"html_url,omitempty" jsonschema:"format=uri-reference,description=HTML alternate URL for this feed."`
}
type FeedConfigs map[string]FeedConfig

// FeedMeta defines feed identity and publishing metadata.
type FeedMeta struct {
	Title       string `json:"title,omitempty" yaml:"title,omitempty" jsonschema:"description=Atom feed title."`
	Description string `json:"description,omitempty" yaml:"description,omitempty" jsonschema:"description=Atom feed subtitle."`
	Author      string `json:"author,omitempty" yaml:"author,omitempty" jsonschema:"description=Atom feed author name."`
	Email       string `json:"email,omitempty" yaml:"email,omitempty" jsonschema:"format=email,description=Atom feed author email address."`
	Favicon     string `json:"favicon,omitempty" yaml:"favicon,omitempty" jsonschema:"format=uri-reference,description=Atom feed author favicon URL."`
	CoverURL    string `json:"cover_url,omitempty" yaml:"cover_url,omitempty" jsonschema:"format=uri-reference,description=Feed artwork URL."`
	Medium      string `json:"medium,omitempty" yaml:"medium,omitempty" jsonschema:"description=Podcasting 2.0 feed medium."`
}

// Feed is the resolved feed catalog containing pages and template data
type Feed struct {
	Metadata FeedMeta
	Kind     FeedKind
	ID       string
	Pages    Pages
	Items    []httt.TemplateData
	URL      url.URL
	HTMLURL  url.URL
	CoverURL url.URL
	Medium   string
	config   FeedConfig
}

type Feeds map[FeedKind]map[string]Feed

type FeedData struct {
	Source string `yaml:"source,omitempty" json:"source,omitempty" jsonschema:"description=Site feed identifier."`
	Tag    string `yaml:"tag,omitempty" json:"tag,omitempty" jsonschema:"description=Automatic tag feed identifier."`
	Limit  int    `yaml:"limit,omitempty" json:"limit,omitempty" jsonschema:"description=Maximum number of feed items exposed to this page."`

	URL   url.URL             `yaml:"-" json:"-"`
	Items []httt.TemplateData `yaml:"-" json:"-"`
}

type FeedDataset map[FeedKind]map[string]FeedData

type FeedSelector struct {
	Path string   `json:"path,omitempty" yaml:"path,omitempty" jsonschema:"description=Path expression matched against page paths."`
	Tags []string `json:"tags,omitempty" yaml:"tags,omitempty" jsonschema:"description=Tags matched against page tags."`
}

type FeedSort map[string]string

type FeedKind string
type FeedFormat string

const (
	UserFeed FeedKind = "user"
	TagFeed  FeedKind = "tag"

	RSSFeedFormat  FeedFormat = "rss"
	AtomFeedFormat FeedFormat = "atom"
)

func (format FeedFormat) MIMEType() string {
	if format == AtomFeedFormat {
		return "application/atom+xml"
	}
	return "application/rss+xml"
}

func (FeedConfig) JSONSchemaTitle() string {
	return "Website feed configuration"
}
func (FeedConfig) JSONSchemaName() string    { return "feed-config" }
func (FeedConfig) JSONSchemaSummary() string { return "Defines a site feed in site.feeds." }

func (FeedConfig) JSONSchemaDescription() string {
	return "Defines feed metadata, selectors, ordering, and an HTML alternate URL for a site feed."
}

func (FeedMeta) JSONSchemaTitle() string   { return "Feed metadata" }
func (FeedMeta) JSONSchemaName() string    { return "feed-meta" }
func (FeedMeta) JSONSchemaSummary() string { return "Feed identity and publishing metadata." }

func (FeedMeta) JSONSchemaDescription() string {
	return "Optional feed metadata that overrides website defaults where applicable."
}

func (configs FeedConfigs) Validate(website *Website) error {
	for id, config := range configs {
		if strings.TrimSpace(id) == "" || strings.Contains(id, "/") {
			return fmt.Errorf("site.feeds identifier is invalid: %q", id)
		}
		if id == strings.Trim(strings.TrimPrefix(website.Config.TagPath, "/"), "/") {
			return fmt.Errorf("site.feeds identifier %q collides with tag namespace", id)
		}
		if len(config.Include) == 1 && len(config.Include[0].Tags) == 1 && config.Include[0].Path == "" && len(config.Exclude) == 0 && len(config.Sort) == 0 && config.HTMLURL == "" && config.Metadata == (FeedMeta{}) {
			return fmt.Errorf("site.feeds.%s duplicates an automatic tag feed", id)
		}
		if config.HTMLURL != "" {
			if _, err := website.ResolveHref(config.HTMLURL); err != nil {
				return fmt.Errorf("site.feeds.%s html_url: %w", id, err)
			}
		}
		if config.Metadata.Favicon != "" {
			if _, err := website.ResolveHref(config.Metadata.Favicon); err != nil {
				return fmt.Errorf("site.feeds.%s metadata.favicon: %w", id, err)
			}
		}
		if config.Metadata.CoverURL != "" {
			if _, err := website.ResolveHref(config.Metadata.CoverURL); err != nil {
				return fmt.Errorf("site.feeds.%s metadata.cover_url: %w", id, err)
			}
		}
		for _, selector := range append(append([]FeedSelector{}, config.Include...), config.Exclude...) {
			if selector.Path == "" && len(selector.Tags) == 0 {
				return fmt.Errorf("site.feeds.%s selector is empty", id)
			}
			if selector.Path != "" {
				if _, err := hsync.ParsePathExpression(selector.Path); err != nil {
					return fmt.Errorf("site.feeds.%s path: %w", id, err)
				}
			}
		}
		for _, sort := range config.Sort {
			if len(sort) != 1 {
				return fmt.Errorf("site.feeds.%s sort rules must contain exactly one field", id)
			}
			for field, direction := range sort {
				if field == "" || (!strings.EqualFold(direction, "asc") && !strings.EqualFold(direction, "desc")) {
					return fmt.Errorf("site.feeds.%s sort rule is invalid", id)
				}
			}
		}
	}
	return nil
}

func (FeedData) JSONSchemaTitle() string {
	return "Page feed reference"
}
func (FeedData) JSONSchemaName() string { return "feed-data" }
func (FeedData) JSONSchemaSummary() string {
	return "References a site or automatic tag feed from page.feeds."
}

func (FeedData) JSONSchemaDescription() string {
	return "References a configured site feed or automatic tag feed for page template data. An empty entry uses its alias as the site feed identifier."
}

func (data FeedData) Validate(website *Website) (FeedKind, error) {
	if (data.Source == "") == (data.Tag == "") {
		return "", fmt.Errorf("page.feeds requires exactly one of source or tag")
	}
	if data.Limit < 0 {
		return "", fmt.Errorf("page.feeds limit must not be negative")
	}
	if data.Source != "" {
		if _, ok := website.Feeds[data.Source]; !ok {
			return "", fmt.Errorf("page.feeds source %q is not configured", data.Source)
		}
		return UserFeed, nil
	}
	if err := ValidateTagName(data.Tag); err != nil {
		return "", fmt.Errorf("page.feeds tag: %w", err)
	}
	return TagFeed, nil
}

func (dataset FeedDataset) Validate(website *Website) error {
	for kind, entries := range dataset {
		for alias, data := range entries {
			if validatedKind, err := data.Validate(website); err != nil {
				return fmt.Errorf("page.feeds.%s: %w", alias, err)
			} else if validatedKind != kind {
				return fmt.Errorf("page.feeds.%s has an invalid kind", alias)
			}
		}
	}
	return nil
}

func (FeedSelector) JSONSchemaTitle() string {
	return "Feed selector"
}
func (FeedSelector) JSONSchemaName() string    { return "feed-selector" }
func (FeedSelector) JSONSchemaSummary() string { return "Matches pages for a site feed." }

func (FeedSelector) JSONSchemaDescription() string {
	return "Matches pages by path expression or tag for a site feed."
}

func (dataset *FeedDataset) UnmarshalYAML(value *yaml.Node) error {
	var flat map[string]FeedData
	if err := value.Decode(&flat); err != nil {
		return err
	}
	result := FeedDataset{UserFeed: {}, TagFeed: {}}
	for alias, data := range flat {
		if data.Source == "" && data.Tag == "" {
			data.Source = alias
		}
		if (data.Source == "") == (data.Tag == "") {
			return fmt.Errorf("page.feeds.%s requires exactly one of source or tag", alias)
		}
		kind := UserFeed
		if data.Tag != "" {
			kind = TagFeed
		}
		result[kind][alias] = data
	}
	*dataset = result
	return nil
}

func (dataset FeedDataset) MarshalYAML() (any, error) {
	flat := map[string]FeedData{}
	for _, kind := range []FeedKind{UserFeed, TagFeed} {
		for alias, data := range dataset[kind] {
			if _, exists := flat[alias]; exists {
				return nil, fmt.Errorf("page.feeds alias %q is duplicated", alias)
			}
			data.URL = url.URL{}
			data.Items = nil
			flat[alias] = data
		}
	}
	return flat, nil
}

func (dataset FeedDataset) Clone() FeedDataset {
	clone := FeedDataset{}
	for kind, entries := range dataset {
		clone[kind] = map[string]FeedData{}
		for alias, data := range entries {
			data.Items = nil
			data.URL = url.URL{}
			clone[kind][alias] = data
		}
	}
	return clone
}

func (dataset FeedDataset) TemplateData() httt.TemplateData {
	result := httt.TemplateData{}
	for _, kind := range []FeedKind{UserFeed, TagFeed} {
		for alias, data := range dataset[kind] {
			feed := httt.TemplateData{"url": data.URL.String(), "items": data.Items}
			if data.Tag != "" {
				feed["tag"] = data.Tag
			}
			result[alias] = feed
		}
	}
	return result
}

func (feeds Feeds) Feed(kind FeedKind, id string) (Feed, bool) {
	feed, ok := feeds[kind][id]
	return feed, ok
}

func (feed Feed) toHyperFeedFeed(website *Website) (*hyperfeed.Feed, error) {
	if website == nil {
		return nil, fmt.Errorf("feed generation requires a website")
	}
	feedURL, err := href.Parse(&feed.URL)
	if err != nil {
		return nil, fmt.Errorf("feed URL: %w", err)
	}
	result := &hyperfeed.Feed{
		URL:         feedURL,
		Title:       feed.Metadata.Title,
		Description: feed.Metadata.Description,
		ModifiedAt:  feed.Pages.LastModified(),
		Links: []*hyperfeed.Link{{
			Href:     feedURL,
			Rel:      "self",
			Mimetype: website.Config.FeedFormat.MIMEType(),
			Origin:   "metadata",
		}},
	}
	if result.Website, err = website.toHyperFeedWebsite(); err != nil {
		return nil, fmt.Errorf("feed website: %w", err)
	}
	if feed.CoverURL.String() != "" {
		coverURL, err := href.Parse(&feed.CoverURL)
		if err != nil {
			return nil, fmt.Errorf("feed cover URL: %w", err)
		}
		result.CoverURL = coverURL
	}
	result.Medium = feed.Medium
	if feed.HTMLURL.String() != "" {
		htmlURL, err := href.Parse(&feed.HTMLURL)
		if err != nil {
			return nil, fmt.Errorf("feed HTML URL: %w", err)
		}
		result.HTMLURL = htmlURL
		result.Links = append(result.Links, &hyperfeed.Link{Href: htmlURL, Rel: "alternate", Mimetype: "text/html", Origin: "metadata"})
	}
	if feed.Metadata.Author != "" {
		byline, err := feed.Metadata.toHyperFeedByline(website)
		if err != nil {
			return nil, fmt.Errorf("feed byline: %w", err)
		}
		result.Bylines = []hyperfeed.Byline{byline}
	}
	for _, page := range feed.Pages {
		item, err := page.toHyperFeedItem(website)
		if err != nil {
			return nil, err
		}
		result.Items = append(result.Items, item)
	}
	return result, nil
}

func (website *Website) toHyperFeedWebsite() (result hyperfeed.Website, err error) {
	result.Entity = hq.Entity{HTMLURL: website.URL().String()}
	if website.CoverURL != "" {
		coverURL, err := website.ResolveHref(website.CoverURL)
		if err != nil {
			return result, err
		}
		result.CoverURL = coverURL.String()
	}
	if website.Favicon.Href != "" {
		faviconURL, err := website.ResolveHref(website.Favicon.Href)
		if err != nil {
			return result, err
		}
		result.FaviconURL = faviconURL.String()
	}
	return result, nil
}

func (metadata FeedMeta) toHyperFeedByline(website *Website) (hyperfeed.Byline, error) {
	byline, err := (&Byline{Name: metadata.Author, Email: metadata.Email, Favicon: metadata.Favicon}).toHyperFeedByline(website, "author", 1)
	if err != nil {
		return hyperfeed.Byline{}, err
	}
	byline.Href = nil // FeedMeta has no author URL; retain the existing Atom feed author shape.
	return byline, nil
}

func (website *Website) HydrateFeeds() error {
	feeds := Feeds{UserFeed: {}, TagFeed: {}}
	for id, config := range website.Feeds {
		feed := website.Feed(UserFeed, id, config)
		feed.Pages = feed.selectPages(website.pages)
		feed.HTMLURL = website.feedHTMLURL(config.HTMLURL)
		feeds[UserFeed][id] = feed
	}
	for _, page := range website.pages {
		if !slices.Contains(FeedVisible, page.Visibility) {
			continue
		}
		for _, tag := range page.Tags {
			if _, exists := feeds[TagFeed][tag]; exists {
				continue
			}
			feed := website.Feed(TagFeed, tag, FeedConfig{Include: []FeedSelector{{Tags: []string{tag}}}})
			feed.HTMLURL = website.tagPageURL(tag)
			feed.Pages = feed.selectPages(website.pages)
			feeds[TagFeed][tag] = feed
		}
	}
	for _, catalog := range feeds {
		for id, feed := range catalog {
			feed.Items = feedItemViews(feed.Pages)
			catalog[id] = feed
		}
	}
	website.feeds = feeds
	return nil
}

func (website *Website) Feed(kind FeedKind, id string, config FeedConfig) Feed {
	parts := []string{strings.Trim(website.Config.FeedPath, "/")}
	if kind == TagFeed {
		parts = append(parts, strings.Trim(website.Config.TagPath, "/"))
	}
	parts = append(parts, id+".xml")
	path := "/" + strings.Join(parts, "/")
	metadata := website.feedMetadata(config.Metadata)
	if kind == TagFeed {
		metadata.Title, metadata.Description = tagFeedIdentity(id)
	}
	feed := Feed{Metadata: metadata, Kind: kind, ID: id, URL: *website.URL().ResolveReference(&url.URL{Path: path}), Medium: metadata.Medium, config: config}
	if metadata.CoverURL != "" {
		feed.CoverURL = website.feedHTMLURL(metadata.CoverURL)
	}
	return feed
}

func (website *Website) feedMetadata(overrides FeedMeta) FeedMeta {
	metadata := FeedMeta{
		Title:       website.Title,
		Description: website.Description,
		Author:      website.Byline.Name,
		Email:       website.Byline.Email,
		Favicon:     website.Byline.Favicon,
	}
	if metadata.Email == "" && website.Contact != nil && len(website.Contact.Email) > 0 {
		metadata.Email = website.Contact.Email[0].Address
	}
	if overrides.Title != "" {
		metadata.Title = overrides.Title
	}
	if overrides.Description != "" {
		metadata.Description = overrides.Description
	}
	if overrides.Author != "" {
		metadata.Author = overrides.Author
	}
	if overrides.Email != "" {
		metadata.Email = overrides.Email
	}
	if overrides.Favicon != "" {
		metadata.Favicon = overrides.Favicon
	}
	if overrides.CoverURL != "" {
		metadata.CoverURL = overrides.CoverURL
	}
	if overrides.Medium != "" {
		metadata.Medium = overrides.Medium
	}
	return metadata
}

func tagFeedIdentity(tag string) (title, description string) {
	return "#" + tag, "Everything tagged with #" + tag
}

func (website *Website) feedHTMLURL(value string) url.URL {
	if value == "" {
		return *website.URL()
	}
	resolved, err := website.ResolveHref(value)
	if err != nil {
		return url.URL{}
	}
	return *resolved
}

func (website *Website) tagPageURL(tag string) url.URL {
	path := "/" + strings.Trim(website.Config.TagPath, "/") + "/" + tag + "/"
	return *website.URL().ResolveReference(&url.URL{Path: path})
}

func (page *Page) HydrateFeeds(website *Website) error {
	for kind, entries := range page.Feeds {
		for alias, data := range entries {
			id := data.Source
			if kind == TagFeed {
				id = data.Tag
			}
			if feed, ok := website.feeds.Feed(kind, id); ok {
				data.URL = feed.URL
				data.Items = feed.Items
				if data.Limit > 0 && data.Limit < len(data.Items) {
					data.Items = data.Items[:data.Limit]
				}
			} else {
				data.URL, data.Items = url.URL{}, []httt.TemplateData{}
			}
			entries[alias] = data
		}
	}
	page.ResolveTemplateData(website)
	return nil
}

func (website *Website) PageFeeds(pages ...*Page) Feeds {
	selected := Feeds{UserFeed: {}, TagFeed: {}}
	for _, page := range pages {
		if page == nil {
			continue
		}
		for id, feed := range website.feeds[UserFeed] {
			if feed.Pages.Contains(page) {
				selected[UserFeed][id] = feed
			}
		}
		for _, tag := range page.Tags {
			if feed, ok := website.feeds.Feed(TagFeed, tag); ok {
				selected[TagFeed][tag] = feed
			}
		}
	}
	return selected
}

func (website *Website) TagPages(tags ...string) Pages {
	pages := Pages{}
	for _, page := range website.pages {
		for _, data := range page.Feeds[TagFeed] {
			if slices.Contains(tags, data.Tag) {
				pages = pages.Add(page)
				break
			}
		}
	}
	return pages
}

func (website *Website) FeedPages(feeds Feeds) Pages {
	pages := Pages{}
	for _, page := range website.pages {
		for kind, entries := range page.Feeds {
			for _, data := range entries {
				id := data.Source
				if kind == TagFeed {
					id = data.Tag
				}
				if _, ok := feeds.Feed(kind, id); ok {
					pages = pages.Add(page)
					break
				}
			}
		}
	}
	return pages
}

func (feed *Feed) selectPages(pages Pages) Pages {
	result := Pages{}
	for _, page := range pages {
		if page != nil && slices.Contains(FeedVisible, page.Visibility) && matchesFeedConfig(page, feed.config) {
			result = append(result, page)
		}
	}
	sortFeedPages(result, feed.config.Sort)
	return result
}

func matchesFeedConfig(page *Page, config FeedConfig) bool {
	included := len(config.Include) == 0
	for _, selector := range config.Include {
		if selector.Matches(page) {
			included = true
			break
		}
	}
	if !included {
		return false
	}
	for _, selector := range config.Exclude {
		if selector.Matches(page) {
			return false
		}
	}
	return true
}

func (selector FeedSelector) Matches(page *Page) bool {
	if selector.Path != "" {
		if expression, err := hsync.ParsePathExpression(selector.Path); err == nil && expression.Matches(page.PublicURL.Path) {
			return true
		}
	}
	return slices.ContainsFunc(selector.Tags, func(tag string) bool {
		return slices.ContainsFunc(page.Tags, func(value string) bool { return strings.EqualFold(tag, value) })
	})
}

func sortFeedPages(pages Pages, rules []FeedSort) {
	slices.SortStableFunc(pages, func(a, b *Page) int {
		for _, rule := range rules {
			for field, direction := range rule {
				if value := cmp.Compare(a.ComparedBy(field), b.ComparedBy(field)); value != 0 {
					if strings.EqualFold(direction, "asc") {
						return value
					}
					return -value
				}
			}
		}
		if len(rules) == 0 {
			if value := cmp.Compare(b.LastModified().UnixMilli(), a.LastModified().UnixMilli()); value != 0 {
				return value
			}
		}
		return cmp.Compare(a.PublicURL.Path, b.PublicURL.Path)
	})
}

func feedItemViews(pages Pages) []httt.TemplateData {
	items := make([]httt.TemplateData, 0, len(pages))
	for _, page := range pages {
		item := page.TemplateData.Clone()
		delete(item, "feeds")
		items = append(items, item)
	}
	return items
}
