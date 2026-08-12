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
	// Internal properties
	URI    *url.URL   `json:"-" yaml:"-"`
	Slug   string     `json:"slug,omitempty" yaml:"slug,omitempty"`
	Cache  *PageCache `json:"-" yaml:"-"`
	Loaded bool       `json:"-" yaml:"-"`

	// Template data properties
	CreatedAt    string             `json:"created_at" yaml:"created_at" jsonschema:"format=date-time,description=Defaults to 'site.created_at'."`
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
	Visibility   PageVisibilityKind `json:"visibility,omitempty" yaml:"visibility,omitempty" jsonschema:"default=public,description=Controls page visibility."`
	UglyURL      bool               `json:"ugly_url,omitempty" yaml:"ugly_url,omitempty"`
	Redirect     string             `json:"redirect,omitempty" yaml:"redirect,omitempty" jsonschema:"format=uri-reference,description=The URI reference written into the page refresh redirect target."`
	Feeds        FeedDataset        `json:"feeds,omitempty" yaml:"feeds,omitempty" jsonschema:"description=Named feeds rendered by this page."`

	// Computed properties
	TemplateData httt.TemplateData `json:"-" yaml:"-"`
	Path         string            `json:"-" yaml:"-"`
	Assets       []string          `json:"-" yaml:"-"` // paths relative to website.ContentDirName()
	Static       bool              `json:"-" yaml:"-"`
}
type Pages []*Page

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
	if _, ok := data.Get("draft").(bool); ok {
		return fmt.Errorf("page %q screened: page.draft is no longer supported; use page.visibility = \"draft\"", page.URI.Path)
	}
	if _, ok := data.Get("unlisted").(bool); ok {
		return fmt.Errorf("page %q screened: page.unlisted is no longer supported; use page.visibility = \"unlisted\"", page.URI.Path)
	}
	if data.Get("feed") != nil {
		return fmt.Errorf("page %q screened: page.feed is no longer supported; use page.feeds", page.URI.Path)
	}
	return nil
}

// Page.Validate()
//
// Validates that the page has all properties required for processing.
func (page *Page) Validate() (err error) {
	if page.URI == nil {
		return fmt.Errorf("page.uri missing")
	}
	if len(page.CreatedAt) == 0 {
		return fmt.Errorf("page.created_at missing (%s)", page.URI.Path)
	}
	if len(page.LayoutRef) == 0 {
		return fmt.Errorf("page.layout missing (%s)", page.URI.Path)
	}
	if strings.Contains(page.Slug, "/") {
		return fmt.Errorf("page.slug cannot contain a slash (%s)", page.Slug)
	}
	if page.Byline != nil && !page.Byline.Ready() {
		return fmt.Errorf("page.byline invalid (%s)", page.URI.Path)
	}
	if err := page.validateTagNames(); err != nil {
		return err
	}
	if err := page.Visibility.Validate(); err != nil {
		return err
	}
	return nil
}

func (page *Page) validateTagNames() error {
	for _, tag := range page.Tags {
		if err := ValidateTagName(tag); err != nil {
			return fmt.Errorf("page.tags: %w", err)
		}
	}
	return nil
}

// Page.Delete()
//
// Deletes the page from disk. If the page has child pages, the page files are
// deleted but the directory is retained.
func (page *Page) Delete(w io.Writer) (err error) {
	if w == nil {
		w = os.Stdout
	}

	fmt.Fprintf(w, "page \"%s\" delete\n", page.URI.Path)
	if page.HasChildPages() {
		fmt.Fprintf(w, "page \"%s\" files will be deleted (descendents will be preserved)\n", filepath.Dir(page.URI.Path))
		if err = page.DeleteFiles(w); err != nil {
			return err
		}
	} else {
		fmt.Fprintf(w, "page \"%s\" directory will be deleted (no descendents to be preserved)\n", filepath.Dir(page.URI.Path))
		if err := os.RemoveAll(filepath.Dir(page.URI.Path)); err != nil {
			return err
		}
	}
	return nil
}
func (page *Page) DeleteFiles(w io.Writer) (err error) {
	if w == nil {
		w = os.Stdout
	}

	var entries []os.DirEntry
	if entries, err = os.ReadDir(filepath.Dir(page.URI.Path)); err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			filePath := filepath.Join(filepath.Dir(page.URI.Path), entry.Name())
			fmt.Fprintf(w, "file \"%s\" delete\n", filePath)
			if err := os.Remove(filePath); err != nil {
				return err
			}
		}
	}
	return nil
}

