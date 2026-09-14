package htxt

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
	"gopkg.in/yaml.v3"

	"github.com/herdworks/httt"
	htfiles "github.com/herdworks/httt/files"
	"github.com/herdworks/hyperfeed/atom"
)

// Page
//
// Page objects do the heavy lifting to parse content data files
// and convert them into rendered pages.
type Page struct {
	// Source provenance is runtime-only. It never appears in page front matter
	// or template data, but callers may construct custom pages with it.
	// Format is supplied by a content type or inferred from Source.Filename.
	// It may differ from Source.Filename's extension until persistence succeeds.
	Source    PageSource `json:"-" yaml:"-"`
	PublicURL url.URL    `json:"-" yaml:"-"`
	Format    string     `json:"-" yaml:"-"`

	// Template data properties
	CreatedAt    string             `json:"created_at" yaml:"created_at" jsonschema:"format=date-time,description=Set when the page is finalized."`
	UpdatedAt    string             `json:"updated_at" yaml:"updated_at" jsonschema:"format=date-time"`
	LayoutRef    string             `json:"layout" yaml:"layout" jsonschema:"default=default,description=Defaults to 'default'."`
	Title        string             `json:"title,omitempty" yaml:"title,omitempty"`
	Description  string             `json:"description,omitempty" yaml:"description,omitempty"`
	Content      string             `json:"content,omitempty" yaml:"content,omitempty" jsonschema:"description=In Markdown files this is the Markdown body not a YAML front-matter property."`
	Summary      string             `json:"summary,omitempty" yaml:"summary,omitempty"`
	More         string             `json:"more,omitempty" yaml:"more,omitempty"`
	Byline       *Byline            `json:"byline,omitempty" yaml:"byline,omitempty"`
	Contributors Bylines            `json:"contributors,omitempty" yaml:"contributors,omitempty"`
	Attachments  Attachments        `json:"attachments,omitempty" yaml:"attachments,omitempty"`
	Metadata     []HTMLMeta         `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Links        []HTMLLink         `json:"links,omitempty" yaml:"links,omitempty"`
	Tags         []string           `json:"tags,omitempty" yaml:"tags,omitempty"`
	Feeds        FeedDataset        `json:"feeds,omitempty" yaml:"feeds,omitempty" jsonschema:"description=Named feeds rendered by this page."`
	Visibility   PageVisibilityKind `json:"visibility,omitempty" yaml:"visibility,omitempty" jsonschema:"default=public,description=Controls page visibility."`
	Redirect     string             `json:"redirect,omitempty" yaml:"redirect,omitempty" jsonschema:"format=uri-reference,description=The URI reference written into the page refresh redirect target."`
	Slug         string             `json:"slug,omitempty" yaml:"slug,omitempty"`
	UglyURL      bool               `json:"ugly_url,omitempty" yaml:"ugly_url,omitempty"`

	// Computed properties
	TemplateData httt.TemplateData `json:"-" yaml:"-"`
	Assets       []string          `json:"-" yaml:"-"` // paths relative to Source.Directory
	Static       bool              `json:"-" yaml:"-"`
	Loaded       bool              `json:"-" yaml:"-"`
}
type Pages []*Page

type PageSource struct {
	Directory string
	Path      string
	Filename  string
}

type PageVisibilityKind string

const (
	AutoVisibility     PageVisibilityKind = ""         // PublicVisibility alias
	DefaultVisibility  PageVisibilityKind = "default"  // PublicVisibility alias
	PublicVisibility   PageVisibilityKind = "public"   //
	UnlistedVisibility PageVisibilityKind = "unlisted" //
	WebsiteVisibility  PageVisibilityKind = "website"  // UnlistedVisibility alias
	StatusVisibility   PageVisibilityKind = "status"   //
	FeedVisibility     PageVisibilityKind = "feed"     // StatusVisibility alias
	StoryVisibility    PageVisibilityKind = "story"    // StatusVisibility alias
	ChatVisibility     PageVisibilityKind = "chat"     // StatusVisibility alias
	MessageVisibility  PageVisibilityKind = "message"  // StatusVisibility alias
	ResponseVisibility PageVisibilityKind = "response" // StatusVisibility alias
	ReplyVisibility    PageVisibilityKind = "reply"    // StatusVisibility alias
	DraftVisibility    PageVisibilityKind = "draft"    //
	PreviewVisibility  PageVisibilityKind = "preview"  // DraftVisibility alias
	NoVisibility       PageVisibilityKind = "none"     //
	HiddenVisibility   PageVisibilityKind = "hidden"   // NoVisibilty alias
)

var (
	PublicVisibilityKinds   = []PageVisibilityKind{PublicVisibility, DefaultVisibility, AutoVisibility}
	UnlistedVisibilityKinds = []PageVisibilityKind{UnlistedVisibility, WebsiteVisibility}
	StatusVisibilityKinds   = []PageVisibilityKind{StatusVisibility, FeedVisibility, StoryVisibility, ChatVisibility, MessageVisibility, ResponseVisibility, ReplyVisibility}
	DraftVisibilityKinds    = []PageVisibilityKind{DraftVisibility, PreviewVisibility}
	NoVisibilityKinds       = []PageVisibilityKind{NoVisibility, HiddenVisibility}

	WebsiteVisible           = slices.Concat(PublicVisibilityKinds, UnlistedVisibilityKinds)
	FeedVisible              = slices.Concat(PublicVisibilityKinds, StatusVisibilityKinds)
	FeedPreviewVisible       = slices.Concat(PublicVisibilityKinds, StatusVisibilityKinds, DraftVisibilityKinds)
	BuildVisible             = slices.Concat(PublicVisibilityKinds, UnlistedVisibilityKinds, StatusVisibilityKinds)
	BuildPreviewVisible      = slices.Concat(PublicVisibilityKinds, UnlistedVisibilityKinds, StatusVisibilityKinds, DraftVisibilityKinds)
	SupportedVisibilityKinds = slices.Concat(
		PublicVisibilityKinds,
		UnlistedVisibilityKinds,
		StatusVisibilityKinds,
		DraftVisibilityKinds,
		NoVisibilityKinds,
	)
)

// Page.Parse(data httt.TemplateData)
//
// Hydrates page properties from template data.
// Parse first marshals the data to YAML bytes, then unmarshals the YAML to the page.
func (page *Page) Parse(data httt.TemplateData) (err error) {
	if err = page.Screen(data); err != nil {
		return err
	}
	var yamlBytes []byte
	page.TemplateData = data
	if yamlBytes, err = yaml.Marshal(data); err != nil {
		return err
	}
	return yaml.Unmarshal(yamlBytes, &page)
}

// Screen page TemplateData for unsupported parameters (e.g. page.draft, page.unlisted, and page.feed)
func (page *Page) Screen(data httt.TemplateData) error {
	uri := page.Source.URI()
	uriPath := ""
	if uri != nil {
		uriPath = uri.Path
	}
	if _, ok := data.Get("draft").(bool); ok {
		return fmt.Errorf("page %q screened: page.draft is no longer supported; use page.visibility = \"draft\"", uriPath)
	}
	if _, ok := data.Get("unlisted").(bool); ok {
		return fmt.Errorf("page %q screened: page.unlisted is no longer supported; use page.visibility = \"unlisted\"", uriPath)
	}
	if data.Get("feed") != nil {
		return fmt.Errorf("page %q screened: page.feed is no longer supported; use page.feeds", uriPath)
	}
	return nil
}

// Page.Validate()
//
// Validates that the page has all properties required for processing.
func (page *Page) Validate() (err error) {
	uri := page.Source.URI()
	if uri == nil {
		return fmt.Errorf("page source state missing")
	}
	if page.CreatedAt == "" && page.UpdatedAt == "" {
		return fmt.Errorf("page.created_at or page.updated_at missing (%s)", uri.Path)
	}
	if strings.Contains(page.Slug, "/") {
		return fmt.Errorf("page.slug cannot contain a slash (%s)", page.Slug)
	}
	if page.Byline != nil && !page.Byline.Ready() {
		return fmt.Errorf("page.byline invalid (%s)", uri.Path)
	}
	if err := page.validateTagNames(); err != nil {
		return err
	}
	if err := page.Visibility.Validate(); err != nil {
		return err
	}
	return nil
}

func (page *Page) Clone() *Page {
	if page == nil {
		return nil
	}
	clone := *page
	clone.Byline = page.Byline.Clone()
	clone.Contributors = page.Contributors.Clone()
	clone.Attachments = page.Attachments.Clone()
	clone.Metadata = append([]HTMLMeta(nil), page.Metadata...)
	clone.Links = append([]HTMLLink(nil), page.Links...)
	clone.Tags = append([]string(nil), page.Tags...)
	clone.Feeds = page.Feeds.Clone()
	clone.Assets = append([]string(nil), page.Assets...)
	if page.TemplateData != nil {
		clone.TemplateData = page.TemplateData.Clone()
	}
	return &clone
}

// Page.Delete()
//
// Deletes the page from disk. If the page has child pages, the page files are
// deleted but the directory is retained.
// TODO: Make deletion website-scoped so it can validate page sources before
// removing files.
func (page *Page) Delete(w io.Writer) (err error) {
	if w == nil {
		w = os.Stdout
	}

	uri := page.Source.URI()
	if uri == nil {
		return fmt.Errorf("page.delete requires page source state")
	}
	fmt.Fprintf(w, "page \"%s\" delete\n", uri.Path)
	if page.HasChildPages() {
		fmt.Fprintf(w, "page \"%s\" files will be deleted (descendents will be preserved)\n", filepath.Dir(uri.Path))
		if err = page.DeleteFiles(w); err != nil {
			return err
		}
	} else {
		fmt.Fprintf(w, "page \"%s\" directory will be deleted (no descendents to be preserved)\n", filepath.Dir(uri.Path))
		if err := os.RemoveAll(filepath.Dir(uri.Path)); err != nil {
			return err
		}
	}
	return nil
}
func (page *Page) DeleteFiles(w io.Writer) (err error) {
	if w == nil {
		w = os.Stdout
	}

	uri := page.Source.URI()
	if uri == nil {
		return fmt.Errorf("page.delete requires page source state")
	}
	var entries []os.DirEntry
	if entries, err = os.ReadDir(filepath.Dir(uri.Path)); err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			filePath := filepath.Join(filepath.Dir(uri.Path), entry.Name())
			fmt.Fprintf(w, "file \"%s\" delete\n", filePath)
			if err := os.Remove(filePath); err != nil {
				return err
			}
		}
	}
	return nil
}

func (page *Page) CanonicalURL(website *Website) (*url.URL, error) {
	if page == nil || page.PublicURL.Path == "" || website == nil {
		return nil, fmt.Errorf("page canonical URL requires page for page.path and website for website.URL()")
	}
	if page.PublicURL.Scheme != "" || page.PublicURL.Opaque != "" || page.PublicURL.Host != "" || page.PublicURL.User != nil {
		return nil, fmt.Errorf("page canonical URL requires a website-relative page public URL")
	}
	return website.URL().ResolveReference(&page.PublicURL), nil
}

// Page.ApplyComputedProperties()
//
// Applies computed properties to the page template data.
func (page *Page) ApplyComputedProperties(website *Website) {
	if canonicalURL, err := page.CanonicalURL(website); err == nil {
		page.TemplateData.Set("canonical_url", canonicalURL.String())
	}
	if len(page.Slug) > 0 {
		page.TemplateData.Set("slug", page.Slug)
	} else {
		delete(page.TemplateData, "slug")
	}
	if len(page.Assets) > 0 {
		page.TemplateData.Set("assets", page.AssetNames())
	}
	if len(page.Feeds) > 0 {
		page.TemplateData.Set("feeds", page.Feeds.TemplateData())
	} else {
		delete(page.TemplateData, "feeds")
	}
}

// Page.ResolveTemplateData()
//
// Resolves non-empty page struct fields into page.TemplateData.
// TODO: merge ResolveTemplateData() with ApplyComputedProperties()
func (page *Page) ResolveTemplateData(website *Website) {
	if page.TemplateData == nil {
		page.TemplateData = httt.TemplateData{}
	}
	if len(page.CreatedAt) > 0 {
		page.TemplateData.Set("created_at", page.CreatedAt)
	}
	if len(page.UpdatedAt) > 0 {
		page.TemplateData.Set("updated_at", page.UpdatedAt)
	}
	if len(page.LayoutRef) > 0 {
		page.TemplateData.Set("layout", page.LayoutRef)
	}
	if len(page.Visibility) > 0 {
		page.TemplateData.Set("visibility", page.Visibility)
	}
	if len(page.Title) > 0 {
		page.TemplateData.Set("title", page.Title)
	}
	if len(page.Description) > 0 {
		page.TemplateData.Set("description", page.Description)
	}
	if page.Byline != nil {
		page.TemplateData.Set("byline", page.Byline.TemplateData())
	}
	if len(page.Contributors) > 0 {
		page.TemplateData.Set("contributors", page.Contributors.TemplateData())
	}
	if len(page.Tags) > 0 {
		page.TemplateData.Set("tags", page.Tags)
	}
	if len(page.Attachments) > 0 {
		page.TemplateData.Set("attachments", page.Attachments.TemplateData())
	}
	if len(page.Assets) > 0 {
		page.TemplateData.Set("assets", page.AssetNames())
	}
	if len(page.Summary) > 0 {
		page.TemplateData.Set("summary", page.Summary)
	}
	if len(page.More) > 0 {
		page.TemplateData.Set("more", page.More)
	}
	if page.PublicURL.Path != "" {
		page.TemplateData.Set("path", page.PublicURL.Path)
	}
	if page.UglyURL {
		page.TemplateData.Set("ugly_url", page.UglyURL)
	}
	if len(page.Redirect) > 0 {
		page.TemplateData.Set("redirect", page.Redirect)
	}
	page.ApplyComputedProperties(website)
}
func (pages Pages) ResolveTemplateData(website *Website) {
	for i := range pages {
		pages[i].ResolveTemplateData(website)
	}
}

// Pages.LastModified()
//
// Returns the last modified timestamp (page.UpdatedAt) of the newest page.
// If the page has not been modified, it returns the creation timestamp (page.CreatedAt).
func (pages Pages) LastModified() (timestamp time.Time) {
	timestamp, _ = time.Parse(time.RFC3339, "1970-01-01T00:00:00Z") // default timestamp
	if len(pages) == 0 {
		return timestamp
	} else {
		return pages[0].LastModified()
	}
}
func (page *Page) LastModified() (ts time.Time) {
	var value string
	var err error
	if value = page.UpdatedAt; value == "" {
		value = page.CreatedAt
	}
	if ts, err = time.Parse(time.RFC3339, value); err != nil {
		ts, _ = time.Parse(time.RFC3339, "1970-01-01T00:00:00Z")
	}
	return ts
}

func (page *Page) publishedAt() string {
	if page == nil || page.CreatedAt == "" {
		if page == nil {
			return ""
		}
		return page.UpdatedAt
	}
	return page.CreatedAt
}

func (pages Pages) Add(incoming ...*Page) Pages {
	for _, page := range incoming {
		if page != nil && !pages.Contains(page) {
			pages = append(pages, page)
		}
	}
	return pages
}

func (pages Pages) Contains(page *Page) bool {
	return page != nil && pages.PageByURI(page.Source.URI()) != nil
}

// PageByURI returns the page whose source URI path matches uri.Path.
func (pages Pages) PageByURI(uri *url.URL) *Page {
	if uri == nil {
		return nil
	}
	index := slices.IndexFunc(pages, func(page *Page) bool {
		pageURI := page.Source.URI()
		return pageURI != nil && strings.EqualFold(pageURI.Path, uri.Path)
	})
	if index > -1 {
		return pages[index]
	}
	return nil
}

// ByPublicURLPath returns the page whose declarative public URL path matches path.
func (pages Pages) ByPublicURLPath(path string) *Page {
	index := slices.IndexFunc(pages, func(page *Page) bool {
		return page != nil && strings.EqualFold(page.PublicURL.Path, path)
	})
	if index > -1 {
		return pages[index]
	}
	return nil
}

func (pages Pages) Sort() {
	slices.SortFunc(pages, func(a, b *Page) int {
		return cmp.Compare(b.LastModified().UnixMilli(), a.LastModified().UnixMilli())
	})
}
func (pages Pages) SortBy(field string, ascending bool) {
	slices.SortFunc(pages, func(a, b *Page) int {
		if ascending {
			return cmp.Compare(a.ComparedBy(field), b.ComparedBy(field))
		}
		return cmp.Compare(b.ComparedBy(field), a.ComparedBy(field))
	})
}

// NOTE: only called by hyperctl cms page ls -> page.SortBy() -> page.ComparedBy()
func (page *Page) ComparedBy(field string) string {
	switch field {
	case "CreatedAt", "created_at":
		return page.CreatedAt
	case "UpdatedAt", "updated_at":
		return page.UpdatedAt
	case "Title", "title":
		return page.Title
	case "Path", "path":
		return page.PublicURL.Path
	default:
		return page.LastModified().Format(time.RFC3339)
	}
}

func (page *Page) HasChildPages() bool {
	uri := page.Source.URI()
	if uri == nil {
		return false
	}
	entries, err := os.ReadDir(filepath.Dir(uri.Path))
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() {
			return true
		}
	}
	return false
}

func (pages Pages) ColumnValues(field string) []string {
	values := []string{}
	for i := range pages {
		switch field {
		case "Path", "path":
			values = append(values, pages[i].PublicURL.Path)
		case "Title", "title":
			values = append(values, pages[i].Title)
		case "LastModified", "lastmod":
			values = append(values, pages[i].LastModified().Format(time.RFC3339))
		case "Visibility", "visibility":
			if pages[i].Visibility == AutoVisibility {
				// return "public" if page.visibility is not set
				values = append(values, string(PublicVisibility))
			} else {
				// return page.visibility
				values = append(values, string(pages[i].Visibility))
			}
		}
	}
	return values
}

func (pages Pages) FilterDraft(draft bool) Pages {
	filtered := Pages{}
	for _, page := range pages {
		if page != nil && page.Visibility.IsDraft() == draft {
			filtered = append(filtered, page)
		}
	}
	return filtered
}

func (pages Pages) TemplateData() (data []httt.TemplateData) {
	for i := range pages {
		data = append(data, pages[i].TemplateData)
	}
	return data
}

// TemplateDataWithout returns each page's TemplateData with named top-level
// keys omitted.
func (pages Pages) TemplateDataWithout(keys ...string) (data []httt.TemplateData) {
	for i := range pages {
		data = append(data, pages[i].TemplateData.Without(keys...))
	}
	return data
}

func (pages Pages) DiscoverAssets(website *Website, verbose bool, w io.Writer) error {
	if w == nil {
		w = os.Stdout
	}

	for i := range pages {
		if err := pages[i].DiscoverAssets(website, verbose, w); err != nil {
			return err
		}
	}
	return nil
}
func (page *Page) DiscoverAssets(website *Website, verbose bool, w io.Writer) error {
	if w == nil {
		w = os.Stdout
	}

	uri := page.Source.URI()
	if uri == nil {
		return fmt.Errorf("page.assets requires page source state")
	}
	pageDir := filepath.Dir(uri.Path)
	files, err := htfiles.List(pageDir, false)
	if err != nil {
		return err
	}
	pageDirRel := filepath.ToSlash(page.Source.Path)
	assets := []string{}
	for _, file := range files {
		if file == page.Source.Filename {
			continue
		}
		relPath := filepath.ToSlash(filepath.Join(pageDirRel, file))
		if _, err = htfiles.CleanRelativePath(relPath); err != nil {
			return err
		}
		assets = append(assets, relPath)
	}
	page.Assets = assets
	if !verbose {
		return nil
	}
	for _, asset := range page.Assets {
		if srcPath, err := website.ResourcePath(page.Source.Directory, asset); err == nil {
			fmt.Fprintf(w, "asset \"%s\" discovered\n", website.RelativePath(srcPath))
		}
	}
	return nil
}

func (page *Page) AssetNames() (names []string) {
	for _, asset := range page.Assets {
		names = append(names, filepath.Base(asset))
	}
	return names
}

func (page Page) AtomEntry(website *Website) (entry *atom.Entry) {
	var atomLinks []atom.Link
	var atomIDURL *url.URL
	var atomAuthor *atom.Author
	var atomContributors []*atom.Author
	var err error
	if atomLinks, err = page.AtomEntryLinks(website); err != nil {
		return nil
	}
	if atomIDURL, err = page.CanonicalURL(website); err != nil {
		return nil
	}
	if atomAuthor, err = page.AtomEntryAuthor(website); err != nil {
		return nil
	}
	if atomContributors, err = page.AtomEntryContributors(website); err != nil {
		return nil
	}
	return &atom.Entry{
		Title:        page.AtomEntryTitle(),
		ID:           atomIDURL.String(),
		Links:        atomLinks,
		Published:    atom.TimeStamp(page.publishedAt()),
		Updated:      atom.TimeStamp(page.UpdatedAt),
		Authors:      []*atom.Author{atomAuthor},
		Contributors: atomContributors,
		Summary:      page.AtomSummary(),
		Content:      page.AtomContent(),
	}
}

func (page *Page) AtomEntryLinks(website *Website) (atomLinks []atom.Link, err error) {
	var atomURL *url.URL
	if atomURL, err = page.CanonicalURL(website); err != nil {
		return nil, err
	}
	if slices.Contains(WebsiteVisible, page.Visibility) {
		atomLinks = append(atomLinks, atom.Link{Rel: "alternate", Type: "text/html", Href: atomURL.String()})
	}
	for _, attachment := range page.Attachments {
		if attachment == nil || attachment.Href == nil || attachment.Href.String() == "" {
			continue
		}
		attachmentURL := atomURL.ResolveReference(&attachment.Href.URL)
		switch {
		case attachment.IsLink():
			atomLinks = append(atomLinks, atom.Link{Rel: "related", Type: "text/html", Href: attachmentURL.String(), Title: attachment.Title()})
		case attachment.IsEnclosure():
			link := atom.Link{Rel: "enclosure", Href: attachmentURL.String(), Type: attachment.AtomMIMEType(attachmentURL), Title: attachment.Title()}
			if length := page.AtomAttachmentLength(attachment); length > 0 {
				link.Length = length
			}
			atomLinks = append(atomLinks, link)
		}
	}
	return atomLinks, nil
}

func (Page) JSONSchemaTitle() string {
	return "HyperTexting page"
}
func (Page) JSONSchemaName() string    { return "page" }
func (Page) JSONSchemaSummary() string { return "Page source data file." }

func (Page) JSONSchemaDescription() string {
	return "A page source file may use Markdown, YAML, or JSON. For Markdown files, object properties form YAML front matter and the Markdown body is the 'content' field."
}

func (page *Page) AtomAttachmentLength(attachment *Attachment) uint {
	uri := page.Source.URI()
	if uri == nil || attachment == nil || attachment.Href == nil {
		return 0
	}
	ref := &attachment.Href.URL
	if ref.IsAbs() || ref.Path == "" || strings.HasPrefix(ref.Path, "/") {
		return 0
	}
	pageDir := filepath.Dir(uri.Path)
	filePath := filepath.Clean(filepath.Join(pageDir, filepath.FromSlash(ref.Path)))
	relPath, err := filepath.Rel(pageDir, filePath)
	if err != nil || relPath == ".." || strings.HasPrefix(relPath, ".."+string(os.PathSeparator)) || filepath.IsAbs(relPath) {
		return 0
	}
	info, err := os.Stat(filePath)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 {
		return 0
	}
	return uint(info.Size())
}

func (page *Page) AtomEntryAuthor(website *Website) (person *atom.Author, err error) {
	if page.Byline == nil {
		page.Byline = &website.Byline
	}
	return atomPerson(website, page.Byline)
}

func (page *Page) AtomEntryContributors(website *Website) (contributors []*atom.Author, err error) {
	return atomPersons(website, page.Contributors)
}

func (page *Page) AtomEntryTitle() (title string) {
	if len(page.Title) > 0 {
		return page.Title
	}
	return ""
}

func (page *Page) AtomSummary() (summary atom.Text) {
	if len(page.Summary) > 0 {
		return atom.Text{Type: "html", Body: page.Summary}
	}
	if len(page.Content) > 0 {
		return atom.Text{Type: "html", Body: page.Content}
	}
	if links := page.Attachments.FilteredBy(AttachmentKindLink); len(links) > 0 {
		description, ok := links[0].Data.Get("hypergraph.description").(string)
		if ok && len(strings.TrimSpace(description)) > 0 {
			return atom.Text{Type: "html", Body: description}
		}
	}
	publishedAt := page.publishedAt()
	ts, err := time.Parse(time.RFC3339, publishedAt)
	if err != nil {
		return atom.Text{Type: "html", Body: publishedAt}
	}
	return atom.Text{Type: "html", Body: ts.Format("Monday, January 2, 2006")}
}
func (page *Page) AtomContent() (content atom.Text) {
	if len(page.Content) > 0 {
		return atom.Text{Type: "html", Body: page.Content}
	}
	return atom.Text{Type: "html", Body: ""}
}

func (page *Page) MarshalYAML() (any, error) {
	return struct {
		Page  *Page             `yaml:",inline"`
		Other httt.TemplateData `yaml:",inline"`
	}{
		Page:  page,
		Other: page.TemplateData.Without("created_at", "updated_at", "layout", "slug", "author", "contributors", "title", "description", "summary", "preview", "tags", "visibility", "ugly_url", "redirect", "attachments", "feeds"),
	}, nil
}

// Save persists page data at its resolved source destination. Format selects
// the serialized data format and destination filename; Source is updated after
// persistence succeeds.
func (page *Page) Save(website *Website, w io.Writer) error {
	return savePage(website, page, w)
}

// RenderData serializes page data using the requested format.
//
// RenderData mutates page by resolving struct fields into TemplateData, and by
// setting TemplateData["content"] for JSON and YAML output. Clone the page first
// when the original TemplateData must be preserved.
func (page *Page) RenderData(website *Website, format string) (fileBytes []byte, err error) {
	if page == nil {
		return nil, fmt.Errorf("page.render requires page")
	}

	var frontmatter []byte // YAML frontmatter (for markdown format only)
	page.ResolveTemplateData(website)
	switch strings.ToLower(format) {
	case "json":
		page.TemplateData.Set("content", page.Content) // add content to template data
		if fileBytes, err = json.Marshal(page.TemplateData); err != nil {
			return nil, err
		}
	case "yaml", "yml":
		page.TemplateData.Set("content", page.Content) // add content to template data
		if fileBytes, err = yaml.Marshal(page.TemplateData); err != nil {
			return nil, err
		}
	case "markdown", "md":
		fallthrough
	default:
		if frontmatter, err = yaml.Marshal(OrderedTemplateData(page.TemplateData)); err != nil {
			return nil, err
		}
		fileBytes = fmt.Appendf([]byte{}, "---\n%s---\n\n%s", frontmatter, page.Content)
	}
	return fileBytes, nil
}

// Update applies a content type's page-object transition through website
// context. Persistence remains a separate Save operation.
func (page *Page) Update(website *Website, contenttype *ContentType) error {
	return updatePage(website, contenttype, page)
}

// ValidateContentType validates page against contenttype without writing it.
func (page *Page) ValidateContentType(website *Website, contenttype *ContentType) error {
	return validatePage(website, contenttype, page)
}

// Prepare returns the page state and canonical content source that
// Finalize would produce without moving or writing files.
func (page *Page) Prepare(website *Website, contenttype *ContentType, createdAt string) (*Page, error) {
	candidate, destination, err := preparePage(website, contenttype, page, createdAt)
	if err != nil {
		return nil, err
	}
	candidate.Source = destination
	return candidate, nil
}

// Finalize validates and moves a page into the website's canonical content root.
// When overwrite is true, it clears direct destination files before moving the
// page bundle.
func (page *Page) Finalize(website *Website, contenttype *ContentType, createdAt string, overwrite bool, w io.Writer) error {
	return finalizePage(website, contenttype, page, createdAt, overwrite, w)
}

func (page *Page) validateTagNames() error {
	for _, tag := range page.Tags {
		if err := ValidateTagName(tag); err != nil {
			return fmt.Errorf("page.tags: %w", err)
		}
	}
	return nil
}

func (page *Page) resolvePublicURLPageSource(directory string) (source PageSource) {
	var publicURLPath, format, dstPath, dstFilename string
	var err error
	if strings.TrimSpace(directory) == "" {
		directory = page.Source.Directory
	}
	if publicURLPath = page.PublicURL.Path; publicURLPath == "" {
		publicURLPath = page.Source.PublicURLPath(page.UglyURL)
	}
	if publicURLPath == "/index.html" && page.UglyURL {
		dstPath = ""
	} else {
		dstPath = filepath.FromSlash(strings.TrimSuffix(strings.Trim(publicURLPath, "/"), ".html"))
	}
	if format = page.Format; format == "" {
		format = page.Source.FilenameFormat()
	}
	if dstFilename, err = pageDataFilename(format); err != nil {
		dstFilename = "index.md"
	}
	return PageSource{
		Directory: directory,
		Path:      dstPath,
		Filename:  dstFilename,
	}
}

// URI returns the page's derived local source URI.
func (source *PageSource) URI() *url.URL {
	if source == nil || source.Directory == "" || source.Filename == "" {
		return nil
	}
	return &url.URL{Path: filepath.Join(source.Directory, source.Path, source.Filename)}
}

// Dir returns the page's source directory.
func (source *PageSource) Dir() string {
	return filepath.Join(source.Directory, source.Path)
}

// FilenameFormat returns Filename's extension without its dot.
func (source *PageSource) FilenameFormat() string {
	if source == nil {
		return ""
	}
	return strings.TrimPrefix(filepath.Ext(source.Filename), ".")
}

func (source PageSource) matches(other PageSource) bool {
	return source.URI() != nil &&
		other.URI() != nil &&
		source.Dir() == other.Dir() &&
		source.Filename == other.Filename
}

func (source PageSource) PublicURLPath(ugly bool) string {
	urlPath := strings.TrimSuffix(path.Join("/", filepath.ToSlash(source.Path)), "/") + "/"
	if !ugly {
		return urlPath // the most common outcome
	}
	if urlPath == "/" {
		return "/index.html" // only once per website
	}
	return strings.TrimSuffix(urlPath, "/") + ".html" // zero or more times per website
}

func (source *PageSource) Exists() (info os.FileInfo, exists bool, err error) {
	var sourceURI *url.URL
	if source == nil {
		return nil, false, nil
	}
	if sourceURI = source.URI(); sourceURI == nil {
		return nil, false, nil
	}
	if info, err = os.Stat(sourceURI.Path); os.IsNotExist(err) {
		return nil, false, nil
	} else if err != nil {
		return nil, false, err
	}
	if !info.Mode().IsRegular() {
		return nil, false, fmt.Errorf("source %q error: not a regular file", sourceURI.Path)
	}
	return info, true, nil
}

// Available reports whether source's directory is available for the source
// data file. When allowSiblings is false, it requires the directory to contain
// no direct regular files. When true, only the source data file itself must be
// absent.
func (source *PageSource) Available(allowSiblings bool) (available bool, err error) {
	var sourceURI = source.URI()
	var paths []string
	if sourceURI == nil {
		return false, fmt.Errorf("source state missing")
	}
	if allowSiblings {
		if _, exists, err := source.Exists(); !exists {
			return err == nil, err
		}
		return false, fmt.Errorf("source %q unavailable: page file exists at %s", source.Directory, sourceURI.Path)
	}
	if paths, err = htfiles.List(filepath.Dir(sourceURI.Path), false); err != nil {
		return false, err
	}
	if len(paths) == 0 {
		return true, nil
	}
	return false, fmt.Errorf("source %q unavailable: found %d file(s) (%s)", source.Directory, len(paths), strings.Join(paths, ", "))
}

func (visibility PageVisibilityKind) Validate() error {
	if !slices.Contains(SupportedVisibilityKinds, visibility) {
		return fmt.Errorf("page.visibility is invalid: %q", visibility)
	}
	return nil
}

func (visibility PageVisibilityKind) IsPublic() bool {
	return slices.Contains(PublicVisibilityKinds, visibility)
}

func (visibility PageVisibilityKind) IsUnlisted() bool {
	return slices.Contains(UnlistedVisibilityKinds, visibility)
}

func (visibility PageVisibilityKind) IsStatus() bool {
	return slices.Contains(StatusVisibilityKinds, visibility)
}

func (visibility PageVisibilityKind) IsDraft() bool {
	return slices.Contains(DraftVisibilityKinds, visibility)
}

func (visibility PageVisibilityKind) IsNone() bool {
	return slices.Contains(NoVisibilityKinds, visibility)
}

// MARK: FUNCTIONS

// LoadPages(ctx context.Context, website *Website, pages Pages, cache *Pages, verbose bool)
//
// Loads the pages in parallel using a task group.
func LoadPages(ctx context.Context, website *Website, pages Pages, cache Pages, verbose bool, w io.Writer) (err error) {
	if w == nil {
		w = os.Stdout
	}

	if cache == nil {
		cache = Pages{}
	}
	taskgroup, taskContext := errgroup.WithContext(ctx)
	for i := range pages {
		if err = errgroupCancellationCheckpoint(taskContext, taskgroup); err != nil {
			return err
		}
		taskgroup.Go(func() error {
			if err := contextCancellationCheckpoint(taskContext); err != nil {
				return err
			}
			if cache.Contains(pages[i]) {
				pages[i] = cache.PageByURI(pages[i].Source.URI())
			}
			return LoadPage(website, pages[i], verbose, w)
		})
	}
	return taskgroup.Wait()
}

// LoadPage(website *Website, page *Page, verbose bool)
//
// Loads the page from disk and parses the content into the page object.
func LoadPage(website *Website, page *Page, verbose bool, w io.Writer) (err error) {
	var uri *url.URL
	var fileBytes []byte
	var fileData httt.TemplateData
	if w == nil {
		w = os.Stdout
	}
	if page.Loaded {
		return // guard
	}
	if uri = page.Source.URI(); uri == nil {
		return fmt.Errorf("page.load requires page source state")
	}
	if verbose {
		fmt.Fprintf(w, "page \"%s\" loading\n", website.RelativePath(uri.Path))
	}
	if fileBytes, err = os.ReadFile(uri.Path); err != nil {
		return err
	}
	if fileData, err = httt.ParseTemplateDataBytes(fileBytes, filepath.Ext(uri.Path), website.Config.Markdown); err == nil {
		page.TemplateData = fileData
	} else {
		return err
	}
	if err = page.Parse(fileData); err != nil {
		return err
	}
	// TODO: Extract this into the shared website-aware page timestamp
	// normalization used by load, save, and preview paths.
	if page.CreatedAt == "" && page.UpdatedAt == "" {
		// Every page needs a timestamp, but we shouldn't assert CreatedAt provenance
		page.UpdatedAt = website.UpdatedAt
	}
	if err = page.Visibility.Validate(); err != nil {
		return err
	}
	if err = page.validateTagNames(); err != nil {
		return err
	}
	if page.PublicURL.Path == "" {
		page.PublicURL.Path = page.Source.PublicURLPath(page.UglyURL)
	}
	if page.LayoutRef == "" {
		page.LayoutRef = "default" // TODO: should layout be required?
	}
	if page.Format == "" {
		page.Format = page.Source.FilenameFormat()
	}
	if page.Visibility.IsStatus() {
		page.LayoutRef = website.Config.StatusLayout
		if page.Redirect == "" {
			page.Redirect = website.Config.StatusRedirect
		}
	}
	if err = page.DiscoverAssets(website, verbose, w); err != nil {
		return err
	}
	page.Loaded = true
	return nil
}

func GetPages(website *Website, directory string) (pages Pages, err error) {
	pages = Pages{} // "discovered pages"
	paths, err := htfiles.ListByPattern(directory, true, "(index|page)\\.(md|yaml|json|html)$", fmt.Sprintf("^%s/*", website.BuildsDirName()))
	if err != nil {
		return nil, err
	}
	for _, p := range paths {
		var absPath, relPath, sourcePath, sourceFilename string
		var static bool
		if absPath, err = htfiles.LocalPath(directory, p); err != nil {
			return nil, err
		}
		if relPath, err = htfiles.RelativePath(directory, absPath); err == nil {
			sourcePath = filepath.Dir(relPath)
			sourceFilename = filepath.Base(relPath)
		} else {
			return nil, err
		}
		if sourcePath == "." {
			sourcePath = ""
		}
		if path.Ext(sourceFilename) == ".html" {
			static = true // mark page as static if source file is HTML (i.e. skip template processing and write directly to output)
		}
		page := &Page{
			Source: PageSource{
				Directory: directory,
				Path:      sourcePath,
				Filename:  sourceFilename,
			},
			Format:       strings.TrimPrefix(path.Ext(sourceFilename), "."),
			Static:       static,
			TemplateData: nil,
		}
		pages = append(pages, page)
	}
	return pages, nil
}

// createPage initializes an unsaved, content-type-backed page at sourceDirectory.
// It delegates to updatePage for consistent state, timestamp, and public-path
// resolution.
func createPage(website *Website, contenttype *ContentType, sourceDirectory string) (page *Page, err error) {
	if website == nil {
		return nil, fmt.Errorf("website is required to create a page")
	}
	if contenttype == nil {
		return nil, fmt.Errorf("content type is required to create a page")
	}
	if sourceDirectory == "" {
		return nil, fmt.Errorf("source directory is required to create a page")
	}
	page = &Page{Source: PageSource{Directory: sourceDirectory}}
	if err = updatePage(website, contenttype, page); err != nil {
		return nil, err
	}
	return page, nil
}

// updatePage applies the object-only transition shared by new and existing
// pages. Persistence is deliberately owned by savePage, so incomplete drafts
// can be updated without content-type readiness or file-system checks.
func updatePage(website *Website, contenttype *ContentType, page *Page) error {
	if website == nil || page == nil {
		return fmt.Errorf("page.update requires website and page")
	}
	if page.Source.Directory == "" {
		return fmt.Errorf("page.update requires page.source_dir")
	}
	if contenttype == nil {
		if page.Format == "" {
			page.Format = page.Source.FilenameFormat()
		}
		if page.Format != "" {
			if _, err := pageDataFilename(page.Format); err != nil {
				return err
			}
		}
	} else {
		if err := contenttype.Spec.Validate(); err != nil {
			return err
		}
		if err := applyPageContentType(page, contenttype); err != nil {
			return err
		}
	}
	page.UpdatedAt = time.Now().Format(time.RFC3339)
	return resolvePublicURLPath(website, contenttype, page)
}

// validatePage is the strict finalization gate: it validates the complete page
// object and then content-type readiness when a content type is selected.
func validatePage(website *Website, contenttype *ContentType, page *Page) error {
	if website == nil || page == nil {
		return fmt.Errorf("page.validate requires website and page")
	}
	if err := page.Validate(); err != nil {
		return err
	}
	if contenttype == nil {
		return nil
	}
	return contenttype.Ready(page)
}

// resolvePublicURLPath updates declarative PublicURL only. It never changes the
// current Source or performs persistence.
func resolvePublicURLPath(website *Website, contenttype *ContentType, page *Page) (err error) {
	var resolver PublicURLPathResolver
	if website == nil || page == nil {
		return fmt.Errorf("page.path resolution requires website and page")
	}
	if contenttype == nil {
		page.PublicURL.Path = page.Source.PublicURLPath(page.UglyURL)
		return nil
	}
	if err := contenttype.ValidateDefinition(); err != nil {
		return err
	}
	if resolver, err = PublicURLPathResolverFor(contenttype.Spec.PathResolver); err != nil {
		return err
	}
	if page.PublicURL.Path, err = resolver.ResolvePublicURLPath(*page, contenttype); err != nil {
		return err
	}
	return nil
}

// savePage persists at the page's current source root. It projects and
// validates the destination before moving files, then writes page data.
func savePage(website *Website, page *Page, w io.Writer) error {
	if website == nil || page == nil || page.Source.Directory == "" {
		return fmt.Errorf("page.save requires page source directory")
	}
	// TODO: Normalize a page with no timestamps from website.UpdatedAt before
	// serializing it, preserving the absence of CreatedAt provenance.
	var destination = page.resolvePublicURLPageSource(page.Source.Directory)
	if err := validatePageSources(website, page.Source, destination); err != nil {
		return err
	}
	if err := movePage(page, destination, false); err != nil {
		return err
	}
	return writePage(website, page, w)
}

// movePage performs a prevalidated same-root or cross-root file transition.
// Cross-root moves carry direct regular siblings; same-root format changes move
// only the page-data file. It commits Source only after that data file moves.
func movePage(page *Page, destination PageSource, overwrite bool) (err error) {
	var includeAssets bool = true
	if page == nil {
		return fmt.Errorf("page.move requires page")
	}
	if _, exists, err := page.Source.Exists(); err != nil {
		return err
	} else if !exists {
		return nil
	}
	if page.Source.matches(destination) {
		return nil // the path did not change, nothing to do
	}
	if page.Source.Dir() == destination.Dir() {
		includeAssets = false
	}
	if overwrite {
		if err = (&Page{Source: destination}).DeleteFiles(io.Discard); err != nil {
			return err
		}
	} else if available, err := destination.Available(!includeAssets); !available {
		return fmt.Errorf("page destination unavailable: %w", err)
	}

	// move page files
	var srcURI = page.Source.URI()
	var dstURI = destination.URI()
	if includeAssets {
		var srcDir = page.Source.Dir()
		var dstDir = destination.Dir()
		var paths []string
		if err = os.MkdirAll(dstDir, 0755); err != nil {
			return err
		}
		if paths, err = htfiles.List(srcDir, false); err != nil {
			return err
		}
		for _, path := range paths {
			var srcAssetPath, dstAssetPath string
			if filepath.Base(srcURI.Path) == path {
				continue // skip the page data file
			}
			if srcAssetPath, err = htfiles.LocalPath(srcDir, path); err != nil {
				return err
			}
			if dstAssetPath, err = htfiles.LocalPath(dstDir, path); err != nil {
				return err
			}
			if err = os.Rename(srcAssetPath, dstAssetPath); err != nil {
				return err
			}
		}
	}
	if err = os.Rename(srcURI.Path, dstURI.Path); err != nil {
		return err
	}
	// Remove only an empty source leaf; nested pages or unexpected files keep it.
	_ = os.Remove(page.Source.Dir())

	// update page state
	page.Source = destination
	return nil
}

// writePage serializes and writes page data at its projected destination. It
// records that destination as Source only after the write succeeds.
func writePage(website *Website, page *Page, w io.Writer) (err error) {
	var destination PageSource
	var destinationURI *url.URL
	var file *os.File
	var fileBytes []byte
	var fileBytesWritten int
	if w == nil {
		w = os.Stdout
	}

	if website == nil || page == nil || page.Source.Directory == "" {
		return fmt.Errorf("page.write requires page source directory")
	}
	if page.Format == "" {
		page.Format = page.Source.FilenameFormat()
	}
	if page.PublicURL.Path == "" {
		page.PublicURL.Path = page.Source.PublicURLPath(page.UglyURL)
	}
	destination = page.resolvePublicURLPageSource(page.Source.Directory)
	destinationURI = destination.URI()
	if fileBytes, err = page.RenderData(website, page.Format); err != nil {
		return err
	}
	fmt.Fprintf(w, "writing %d bytes to %s\n", len(fileBytes), website.RelativePath(destinationURI.Path))
	if err = os.MkdirAll(filepath.Dir(destinationURI.Path), 0755); err != nil {
		return err
	}
	if file, err = os.OpenFile(destinationURI.Path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644); err != nil {
		return err
	}
	defer file.Close()
	if fileBytesWritten, err = file.Write(fileBytes); err != nil {
		return err
	}
	if fileBytesWritten != len(fileBytes) {
		return fmt.Errorf("expected to write %v bytes, but wrote %v", len(fileBytes), fileBytesWritten)
	}
	page.Source = destination
	return nil
}

// finalizePage is the sole cross-root publish transition. It builds and
// validates a candidate before moving or writing, preserving the caller's draft
// page on a validation or destination-conflict failure.
func finalizePage(website *Website, contenttype *ContentType, page *Page, createdAt string, overwrite bool, w io.Writer) (err error) {
	var candidate *Page
	var destination PageSource
	if w == nil {
		w = os.Stdout
	}
	if candidate, destination, err = preparePage(website, contenttype, page, createdAt); err != nil {
		return err
	}

	// Finalization is the sole cross-root move: all prior saves remain in the
	// page's configured source directory, while this transition targets content.
	if err = movePage(candidate, destination, overwrite); err != nil {
		return err
	}
	if err = writePage(website, candidate, w); err != nil {
		return err
	}
	// Composer sidecars are draft-only. movePage relocates it with the other
	// regular page files, so delete it only after final persistence succeeds.
	sidecarPath := filepath.Join(filepath.Dir(candidate.Source.URI().Path), ".composer")
	if err = os.Remove(sidecarPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	// movePage leaves a draft leaf with its sidecar in place; remove it now that
	// finalization has removed that last draft-only file.
	_ = os.Remove(page.Source.Dir())
	*page = *candidate
	return nil
}

// preparePage returns a candidate that retains page.Source as the
// physical source bundle and a separate projected destination. Finalize needs
// both values to move that bundle safely; Prepare projects the
// destination onto its returned copy for callers that only need the result.
func preparePage(website *Website, contenttype *ContentType, page *Page, createdAt string) (candidate *Page, destination PageSource, err error) {
	timestamp := time.Now().Format(time.RFC3339)
	if page == nil {
		return nil, PageSource{}, fmt.Errorf("page.finalize requires a page object")
	}
	if page.Source.URI() == nil {
		return nil, PageSource{}, fmt.Errorf("page.finalize requires page source state")
	}
	if _, exists, err := page.Source.Exists(); err != nil {
		return nil, PageSource{}, err
	} else if !exists {
		return nil, PageSource{}, fmt.Errorf("page.finalize source does not exist: %s", page.Source.URI().Path)
	}
	candidate = page.Clone()
	if contenttype != nil {
		if err = contenttype.Spec.Validate(); err != nil {
			return nil, PageSource{}, err
		}
		if candidate, err = contenttype.OverlayPage(candidate); err != nil {
			return nil, PageSource{}, err
		}
		if err = applyPageContentType(candidate, contenttype); err != nil {
			return nil, PageSource{}, err
		}
	}
	if createdAt == "" {
		createdAt = timestamp
	}
	candidate.CreatedAt = createdAt
	candidate.UpdatedAt = timestamp
	if candidate.Visibility.IsDraft() {
		candidate.Visibility = ""
	}
	if candidate.TemplateData != nil {
		candidate.TemplateData.Delete("visibility")
	}
	if err = validatePage(website, contenttype, candidate); err != nil {
		return nil, PageSource{}, err
	}
	if err = resolvePublicURLPath(website, contenttype, candidate); err != nil {
		return nil, PageSource{}, err
	}
	destination = candidate.resolvePublicURLPageSource(website.ContentDirPath())
	if err = validatePageSources(website, candidate.Source, destination); err != nil {
		return nil, PageSource{}, err
	}
	return candidate, destination, nil
}

// applyPageContentType applies the direct page properties supplied by a content type.
//
// NOTE: Template data and prompt patches are applied separately by ContentType.OverlayPage.
func applyPageContentType(page *Page, contenttype *ContentType) (err error) {
	if page == nil || contenttype == nil {
		return nil
	}
	page.Format = contenttype.Spec.Format
	if contenttype.Spec.Layout != "" {
		page.LayoutRef = contenttype.Spec.Layout
	}

	page.Visibility = contenttype.Spec.Visibility
	return nil
}

// validatePageSources ensures persistence sources remain inside the website
// root. A current source may omit its filename while Save creates a new page;
// projected destinations always include a filename.
func validatePageSources(website *Website, sources ...PageSource) (err error) {
	var root string
	if website == nil || website.URI == nil {
		return fmt.Errorf("page source validation requires website URI")
	}
	if root, err = filepath.Abs(website.RootDir()); err != nil {
		return err
	}
	for _, source := range sources {
		var sourceDirectory, relativeDirectory string
		if source.Directory == "" {
			return fmt.Errorf("page source directory missing")
		}
		if sourceDirectory, err = filepath.Abs(source.Directory); err != nil {
			return err
		}
		if relativeDirectory, err = filepath.Rel(root, sourceDirectory); err != nil {
			return err
		}
		if relativeDirectory == ".." || strings.HasPrefix(relativeDirectory, ".."+string(filepath.Separator)) || filepath.IsAbs(relativeDirectory) {
			return fmt.Errorf("page source directory %q escapes website root %q", source.Directory, root)
		}
		if source.Filename == "" {
			continue
		}
		uri := source.URI()
		if uri == nil {
			return fmt.Errorf("page source state incomplete")
		}
		if _, err = htfiles.RelativePath(root, uri.Path); err != nil {
			return fmt.Errorf("page source %q escapes website root %q: %w", uri.Path, root, err)
		}
	}
	return nil
}

// mergePagesBySourcePath applies later-source-root precedence to persisted
// pages. Source.Path is each page's relative source-directory identity: pages
// at the same source path in a later root replace earlier-root pages.
func mergePagesBySourcePath(a, b Pages) Pages {
	merged := a
	for _, page := range b {
		// Later roots override earlier roots at the same relative source path.
		if index := slices.IndexFunc(merged, func(existing *Page) bool {
			return existing != nil && strings.EqualFold(existing.Source.Path, page.Source.Path)
		}); index >= 0 {
			merged[index] = page
			continue
		}
		merged = append(merged, page)
	}
	return merged
}

func ValidateTagName(name string) error {
	if _, err := htfiles.CleanRelativePath(name); err != nil {
		return fmt.Errorf("tag name %q is invalid: %w", name, err)
	}
	return nil
}
