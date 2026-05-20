package hypertexting

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
	"gopkg.in/yaml.v3"

	atom "github.com/herdworks/hyperfeed-go/types/atom"
	hypertemplates "github.com/herdworks/hypertemplates-go"
)

// Page
//
// Page objects do the heavy lifting to parse content data files
// and convert them into rendered pages.
type Page struct {
	// Internal properties
	URI    *url.URL   `json:"-" yaml:"-"`
	Slug   string     `json:"-" yaml:"-"`
	Cache  *PageCache `json:"-" yaml:"-"`
	Loaded bool       `json:"-" yaml:"-"`

	// Template data properties
	CreatedAt    string      `json:"created_at" yaml:"created_at"`
	UpdatedAt    string      `json:"updated_at" yaml:"updated_at"`
	LayoutRef    string      `json:"layout" yaml:"layout"`
	Title        string      `json:"title,omitempty" yaml:"title,omitempty"`
	Description  string      `json:"description,omitempty" yaml:"description,omitempty"`
	Content      string      `json:"content,omitempty" yaml:"content,omitempty"`
	Summary      string      `json:"summary,omitempty" yaml:"summary,omitempty"`
	More         string      `json:"more,omitempty" yaml:"more,omitempty"`
	Byline       *Byline     `json:"byline,omitempty" yaml:"byline,omitempty"`
	Contributors Bylines     `json:"contributors,omitempty" yaml:"contributors,omitempty"`
	Attachments  Attachments `json:"attachments,omitempty" yaml:"attachments,omitempty"`
	Metadata     []HTMLMeta  `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Links        []HTMLLink  `json:"links,omitempty" yaml:"links,omitempty"`
	Tags         []string    `json:"tags,omitempty" yaml:"tags,omitempty"`
	Draft        bool        `json:"draft,omitempty" yaml:"draft,omitempty"`
	Unlisted     bool        `json:"unlisted,omitempty" yaml:"unlisted,omitempty"`
	UglyURL      bool        `json:"ugly_url,omitempty" yaml:"ugly_url,omitempty"`
	Redirect     string      `json:"redirect,omitempty" yaml:"redirect,omitempty"`
	Feed         *HyperFeed  `json:"feed,omitempty" yaml:"feed,omitempty"`

	// Computed properties
	TemplateData hypertemplates.TemplateData `json:"-" yaml:"-"`
	Path         string                      `json:"-" yaml:"-"`
	Assets       []string                    `json:"-" yaml:"-"` // paths relative to website.ContentDirName()
	Static       bool                        `json:"-" yaml:"-"`
}
type Pages []*Page

// Page.Parse(data hypertemplates.TemplateData)
//
// Hydrates page properties from template data.
// Parse first marshals the data to YAML bytes, then unmarshals the YAML to the page.
func (page *Page) Parse(data hypertemplates.TemplateData) (err error) {
	var yamlBytes []byte
	if yamlBytes, err = yaml.Marshal(page.TemplateData); err != nil {
		return err
	}
	return yaml.Unmarshal(yamlBytes, &page)
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
	return nil
}

// Page.Delete()
//
// Deletes the page from disk. If the page has child pages, the page files are
// deleted but the directory is retained.
func (page *Page) Delete() (err error) {
	fmt.Printf("page \"%s\" delete\n", page.URI.Path)
	if page.HasChildPages() {
		fmt.Printf("page \"%s\" files will be deleted (descendents will be preserved)\n", filepath.Dir(page.URI.Path))
		if err = page.DeleteFiles(); err != nil {
			return err
		}
	} else {
		fmt.Printf("page \"%s\" directory will be deleted (no descendents to be preserved)\n", filepath.Dir(page.URI.Path))
		if err := os.RemoveAll(filepath.Dir(page.URI.Path)); err != nil {
			return err
		}
	}
	return nil
}
func (page *Page) DeleteFiles() (err error) {
	var entries []os.DirEntry
	if entries, err = os.ReadDir(filepath.Dir(page.URI.Path)); err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			filePath := filepath.Join(filepath.Dir(page.URI.Path), entry.Name())
			fmt.Printf("file \"%s\" delete\n", filePath)
			if err := os.Remove(filePath); err != nil {
				return err
			}
		}
	}
	return nil
}

// Page.ApplyComputedProperties()
//
// Applies computed properties to the page template data.
func (page *Page) ApplyComputedProperties(website *Website) {
	if len(page.Path) > 0 {
		page.TemplateData.Set("path", page.Path)
	}
	if len(page.Path) > 0 {
		path, _ := url.Parse(page.Path)
		page.TemplateData.Set("canonical_url", website.URL().ResolveReference(path).String())
	}
	if len(page.Redirect) > 0 {
		page.TemplateData.Set("refresh", fmt.Sprintf("%d; url=%s", website.Config.RefreshInterval, page.Redirect))
	}
	if len(page.Assets) > 0 {
		page.TemplateData.Set("assets", page.AssetNames())
	}
	if page.Feed != nil {
		page.TemplateData.Set("feed", page.Feed.TemplateData())
	}
	if page.Feed != nil {
		page.TemplateData.Set("assets", append(page.AssetNames(), "atom.xml"))
	}
}

// Pages.ResolveTemplateData()
//
// Resolves non-empty page struct fields into page.TemplateData.
// TODO: merge ResolveTemplateData() with ApplyComputedProperties()
func (pages Pages) ResolveTemplateData(website *Website) {
	for i := range pages {
		pages[i].ResolveTemplateData(website)
	}
}
func (page *Page) ResolveTemplateData(website *Website) {
	if page.TemplateData == nil {
		page.TemplateData = hypertemplates.TemplateData{}
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
	if page.Draft {
		page.TemplateData.Set("draft", page.Draft)
	}
	if page.Unlisted {
		page.TemplateData.Set("unlisted", page.Unlisted)
	}
	if page.UglyURL {
		page.TemplateData.Set("ugly_url", page.UglyURL)
	}
	if len(page.Redirect) > 0 {
		page.TemplateData.Set("redirect", page.Redirect)
	}
	if len(page.Assets) > 0 {
		page.TemplateData.Set("assets", page.AssetNames())
	}
	if len(page.Attachments) > 0 {
		page.TemplateData.Set("attachments", page.Attachments.TemplateData())
	}
	if len(page.Summary) > 0 {
		page.TemplateData.Set("summary", page.Summary)
	}
	if len(page.More) > 0 {
		page.TemplateData.Set("more", page.More)
	}
	page.ApplyComputedProperties(website)
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

func (pages Pages) Contains(page *Page) bool {
	return pages.PageByURI(page.URI) != nil
}

// PageByURI returns the page whose source URI path matches uri.Path.
func (pages Pages) PageByURI(uri *url.URL) *Page {
	index := slices.IndexFunc(pages, func(page *Page) bool {
		return strings.EqualFold(page.URI.Path, uri.Path)
	})
	if index > -1 {
		return pages[index]
	}
	return nil
}

// PageByPath returns the page whose public website path matches path.
func (pages Pages) PageByPath(path string) *Page {
	index := slices.IndexFunc(pages, func(page *Page) bool {
		return strings.EqualFold(page.Path, path)
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

func (pages Pages) ContainsPath(href string) bool {
	return pages.PageByPath(href) != nil
}

func (pages Pages) FilterDraft(draft bool) Pages {
	filtered := Pages{}
	for _, page := range pages {
		if page != nil && page.Draft == draft {
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

func (pages Pages) TemplateData() (data []hypertemplates.TemplateData) {
	for i := range pages {
		data = append(data, pages[i].TemplateData)
	}
	return data
}

// TemplateDataWithout returns each page's TemplateData with the named
// top-level keys omitted. Used by HyperFeed.TemplateData to break the
// page→feed→pages→page reference cycle that arises when two feed-bearing
// pages both match each other.
func (pages Pages) TemplateDataWithout(keys ...string) (data []hypertemplates.TemplateData) {
	for i := range pages {
		data = append(data, pages[i].TemplateData.Without(keys...))
	}
	return data
}

func (pages Pages) DiscoverAssets(website *Website, verbose bool) error {
	for i := range pages {
		if err := pages[i].DiscoverAssets(website, verbose); err != nil {
			return err
		}
	}
	return nil
}
func (page *Page) DiscoverAssets(website *Website, verbose bool) error {
	exclude := "(index|page)\\.(md|yaml|json|html)$"
	exclude += "|(site)\\.(yaml|json)$"
	exclude += "|theme\\.json$"
	pageDir := filepath.Dir(page.URI.Path)
	files, err := hypertemplates.ListFilesByPattern(pageDir, false, "", exclude)
	if err != nil {
		return err
	}
	pageDirRel := ""
	if pageDirAbs, err := filepath.Abs(pageDir); err != nil {
		return err
	} else if contentDirAbs, err := filepath.Abs(website.ContentDirPath()); err != nil {
		return err
	} else if pageDirAbs != contentDirAbs {
		pageDirRel, err = hypertemplates.RelativeFilePath(website.ContentDirPath(), pageDir)
		if err != nil {
			return err
		}
		pageDirRel = filepath.ToSlash(pageDirRel)
	}
	assets := []string{}
	for _, file := range files {
		relPath := filepath.ToSlash(filepath.Join(pageDirRel, file))
		if _, err = hypertemplates.CleanRelativeFilePath(relPath); err != nil {
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
			fmt.Printf("asset \"%s\" discovered\n", website.RelativePath(srcPath))
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
	var atomAuthor *atom.Author
	var atomContributors []*atom.Author
	var err error
	if atomLinks, err = page.AtomEntryLinks(website); err != nil {
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
		ID:           atomLinks[len(atomLinks)-1].Href,
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
	var atomRelatedURL *url.URL
	if atomURL, err = website.ResolveHref(page.AtomEntryLink()); err != nil {
		return nil, err
	}
	if atomRelatedURL, err = website.ResolveHref(page.Path); err != nil {
		return nil, err
	}
	return []atom.Link{
		{Rel: "alternate", Type: "text/html", Href: atomURL.String()},
		{Rel: "via", Type: "text/html", Href: atomURL.String()},
		{Rel: "related", Type: "text/html", Href: atomRelatedURL.String()},
	}, nil
}

func (page *Page) AtomEntryLink() string {
	redirected := len(page.Redirect) > 0
	nocontent := len(page.Content) == 0
	if redirected && nocontent {
		return page.Redirect
	}
	return page.Path
}

func (page *Page) AtomEntryAuthor(website *Website) (person *atom.Author, err error) {
	if page.Byline == nil {
		page.Byline = &website.Byline
	}
	return HyperFeed{}.AtomPerson(website, page.Byline)
}

func (page *Page) AtomEntryContributors(website *Website) (contributors []*atom.Author, err error) {
	return HyperFeed{}.AtomPersons(website, page.Contributors)
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
	if len(page.Attachments.FilteredBy(AttachmentKindLink)) > 0 {
		attachment := page.Attachments.FilteredBy(AttachmentKindLink)[0]
		attachment.Data.Get("hypergraph.description")
		return atom.Text{Type: "html", Body: fmt.Sprintf("Link to %s", attachment.Href)}
	}
	if len(page.More) > 0 {
		return atom.Text{Type: "html", Body: page.More}
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
		Page  *Page                       `yaml:",inline"`
		Other hypertemplates.TemplateData `yaml:",inline"`
	}{
		Page:  page,
		Other: page.TemplateData.Without("created_at", "updated_at", "layout", "author", "contributors", "title", "description", "summary", "preview", "tags", "unlisted", "ugly_url", "redirect", "attachments"),
	}, nil
}

// ============================================================================
// Helper types, methods, and functions
// ============================================================================
type PageCache struct {
	URI *url.URL
}

// CreatePage(website *Website, page *Page, contenttype *ContentType)
//
// Persists the page content file to disk according to specifications provided
// by contenttype, and updates page.URI.
func CreatePage(website *Website, page *Page, contenttype *ContentType) (err error) {
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
	if !page.Draft {
		if err = contenttype.Validate(page); err != nil {
			return err
		}
	}
	if err = contenttype.Apply(website, page); err != nil {
		return err
	}
	if err = assertPageCreateDestinationAvailable(page); err != nil {
		return err
	}

	// write to disk
	return WritePage(website, page, contenttype.Spec.Format)
}

// UpdatePage updates an existing persisted page. The input page.URI identifies
// the current source file. The destination path is always recomputed from the
// submitted page and content type, allowing content type, slug, and format
// changes to rehome the page payload without touching nested page directories.
func UpdatePage(website *Website, page *Page, contenttype *ContentType) (err error) {
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
		if err = LoadPage(website, existing, false); err == nil && existing.CreatedAt != "" {
			page.CreatedAt = existing.CreatedAt
		}
	}
	if len(page.CreatedAt) == 0 {
		page.CreatedAt = timestamp
	}
	page.UpdatedAt = timestamp

	// content-type
	contenttype = pageContentType(contenttype)
	if !page.Draft {
		if err = contenttype.Validate(page); err != nil {
			return err
		}
	}
	page.URI = nil
	page.Path = ""
	if err = contenttype.Apply(website, page); err != nil {
		return err
	}
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

	if err = WritePage(website, page, contenttype.Spec.Format); err != nil {
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

func pageContentType(contenttype *ContentType) *ContentType {
	if contenttype.IsEmpty() {
		return &DefaultContentType
	}
	return contenttype
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

// persist page index file to disk
func WritePage(website *Website, page *Page, format string) (err error) {
	var file *os.File
	var fileBytes []byte
	var fileBytesWritten int
	var frontmatter []byte // YAML frontmatter (for markdown format only)
	page.ResolveTemplateData(website)
	switch format {
	case "json":
		page.TemplateData.Set("content", page.Content) // add content to template data
		if fileBytes, err = json.Marshal(page.TemplateData); err != nil {
			return err
		}
	case "yaml", "yml":
		page.TemplateData.Set("content", page.Content) // add content to template data
		if fileBytes, err = yaml.Marshal(page.TemplateData); err != nil {
			return err
		}
	case "markdown", "md":
		fallthrough
	default:
		if frontmatter, err = yaml.Marshal(OrderedTemplateData(page.TemplateData)); err != nil {
			return err
		}
		fileBytes = fmt.Appendf([]byte{}, "---\n%s---\n\n%s", frontmatter, page.Content)
	}
	fmt.Printf("writing %d bytes to %s\n", len(fileBytes), website.RelativePath(page.URI.Path))
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
func LoadPages(ctx context.Context, website *Website, pages Pages, cache Pages, verbose bool) (err error) {
	if cache == nil {
		cache = Pages{}
	}
	taskgroup, _ := errgroup.WithContext(ctx)
	for i := range pages {
		taskgroup.Go(func() error {
			if cache.Contains(pages[i]) {
				pages[i] = cache.PageByURI(pages[i].URI)
			}
			return LoadPage(website, pages[i], verbose)
		})
	}
	return taskgroup.Wait()
}

// LoadPage(website *Website, page *Page, verbose bool)
//
// Loads the page from disk and parses the content into the page object.
func LoadPage(website *Website, page *Page, verbose bool) (err error) {
	var fileExt string = filepath.Ext(page.URI.Path)
	var fileBytes []byte
	var fileData hypertemplates.TemplateData
	if page.Loaded {
		return // guard
	}
	if verbose {
		fmt.Printf("page \"%s\" loading\n", website.RelativePath(page.URI.Path))
	}
	if fileBytes, err = os.ReadFile(page.URI.Path); err != nil {
		return err
	}
	if fileData, err = hypertemplates.ParseTemplateDataBytes(fileBytes, fileExt, website.Config.Markdown); err != nil {
		return err
	}
	page.CreatedAt = website.CreatedAt // default created_at to website created_at
	page.TemplateData = fileData
	page.Parse(fileData)
	page.ResolvePath(website)
	if page.LayoutRef == "" {
		page.LayoutRef = "default" // TODO: should layout be required?
	}
	if err = page.DiscoverAssets(website, verbose); err != nil {
		return err
	}
	page.Loaded = true
	return nil
}

// AddFeedPages(context context.Context, build *HyperBuild)
//
// Adds pages to a build if a build page matches a non-build page feed.
func AddFeedPages(context context.Context, build *HyperBuild) (err error) {
	taskgroup, _ := errgroup.WithContext(context)
	for i := range build.Website.Pages {
		taskgroup.Go(func() error {
			if build.Website.Pages[i].Feed == nil {
				return nil // skip page if no page feed is configured
			}
			if !build.Website.Pages[i].Feed.MatchesAny(*build.Pages) {
				return nil // check if a build page is in a site page feed
			}
			*build.Pages = append(*build.Pages, build.Website.Pages[i]) // add page to build pages
			return nil
		})
	}
	return taskgroup.Wait()
}

// LoadPageFeeds(website *Website, pages Pages)
func LoadPageFeeds(context context.Context, website *Website, pages Pages) (err error) {
	taskgroup, _ := errgroup.WithContext(context)
	website.Pages.ResolveTemplateData(website) // resolve template data for source pages
	for i := range pages {
		taskgroup.Go(func() error {
			if pages[i].Feed == nil {
				return nil // skip page if no page feed is configured
			}
			if len(pages[i].Feed.Title) == 0 {
				pages[i].Feed.Title = website.Title
			}
			if len(pages[i].Feed.Description) == 0 {
				pages[i].Feed.Description = website.Description
			}
			if pages[i].Feed.Byline == nil {
				pages[i].Feed.Byline = &website.Byline
			}
			if err = pages[i].Feed.AddPages(website.Pages...); err != nil {
				return err
			}
			pages[i].Feed.Pages.Sort() // sort the feed pages in reverse-chron order, as nature intended 🤌🏽
			return nil
		})
	}
	return taskgroup.Wait()
}

// LoadTagPages(website *Website, pages Pages) (err error)
func LoadTagPages(build *HyperBuild) (err error) {
	for i := range *build.Pages {
		build.Tags.Increment(1, (*build.Pages)[i].Tags...) // count tags
	}
	for tag := range build.Tags.Counters {
		fmt.Printf("tag \"%s\" loading\n", tag)
		tagPath := filepath.Join(build.Website.Config.TagPath, tag)
		tagPage := build.Pages.PageByPath(tagPath)
		if tagPage == nil {
			tagPage = &Page{
				URI:         &url.URL{Path: filepath.Join(tagPath, "index.md")},
				CreatedAt:   build.Website.Pages[0].CreatedAt,
				LayoutRef:   build.Website.Config.TagLayout,
				Path:        tagPath,
				Title:       fmt.Sprintf("#%s", tag),
				Description: fmt.Sprintf("Everything tagged with #%s", tag),
			}
		}
		if tagPage.Feed == nil {
			tagPage.Feed = &HyperFeed{
				Title:       fmt.Sprintf("#%s", tag),
				Description: fmt.Sprintf("Everything tagged with #%s", tag),
				Byline:      &build.Website.Byline,
				Tags:        []string{tag},
				Pages:       Pages{},
			}
		}
		tagPage.Feed.AddPages(build.Website.Pages...) // populate tag page feed with pages
		tagPage.Feed.Pages.Sort()                     // sort the feed pages in reverse-chron order, as nature intended 🤌🏽
		tagPage.ResolveTemplateData(build.Website)    // resolve template data for tag page so it's ready for
		*build.Pages = append(*build.Pages, tagPage)  // attach feed to the page
	}
	return nil
}

func GetPages(website *Website, directory string) (pages Pages, err error) {
	pages = Pages{} // "discovered pages"
	paths, err := hypertemplates.ListFilesByPattern(directory, true, "(index|page)\\.(md|yaml|json|html)$", fmt.Sprintf("^%s/*", website.BuildsDirName()))
	if err != nil {
		return nil, err
	}
	for _, p := range paths {
		var uri *url.URL
		absPath, err := hypertemplates.LocalFilePath(directory, p)
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

func PagePath(website *Website, uri *url.URL) string {
	subPath, err := hypertemplates.RelativeFilePath(website.ContentDirPath(), uri.Path)
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