func (page *Page) CanonicalURL(website *Website) (*url.URL, error) {
	if page == nil || len(page.Path) == 0 || website == nil {
		return nil, fmt.Errorf("page canonical URL requires page for page.path and website for website.URL()")
	}
	path, err := url.Parse(page.Path)
	if err != nil {
		return nil, err
	}
	return website.URL().ResolveReference(path), nil
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
	if page.Feeds != nil {
		page.TemplateData.Set("feeds", page.Feeds.TemplateData())
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
	if len(page.Path) > 0 {
		page.TemplateData.Set("path", page.Path)
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

func (pages Pages) Add(incoming ...*Page) Pages {
	for _, page := range incoming {
		if page != nil && !pages.Contains(page) {
			pages = append(pages, page)
		}
	}
	return pages
}

func (pages Pages) Contains(page *Page) bool {
	return page != nil && pages.PageByURI(page.URI) != nil
}

// PageByURI returns the page whose source URI path matches uri.Path.
func (pages Pages) PageByURI(uri *url.URL) *Page {
	if uri == nil {
		return nil
	}
	index := slices.IndexFunc(pages, func(page *Page) bool {
		return page != nil && page.URI != nil && strings.EqualFold(page.URI.Path, uri.Path)
	})
	if index > -1 {
		return pages[index]
	}
	return nil
}

// PageByPath returns the page whose public website path matches path.
func (pages Pages) PageByPath(path string) *Page {
	index := slices.IndexFunc(pages, func(page *Page) bool {
		return page != nil && strings.EqualFold(page.Path, path)
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
		return page.Path
	default:
		return page.LastModified().Format(time.RFC3339)
	}
}

func (pages Pages) ColumnValues(field string) []string {
	values := []string{}
	for i := range pages {
		switch field {
		case "Path", "path":
			values = append(values, pages[i].Path)
		case "Title", "title":
			values = append(values, pages[i].Title)
		case "LastModified", "lastmod":
			values = append(values, pages[i].LastModified().Format(time.RFC3339))
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

func (page *Page) HasChildPages() bool {
	entries, err := os.ReadDir(filepath.Dir(page.URI.Path))
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

	exclude := "(index|page)\\.(md|yaml|json|html)$"
	exclude += "|(site)\\.(yaml|json)$"
	exclude += "|theme\\.json$"
	pageDir := filepath.Dir(page.URI.Path)
	files, err := htfiles.ListByPattern(pageDir, false, "", exclude)
	if err != nil {
		return err
	}
	pageDirRel := ""
	if pageDirAbs, err := filepath.Abs(pageDir); err != nil {
		return err
	} else if contentDirAbs, err := filepath.Abs(website.ContentDirPath()); err != nil {
		return err
	} else if pageDirAbs != contentDirAbs {
		pageDirRel, err = htfiles.RelativePath(website.ContentDirPath(), pageDir)
		if err != nil {
			return err
		}
		pageDirRel = filepath.ToSlash(pageDirRel)
	}
	assets := []string{}
	for _, file := range files {
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
		if srcPath, err := website.ResourcePath(website.ContentDirPath(), asset); err == nil {
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
		Published:    atom.TimeStamp(page.CreatedAt),
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
	if page == nil || page.URI == nil || page.URI.Path == "" || attachment == nil || attachment.Href == nil {
		return 0
	}
	ref := &attachment.Href.URL
	if ref.IsAbs() || ref.Path == "" || strings.HasPrefix(ref.Path, "/") {
		return 0
	}
	pageDir := filepath.Dir(page.URI.Path)
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
	ts, err := time.Parse(time.RFC3339, page.CreatedAt)
	if err != nil {
		return atom.Text{Type: "html", Body: page.CreatedAt}
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

// ============================================================================
// Helper types, methods, and functions
// ============================================================================
type PageCache struct {
	URI *url.URL
}

func (page *Page) Clone() *Page {
	if page == nil {
		return nil
	}
	clone := *page
	clone.URI = cloneURL(page.URI)
	clone.Cache = page.Cache.Clone()
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

func (cache *PageCache) Clone() *PageCache {
	if cache == nil {
		return nil
	}
	clone := *cache
	clone.URI = cloneURL(cache.URI)
	return &clone
}

// CreatePage(website *Website, page *Page, contenttype *ContentType)
//
// Persists the page content file to disk according to specifications provided
// by contenttype, and updates page.URI.
func CreatePage(website *Website, page *Page, contenttype *ContentType, w io.Writer) (err error) {
	if w == nil {
		w = os.Stdout
	}

	if page == nil {
		return fmt.Errorf("page.create requires page")
	}
	if page.URI != nil {
		return fmt.Errorf("page.create requires page.uri to be empty; use page.update to modify an existing page")
	}

	// timestamp
	timestamp := time.Now().Format(time.RFC3339)
	if len(page.CreatedAt) == 0 {
		page.CreatedAt = timestamp // only set created_at if it is empty
	}
	page.UpdatedAt = timestamp

	// content-type
	contenttype = pageContentType(contenttype)
	if err = contenttype.Spec.Validate(); err != nil {
		return err
	}
	applyContentTypePageDefaults(page, contenttype)
	if !page.Visibility.IsDraft() {
		if err = contenttype.Ready(page); err != nil {
			return err
		}
	}
	if page.URI, err = contenttype.ResolvePageURI(website, page, nil); err != nil {
		return err
	}
	page.ResolvePath(website)
	if err = assertPageCreateDestinationAvailable(page); err != nil {
		return err
	}

	// write to disk
	return WritePage(website, page, contenttype.Spec.Format, w)
}

// UpdatePage updates an existing persisted page. The input page.URI identifies
// the current source file. Empty content types write in place using the source
// file format. Real content types recompute the destination path from the
// submitted page and content type, allowing content type, slug, and format
// changes to rehome the page payload without touching nested page directories.
func UpdatePage(website *Website, page *Page, contenttype *ContentType, w io.Writer) (err error) {
	if w == nil {
		w = os.Stdout
	}

	if page == nil {
		return fmt.Errorf("page.update requires page")
	}
	if page.URI == nil || page.URI.Path == "" {
		return fmt.Errorf("page.update requires page.uri")
	}

	sourceURI := *page.URI
	sourcePath := sourceURI.Path
	sourceInfo, err := os.Stat(sourcePath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("page.update source does not exist: %s", sourcePath)
		}
		return err
	}
	if sourceInfo.IsDir() || !sourceInfo.Mode().IsRegular() {
		return fmt.Errorf("page.update source must be a regular file: %s", sourcePath)
	}
	// timestamp
	timestamp := time.Now().Format(time.RFC3339)
	if len(page.CreatedAt) == 0 {
		existing := &Page{URI: &sourceURI}
		if err = LoadPage(website, existing, false, w); err == nil && existing.CreatedAt != "" {
			page.CreatedAt = existing.CreatedAt
		}
	}
	if len(page.CreatedAt) == 0 {
		page.CreatedAt = timestamp
	}
	page.UpdatedAt = timestamp

	if contenttype.IsEmpty() {
		format, err := pageUpdateSourceFormat(sourcePath)
		if err != nil {
			return err
		}
		page.URI = &sourceURI
		page.ResolvePath(website)
		return WritePage(website, page, format, w)
	}

	// content-type
	contenttype = pageContentType(contenttype)
	if err = contenttype.Spec.Validate(); err != nil {
		return err
	}
	applyContentTypePageDefaults(page, contenttype)
	if !page.Visibility.IsDraft() {
		if err = contenttype.Ready(page); err != nil {
			return err
		}
	}
	if page.URI, err = contenttype.ResolvePageURI(website, page, &sourceURI); err != nil {
		return err
	}
	page.ResolvePath(website)
	if page.URI == nil || page.URI.Path == "" {
		return fmt.Errorf("page.update failed to resolve destination page.uri")
	}
	destinationPath := page.URI.Path

	if err = assertPageUpdateDestinationAvailable(sourcePath, destinationPath); err != nil {
		return err
	}
	moves, err := pageRegularFileMoves(sourcePath, destinationPath)
	if err != nil {
		return err
	}
	if len(moves) > 0 {
		if err = os.MkdirAll(filepath.Dir(destinationPath), 0755); err != nil {
			return err
		}
		for _, move := range moves {
			if err = os.Rename(move.sourcePath, move.destinationPath); err != nil {
				return err
			}
		}
	}

	if err = WritePage(website, page, contenttype.Spec.Format, w); err != nil {
		return err
	}
	if !sameCleanPath(sourcePath, destinationPath) {
		if err = os.Remove(sourcePath); err != nil && !os.IsNotExist(err) {
			return err
		}
		if err = removeDirectoryIfEmpty(filepath.Dir(sourcePath)); err != nil {
			return err
		}
	}
	return nil
}

func FinalizePage(website *Website, page *Page, contenttype *ContentType, w io.Writer) (err error) {
	if w == nil {
		w = os.Stdout
	}
	if page == nil {
		return fmt.Errorf("page.finalize requires page")
	}
	if page.URI == nil || page.URI.Path == "" {
		return fmt.Errorf("page.finalize requires page.uri")
	}

	sourceURI := *page.URI
	sourcePath := sourceURI.Path
	sourceInfo, err := os.Stat(sourcePath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("page.finalize source does not exist: %s", sourcePath)
		}
		return err
	}
	if sourceInfo.IsDir() || !sourceInfo.Mode().IsRegular() {
		return fmt.Errorf("page.finalize source must be a regular file: %s", sourcePath)
	}

	timestamp := time.Now().Format(time.RFC3339)
	freeform := contenttype.IsEmpty()
	if len(page.CreatedAt) == 0 || (freeform && len(page.LayoutRef) == 0) {
		existing := &Page{URI: &sourceURI}
		if err = LoadPage(website, existing, false, w); err == nil {
			if len(page.CreatedAt) == 0 {
				page.CreatedAt = existing.CreatedAt
			}
			if freeform && len(page.LayoutRef) == 0 {
				page.LayoutRef = existing.LayoutRef
			}
		}
	}
	if len(page.CreatedAt) == 0 {
		page.CreatedAt = timestamp
	}
	page.UpdatedAt = timestamp
	finalizePageDraftState(page)

	if freeform {
		format, err := pageUpdateSourceFormat(sourcePath)
		if err != nil {
			return err
		}
		page.URI = &sourceURI
		page.ResolvePath(website)
		if err = page.Validate(); err != nil {
			return err
		}
		return WritePage(website, page, format, w)
	}

	contenttype = pageContentType(contenttype)
	if err = contenttype.Spec.Validate(); err != nil {
		return err
	}
	finalPage, err := contenttype.OverlayPage(page)
	if err != nil {
		return err
	}
	if contenttype.Spec.Visibility != "" {
		finalPage.Visibility = contenttype.Spec.Visibility
	}
	finalizePageDraftState(finalPage)
	if err = contenttype.Ready(finalPage); err != nil {
		return err
	}
	if finalPage.URI, err = contenttype.ResolvePageURI(website, finalPage, &sourceURI); err != nil {
		return err
	}
	finalPage.ResolvePath(website)
	if finalPage.URI == nil || finalPage.URI.Path == "" {
		return fmt.Errorf("page.finalize failed to resolve destination page.uri")
	}
	if err = finalPage.Validate(); err != nil {
		return err
	}
	destinationPath := finalPage.URI.Path

	if err = assertPageUpdateDestinationAvailable(sourcePath, destinationPath); err != nil {
		return err
	}
	moves, err := pageRegularFileMoves(sourcePath, destinationPath)
	if err != nil {
		return err
	}
	if len(moves) > 0 {
		if err = os.MkdirAll(filepath.Dir(destinationPath), 0755); err != nil {
			return err
		}
		for _, move := range moves {
			if err = os.Rename(move.sourcePath, move.destinationPath); err != nil {
				return err
			}
		}
	}

	*page = *finalPage
	if err = WritePage(website, page, contenttype.Spec.Format, w); err != nil {
		return err
	}
	if !sameCleanPath(sourcePath, destinationPath) {
		if err = os.Remove(sourcePath); err != nil && !os.IsNotExist(err) {
			return err
		}
		if err = removeDirectoryIfEmpty(filepath.Dir(sourcePath)); err != nil {
			return err
		}
	}
	return nil
}

type pageFileMove struct {
	sourcePath      string
	destinationPath string
}

func applyContentTypePageDefaults(page *Page, contenttype *ContentType) {
	if page == nil || contenttype == nil {
		return
	}
	if page.LayoutRef == "" {
		page.LayoutRef = contenttype.Spec.Layout
	}
	if contenttype.Spec.Visibility != "" {
		page.Visibility = contenttype.Spec.Visibility
	}
}

func finalizePageDraftState(page *Page) {
	if page == nil {
		return
	}
	if page.Visibility.IsDraft() {
		page.Visibility = PublicVisibility
	}
	if page.TemplateData != nil {
		page.TemplateData.Delete("visibility")
	}
}

func pageContentType(contenttype *ContentType) *ContentType {
	if contenttype.IsEmpty() {
		return &DefaultContentType
	}
	return contenttype
}

func pageUpdateSourceFormat(sourcePath string) (string, error) {
	switch strings.ToLower(filepath.Ext(sourcePath)) {
	case ".md", ".markdown":
		return "markdown", nil
	case ".json":
		return "json", nil
	case ".yaml", ".yml":
		return "yaml", nil
	default:
		return "", fmt.Errorf("page.update auto content type does not support source file extension %q: %s", filepath.Ext(sourcePath), sourcePath)
	}
}

func assertPageCreateDestinationAvailable(page *Page) error {
	if page.URI == nil || page.URI.Path == "" {
		return fmt.Errorf("page.create failed to resolve destination page.uri")
	}
	if _, exists, err := fileInfoIfExists(page.URI.Path); err != nil {
		return err
	} else if exists {
		return fmt.Errorf("page.create conflict: page already exists at %s", page.URI.Path)
	}
	return nil
}

func assertPageUpdateDestinationAvailable(sourcePath string, destinationPath string) error {
	if _, exists, err := fileInfoIfExists(destinationPath); err != nil {
		return err
	} else if exists && !sameExistingFile(sourcePath, destinationPath) {
		return fmt.Errorf("page.update conflict: page already exists at %s", destinationPath)
	}
	return nil
}

func pageRegularFileMoves(sourcePagePath string, destinationPagePath string) ([]pageFileMove, error) {
	sourceDir := filepath.Dir(sourcePagePath)
	destinationDir := filepath.Dir(destinationPagePath)
	if sameCleanPath(sourceDir, destinationDir) {
		return nil, nil
	}

	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return nil, err
	}
	moves := []pageFileMove{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			continue
		}
		sourcePath := filepath.Join(sourceDir, entry.Name())
		if sameExistingFile(sourcePath, sourcePagePath) {
			continue
		}

		destinationPath := filepath.Join(destinationDir, entry.Name())
		if sameCleanPath(destinationPath, destinationPagePath) {
			return nil, fmt.Errorf("page.update conflict: regular file %s would overwrite destination page file %s", sourcePath, destinationPagePath)
		}
		if _, exists, err := fileInfoIfExists(destinationPath); err != nil {
			return nil, err
		} else if exists {
			return nil, fmt.Errorf("page.update conflict: regular file already exists at %s", destinationPath)
		}
		moves = append(moves, pageFileMove{sourcePath: sourcePath, destinationPath: destinationPath})
	}
	return moves, nil
}

func fileInfoIfExists(path string) (os.FileInfo, bool, error) {
	info, err := os.Stat(path)
	if err == nil {
		return info, true, nil
	}
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	return nil, false, err
}

func sameExistingFile(a string, b string) bool {
	if sameCleanPath(a, b) {
		return true
	}
	aInfo, aErr := os.Stat(a)
	bInfo, bErr := os.Stat(b)
	return aErr == nil && bErr == nil && os.SameFile(aInfo, bInfo)
}

func sameCleanPath(a string, b string) bool {
	aAbs, err := filepath.Abs(a)
	if err == nil {
		a = aAbs
	}
	bAbs, err := filepath.Abs(b)
	if err == nil {
		b = bAbs
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

func removeDirectoryIfEmpty(path string) error {
	entries, err := os.ReadDir(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(entries) > 0 {
		return nil
	}
	return os.Remove(path)
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
	switch format {
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

// persist page data file to disk
func WritePage(website *Website, page *Page, format string, w io.Writer) (err error) {
	if w == nil {
		w = os.Stdout
	}

	var file *os.File
	var fileBytes []byte
	var fileBytesWritten int
	if fileBytes, err = page.RenderData(website, format); err != nil {
		return err
	}
	fmt.Fprintf(w, "writing %d bytes to %s\n", len(fileBytes), website.RelativePath(page.URI.Path))
	if err = os.MkdirAll(filepath.Dir(page.URI.Path), 0755); err != nil {
		return err
	}
	if file, err = os.OpenFile(page.URI.Path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644); err != nil {
		return err
	}
	defer file.Close()
	if fileBytesWritten, err = file.Write(fileBytes); err != nil {
		return err
	}
	if fileBytesWritten != len(fileBytes) {
		return fmt.Errorf("expected to write %v bytes, but wrote %v", len(fileBytes), fileBytesWritten)
	}
	return nil
}

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
	taskgroup, _ := errgroup.WithContext(ctx)
	for i := range pages {
		taskgroup.Go(func() error {
			if cache.Contains(pages[i]) {
				pages[i] = cache.PageByURI(pages[i].URI)
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
	if w == nil {
		w = os.Stdout
	}

	var fileExt string = filepath.Ext(page.URI.Path)
	var fileBytes []byte
	var fileData httt.TemplateData
	if page.Loaded {
		return // guard
	}
	if verbose {
		fmt.Fprintf(w, "page \"%s\" loading\n", website.RelativePath(page.URI.Path))
	}
	if fileBytes, err = os.ReadFile(page.URI.Path); err != nil {
		return err
	}
	if fileData, err = httt.ParseTemplateDataBytes(fileBytes, fileExt, website.Config.Markdown); err != nil {
		return err
	}
	page.CreatedAt = website.CreatedAt // default created_at to website created_at
	page.TemplateData = fileData
	if err = page.Parse(fileData); err != nil {
		return err
	}
	if err = page.Visibility.Validate(); err != nil {
		return err
	}
	if err = page.validateTagNames(); err != nil {
		return err
	}
	page.ResolvePath(website)
	if page.LayoutRef == "" {
		page.LayoutRef = "default" // TODO: should layout be required?
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
		var uri *url.URL
		absPath, err := htfiles.LocalPath(directory, p)
		if err != nil {
			return nil, err
		}
		if uri, err = url.Parse(absPath); err != nil {
			return nil, err
		}
		page := &Page{URI: uri, Path: PagePath(website, uri), TemplateData: nil}
		if path.Ext(page.URI.Path) == ".html" {
			page.Static = true // mark page as static if source file is HTML (i.e. skip template processing and write directly to output)
		}
		pages = append(pages, page)
	}
	return pages, nil
}

func ValidateTagName(name string) error {
	if _, err := htfiles.CleanRelativePath(name); err != nil {
		return fmt.Errorf("tag name %q is invalid: %w", name, err)
	}
	return nil
}

func PagePath(website *Website, uri *url.URL) string {
	subPath, err := htfiles.RelativePath(website.ContentDirPath(), uri.Path)
	if err != nil {
		return ""
	}
	subPath = filepath.Dir(subPath)
	if subPath == "." {
		return "/"
	}
	return "/" + filepath.ToSlash(subPath) + "/"
}

func (page *Page) ResolvePath(website *Website) {
	if website == nil || page.URI == nil {
		return
	}
	page.Path = PagePath(website, page.URI)
	if !page.UglyURL || page.Path == "/" {
		return
	}
	page.Path = strings.TrimSuffix(page.Path, "/") + ".html"
}
