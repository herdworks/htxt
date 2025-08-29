package hypertexting

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/herdworks/hypertexting-monorepo/hypergraph"
	"github.com/herdworks/hypertexting-monorepo/hypertemplates"
	"github.com/herdworks/hypertexting-monorepo/hypertemplates/atom"
	"gopkg.in/yaml.v3"

	"golang.org/x/sync/errgroup"
)

// Page
//
// Page objects do the heavy lifting to parse content data files
// and convert them into rendered pages.
type Page struct {
	// Internal properties
	URI *url.URL `json:"-" yaml:"-"`
	Slug string `json:"-" yaml:"-"`
	Cache  *PageCache `json:"-" yaml:"-"`
	Loaded bool `json:"-" yaml:"-"`

	// Template data properties
	CreatedAt string `json:"created_at" yaml:"created_at"`
	UpdatedAt string `json:"updated_at" yaml:"updated_at"`
	LayoutRef string `json:"layout" yaml:"layout"`
	Title string `json:"title,omitempty" yaml:"title,omitempty"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	Content string `json:"content,omitempty" yaml:"content,omitempty"`
	Summary string `json:"summary,omitempty" yaml:"summary,omitempty"`
	More string `json:"more,omitempty" yaml:"more,omitempty"`
	Author *hypergraph.Entity `json:"author,omitempty" yaml:"author,omitempty"`
	Contributors hypergraph.Entities `json:"contributors,omitempty" yaml:"contributors,omitempty"`
	Attachments Attachments `json:"attachments,omitempty" yaml:"attachments,omitempty"`
	Metadata []HTMLMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Links []HTMLLink `json:"links,omitempty" yaml:"links,omitempty"`
	Tags []string `json:"tags,omitempty" yaml:"tags,omitempty"`
	Draft bool `json:"draft,omitempty" yaml:"draft,omitempty"`
	Unlisted bool `json:"unlisted,omitempty" yaml:"unlisted,omitempty"`
	Redirect string `json:"redirect,omitempty" yaml:"redirect,omitempty"`
	Feed *HyperFeed `json:"feed,omitempty" yaml:"feed,omitempty"`

	// Computed properties
	TemplateData hypertemplates.TemplateData `json:"-" yaml:"-"`
	Path string `json:"-" yaml:"-"`
	Assets hypertemplates.HyperFiles `json:"-" yaml:"-"`
}
type Pages []*Page

// Page.Parse(data hypertemplates.TemplateData)
//
// Hydrates page properties from template data.
// Parse first marshals the data to YAML bytes, then unmarshals the YAML to the page.
func (page *Page) Parse(data hypertemplates.TemplateData) (err error) {
	var yamlBytes []byte
	if yamlBytes,err = yaml.Marshal(page.TemplateData); err != nil { return err }
	return yaml.Unmarshal(yamlBytes, &page)
}

// Page.Validate()
//
// Validates that the page has all properties required for processing.
func (page *Page) Validate() (err error) {
	if page.URI == nil { return fmt.Errorf("page.uri missing") }
	if len(page.CreatedAt) == 0 { return fmt.Errorf("page.created_at missing (%s)", page.URI.Path) }
	if len(page.LayoutRef) == 0 { return fmt.Errorf("page.layout missing (%s)", page.URI.Path) }
	if strings.Contains(page.Slug, "/") { return fmt.Errorf("page.slug cannot contain a slash (%s)", page.Slug) }
	if page.Author == nil { return nil } // early return
	if err = page.Author.Validate("page.author"); err != nil { 
		return fmt.Errorf("%w (%s)", err, page.URI.Path)
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
		if err = page.DeleteFiles(); err != nil { return err }
	} else {
		fmt.Printf("page \"%s\" directory will be deleted (no descendents to be preserved)\n", filepath.Dir(page.URI.Path))
		if err := os.RemoveAll(filepath.Dir(page.URI.Path)); err != nil { return err }
	}
	return nil
}
func (page *Page) DeleteFiles() (err error) {
	var entries []os.DirEntry
	if entries, err = os.ReadDir(filepath.Dir(page.URI.Path)); err != nil { return err }
	for _, entry := range entries {
		if !entry.IsDir() {
			filePath := filepath.Join(filepath.Dir(page.URI.Path), entry.Name())
			fmt.Printf("file \"%s\" delete\n", filePath)
			if err := os.Remove(filePath); err != nil { return err }
		}
	}
	return nil
}

// Page.ApplyComputedProperties()
//
// Applies computed properties to the page template data.
func (page *Page) ApplyComputedProperties(website *Website) {
	if len(page.Path) > 0 { page.TemplateData.Set("path", page.Path) }
	if len(page.Redirect) > 0 { page.TemplateData.Set("refresh", fmt.Sprintf("%d; url=%s", website.Config.RefreshInterval, page.Redirect)) }
	if len(page.Assets) > 0 { page.TemplateData.Set("assets", page.Assets.FileNames()) }
	if page.Feed != nil { page.TemplateData.Set("feed", page.Feed.TemplateData()) }
	if page.Feed != nil { page.TemplateData.Set("assets", append(page.Assets.FileNames(), "atom.xml")) }
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
	if page.TemplateData == nil { page.TemplateData = hypertemplates.TemplateData{} }
	if len(page.CreatedAt) > 0 { page.TemplateData.Set("created_at", page.CreatedAt) }
	if len(page.UpdatedAt) > 0 { page.TemplateData.Set("updated_at", page.UpdatedAt) }
	if len(page.LayoutRef) > 0 { page.TemplateData.Set("layout", page.LayoutRef) }
	if len(page.Title) > 0 { page.TemplateData.Set("title", page.Title) }
	if len(page.Description) > 0 { page.TemplateData.Set("description", page.Description) }
	if page.Author != nil { page.TemplateData.Set("author", page.Author.TemplateData()) }
	if len(page.Contributors) > 0 { page.TemplateData.Set("contributors", page.Contributors.TemplateData()) }
	if len(page.Tags) > 0 { page.TemplateData.Set("tags", page.Tags) }
	if page.Draft { page.TemplateData.Set("draft", page.Draft) }
	if page.Unlisted { page.TemplateData.Set("unlisted", page.Unlisted) }
	if len(page.Redirect) > 0 { page.TemplateData.Set("redirect", page.Redirect) }
	if len(page.Assets) > 0 { page.TemplateData.Set("assets", page.Assets.FileNames()) }
	if len(page.Attachments) > 0 { page.TemplateData.Set("attachments", page.Attachments.TemplateData()) }
	if len(page.Summary) > 0 { page.TemplateData.Set("summary", page.Summary) }
	if len(page.More) > 0 { page.TemplateData.Set("more", page.More) }
	page.ApplyComputedProperties(website)
}

// Page.IndexPath()
//
// Returns the final page index path (i.e. URL path).
func (page *Page) IndexPath() (string) {
	return filepath.Join(page.Path, "index.html")
}

// Pages.LastModified()
//
// Returns the last modified timestamp (page.UpdatedAt) of the newest page.
// If the page has not been modified, it returns the creation timestamp (page.CreatedAt).
func (pages Pages) LastModified() (timestamp time.Time) {
	timestamp,_ = time.Parse(time.RFC3339, "1970-01-01T00:00:00Z") // default timestamp
	if len(pages) == 0 {
		return timestamp
	} else {
		return pages[0].LastModified()
	}
}
func (page *Page) LastModified() (time.Time) {
	if page.UpdatedAt == "" { page.UpdatedAt = page.CreatedAt }
	ts,err := time.Parse(time.RFC3339, page.UpdatedAt)
	if err != nil { ts,_ = time.Parse(time.RFC3339, "1970-01-01T00:00:00Z") }
	return ts
}

func (pages Pages) Contains(page *Page) (bool) {
	return pages.GetURI(page.URI) != nil
}

func (pages Pages) GetURI(uri *url.URL) (*Page) {
	index := slices.IndexFunc(pages, func(page *Page)(bool){
		return strings.EqualFold(page.URI.Path, uri.Path)
	})
	if index > -1 { return pages[index] }
	return nil
}

func (pages Pages) GetPath(path string) (*Page) {
	index := slices.IndexFunc(pages, func(page *Page)(bool){
		return strings.EqualFold(page.Path, path)
	})
	if index > -1 { return pages[index] }
	return nil
}


func (pages Pages) Sort() {
	slices.SortFunc(pages, func(a, b *Page) int {
		return cmp.Compare(b.LastModified().UnixMilli(), a.LastModified().UnixMilli())
	})
}
func (pages Pages) SortBy(field string, ascending bool) {
	slices.SortFunc(pages, func(a, b *Page) int {
		if ascending { return cmp.Compare(a.ComparedBy(field), b.ComparedBy(field)) }
		return cmp.Compare(b.ComparedBy(field), a.ComparedBy(field))
	})
}

// NOTE: only called by hyperctl page list → page.SortBy() → page.ComparedBy()
func (page *Page) ComparedBy(field string) string {
	switch field {
		case "CreatedAt", "created_at": return page.CreatedAt
		case "UpdatedAt", "updated_at": return page.UpdatedAt
		case "Title", "title": return page.Title
		case "Path", "path": return page.Path
		default: return page.LastModified().Format(time.RFC3339)
	}
}

func (pages Pages) ColumnValues(field string) []string {
	values := []string{}
	for i := range pages {
		switch field {
			case "Path", "path": values = append(values, pages[i].Path)
			case "Title", "title": values = append(values, pages[i].Title)
			case "LastModified", "lastmod": values = append(values, pages[i].LastModified().Format(time.RFC3339))
		}
	}
	return values
}

func (pages Pages) ContainsPath(href string) (bool) {
	if len(pages) == 0 { return false } // guard
	for i := range pages {
		if strings.EqualFold(href, pages[i].Path) { return true }
	}
	return false
}

func (page *Page) HasChildPages() (bool) {
	entries, err := os.ReadDir(filepath.Dir(page.URI.Path))
	if err != nil { return false }
	for _, entry := range entries {
		if entry.IsDir() { return true }
	}
	return false
}

func (pages Pages) TemplateData() (data []hypertemplates.TemplateData) {
	for i := range pages {
		data = append(data, pages[i].TemplateData)
	}
	return data
}

func (pages Pages) DiscoverAssets(website *Website, verbose bool) {
	for i := range pages {
		pages[i].DiscoverAssets(website, verbose)
	}
}
func (page *Page) DiscoverAssets(website *Website, verbose bool) {
	exclude := fmt.Sprintf("%sindex\\.(md|yaml|json)$", string(filepath.Separator))
	page.Assets = hypertemplates.GetFilesByPattern(filepath.Dir(page.URI.Path), false, "", exclude)
	page.Assets.SetRoot(website.ContentDir())
	if !verbose { return }
	for _,asset := range page.Assets {
		fmt.Printf("asset \"%s\" discovered\n", website.RelativePath(asset.URI.Path))
	}
}

func (page Page) AtomEntry(website *Website) (entry *atom.Entry) {
	var atomLinks []atom.Link
	var atomAuthor *atom.Person
	var atomContributors []*atom.Person
	var err error
	if atomLinks,err = page.AtomEntryLinks(website); err != nil { return nil }
	if atomAuthor,err = page.AtomEntryAuthor(website); err != nil { return nil }
	if atomContributors,err = page.AtomEntryContributors(website); err != nil { return nil }
	return &atom.Entry{
		Title: page.AtomEntryTitle(),
		ID: atomLinks[len(atomLinks)-1].Href,
		Link: atomLinks,
		Published: atom.TimeStamp{ RawValue: page.CreatedAt },
		Updated: atom.TimeStamp{ RawValue: page.UpdatedAt },
		Author: atomAuthor,
		Contributor: atomContributors,
		Summary: page.AtomSummary(),
		Content: page.AtomContent(),
	}
}

func (page *Page) AtomEntryLinks(website *Website) (atomLinks []atom.Link, err error) {
	var atomURL *url.URL
	var atomRelatedURL *url.URL
	if atomURL,err = website.ResolveHref(page.AtomEntryLink()); err != nil { return nil, err }
	if atomRelatedURL,err = website.ResolveHref(page.Path); err != nil { return nil, err }
	return []atom.Link{
		{ Rel: "alternate", Type: "text/html", Href: atomURL.String(), },
		{ Rel: "via", Type: "text/html", Href: atomURL.String(), },
		{ Rel: "related", Type: "text/html", Href: atomRelatedURL.String(), },
	}, nil
}

func (page *Page) AtomEntryLink() (string) {
	redirected := len(page.Redirect) > 0
	nocontent  := len(page.Content) == 0
	if redirected && nocontent { return page.Redirect }
	return page.Path
}

func (page *Page) AtomEntryAuthor(website *Website) (person *atom.Person, err error) {
	if page.Author == nil { page.Author = website.Author }
	return HyperFeed{}.AtomPerson(website, page.Author)
}

func (page *Page) AtomEntryContributors(website *Website) (contributors []*atom.Person, err error) {
	return HyperFeed{}.AtomPersons(website, page.Contributors)
}

func (page *Page) AtomEntryTitle() (title string) {
	if len(page.Title) > 0 { return page.Title }
	if len(page.Contributors) > 0 && len(page.Content) == 0 { return fmt.Sprintf("%s (%s)", page.Contributors[0].Name, page.Contributors[0].Username) }
	if page.Author != nil { return fmt.Sprintf("%s (%s)", page.Author.Name, page.Author.Username) }
	return ""
}

func (page *Page) AtomSummary() (summary *atom.Text) {
	if len(page.Summary) > 0 { return &atom.Text{ Type: "html", Body: page.Summary } }
	if len(page.Content) > 0 { return &atom.Text{ Type: "html", Body: page.Content } }
	if len(page.Attachments.FilteredBy(AttachmentKindLink)) > 0 {
		attachment := page.Attachments.FilteredBy(AttachmentKindLink)[0]
		attachment.Data.Get("hypergraph.description")
		return &atom.Text{ Type: "html", Body: fmt.Sprintf("Link to %s", attachment.Href) }
	}
	if len(page.More) > 0 { return &atom.Text{ Type: "html", Body: page.More } }
	ts, err := time.Parse(time.RFC3339, page.CreatedAt)
	if err != nil { return &atom.Text{ Type: "html", Body: page.CreatedAt } }
	return &atom.Text{ Type: "html", Body: ts.Format("Monday, January 2, 2006") }
}
func (page *Page) AtomContent() (content *atom.Text) {
	if len(page.Content) > 0 { return &atom.Text{ Type: "html", Body: page.Content } }
	return nil
}

func (page *Page) MarshalYAML() (any, error) {
	return struct{
		Page *Page `yaml:",inline"`
		Other hypertemplates.TemplateData `yaml:",inline"`
	}{
		Page: page,
		Other: page.TemplateData.Clean([]string{"created_at","updated_at","layout","author","contributors","title","description","summary","preview","tags","unlisted","redirect","attachments"}),
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
	// timestamp
	timestamp := time.Now().Format(time.RFC3339)
	if len(page.CreatedAt) == 0 { page.CreatedAt = timestamp } // only set created_at if it is empty
	page.UpdatedAt = timestamp

	// content-type
	if contenttype.IsEmpty() { contenttype = &DefaultContentType }
	if !page.Draft { if err = contenttype.Validate(page); err != nil { return err } }
	if err = contenttype.Apply(website, page); err != nil { return err }

	// write to disk
	return WritePage(website, page, contenttype.Spec.Format)
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
			if fileBytes,err = json.Marshal(page.TemplateData); err != nil { return err }
		case "yaml", "yml":
			page.TemplateData.Set("content", page.Content) // add content to template data
			if fileBytes,err = yaml.Marshal(page.TemplateData); err != nil { return err }
		case "markdown", "md": fallthrough
		default:
			if frontmatter,err = yaml.Marshal(hypertemplates.OrderedTemplateData(page.TemplateData)); err != nil { return err }
			fileBytes = fmt.Appendf([]byte{}, "---\n%s---\n\n%s", frontmatter, page.Content)
	}
	fmt.Printf("writing %d bytes to %s\n", len(fileBytes), website.RelativePath(page.URI.Path))
	if err = os.MkdirAll(filepath.Dir(page.URI.Path), 0755); err != nil { return err }
	if file, err = os.OpenFile(page.URI.Path, os.O_CREATE|os.O_WRONLY, 0644); err != nil { return err }; defer file.Close()
	if fileBytesWritten, err = file.Write(fileBytes); err != nil { return err }
	if fileBytesWritten != len(fileBytes) { return fmt.Errorf("expected to write %v bytes, but wrote %v", len(fileBytes), fileBytesWritten) }
	return nil
}

// LoadPages(ctx context.Context, website *Website, pages Pages, cache *Pages, verbose bool)
//
// Loads the pages in parallel using a task group.
func LoadPages(ctx context.Context, website *Website, pages Pages, cache Pages, verbose bool) (err error) {
	if cache == nil { cache = Pages{} }
	taskgroup,_ := errgroup.WithContext(ctx)
	for i := range pages {
		taskgroup.Go(func() (error) {
			if cache.Contains(pages[i]) { pages[i] = cache.GetURI(pages[i].URI) }
			return LoadPage(website, pages[i], verbose)
		})
	}
	return taskgroup.Wait()
}

// LoadPage(website *Website, page *Page, verbose bool)
//
// Loads the page from disk and parses the content into the page object.
func LoadPage(website *Website, page *Page, verbose bool) (err error) {
	var fileExt   string = filepath.Ext(page.URI.Path)
	var fileBytes []byte
	var fileData  hypertemplates.TemplateData
	if page.Loaded { return } // guard
	if verbose { fmt.Printf("page \"%s\" loading\n", website.RelativePath(page.URI.Path)) }
	if fileBytes,err = os.ReadFile(page.URI.Path); err != nil { return err }
	if fileData,err = hypertemplates.ParseTemplateDataBytes(fileBytes, fileExt, website.Config.Markdown); err != nil { return err }
	page.CreatedAt = website.CreatedAt // default created_at to website created_at
	page.TemplateData = fileData
	page.Parse(fileData)
	if page.LayoutRef == "" { page.LayoutRef = "default" } // TODO: should layout be required?
	page.DiscoverAssets(website, verbose)
	page.Loaded = true;
	return nil
}

// AddFeedPages(context context.Context, build *HyperBuild)
//
// Adds pages to a build if a build page matches a non-build page feed.
func AddFeedPages(context context.Context, build *HyperBuild) (err error) {
	taskgroup,_ := errgroup.WithContext(context)
	for i := range build.Website.Pages {
		taskgroup.Go(func() (error) {
			if build.Website.Pages[i].Feed == nil { return nil } // skip page if no page feed is configured
			if !build.Website.Pages[i].Feed.MatchesAny(*build.Pages) { return nil } // check if a build page is in a site page feed
			*build.Pages = append(*build.Pages, build.Website.Pages[i]) // add page to build pages
			return nil
		})
	}
	return taskgroup.Wait()
}

// LoadPageFeeds(website *Website, pages Pages)
func LoadPageFeeds(context context.Context, website *Website, pages Pages) (err error) {
	taskgroup,_ := errgroup.WithContext(context)
	website.Pages.ResolveTemplateData(website) // resolve template data for source pages
	for i := range pages {
		taskgroup.Go(func() (error) {
			if pages[i].Feed == nil { return nil } // skip page if no page feed is configured
			if len(pages[i].Feed.Title) == 0 { pages[i].Feed.Title = website.Title }
			if len(pages[i].Feed.Description) == 0 { pages[i].Feed.Description = website.Description }
			if pages[i].Feed.Author == nil { pages[i].Feed.Author = website.Author }
			if err = pages[i].Feed.AddPages(website.Pages...); err != nil { return err }
			pages[i].Feed.Pages.Sort()     // sort the feed pages in reverse-chron order, as nature intended 🤌🏽
			return nil
		})
	}
	return taskgroup.Wait()
}

// LoadTagPages(website *Website, pages Pages) (err error)
func LoadTagPages(build *HyperBuild) (err error) {
	for i := range *build.Pages { build.Tags.Increment(1, (*build.Pages)[i].Tags...) } // count tags
	for tag := range build.Tags.Counters {
		fmt.Printf("tag \"%s\" loading\n", tag)
		tagPath := filepath.Join( build.Website.Config.TagPath, tag )
		tagPage := build.Pages.GetPath(tagPath)
		if tagPage == nil {
			tagPage = &Page{
				URI: &url.URL{ Path: filepath.Join(build.Website.Config.ContentDirectory, tagPath, "index.md") },
				CreatedAt: build.Website.Pages[0].CreatedAt,
				LayoutRef: build.Website.Config.TagLayout,
				Path: tagPath,
				Title: fmt.Sprintf("#%s", tag),
				Description: fmt.Sprintf("Everything tagged with #%s", tag),
			}
		}
		if tagPage.Feed == nil {
			tagPage.Feed = &HyperFeed{
				Title: fmt.Sprintf("#%s", tag),
				Description: fmt.Sprintf("Everything tagged with #%s", tag),
				Author: build.Website.Author,
				Tags: []string{ tag },
				Pages: Pages{},
			}
		}
		tagPage.Feed.AddPages(build.Website.Pages...) // populate tag page feed with pages
		tagPage.ResolveTemplateData(build.Website) // resolve template data for tag page so it's ready for
		*build.Pages = append(*build.Pages, tagPage) // attach feed to the page
	}
	return nil
}

func GetPages(website *Website, directory string) (pages Pages, err error) {
	pages = Pages{} // "discovered pages"
	paths := hypertemplates.GetPathsByPattern(directory, true, "/index\\.(md|yaml|json)$", "")
	for _,p := range paths {
		var uri *url.URL
		if uri,err = url.Parse(p); err != nil { return nil, err }
		page := &Page{ URI: uri, Path: PagePath(website, uri), TemplateData: nil }
		pages = append(pages, page)
	}
	return pages, nil
}

func PagePath(website *Website, uri *url.URL) (string) {
	subPath := uri.Path
	subPath = strings.TrimPrefix(subPath, website.RootDir() + string(filepath.Separator))
	subPath = strings.TrimPrefix(subPath, website.Config.DraftsDirectory)
	subPath = strings.TrimPrefix(subPath, website.Config.ContentDirectory)
	subPath = filepath.Dir(subPath) + string(filepath.Separator)
	return strings.Replace(
		subPath,
		fmt.Sprintf("%s%s", string(filepath.Separator), string(filepath.Separator)),
		string(filepath.Separator),
		1,
	)
}

func PageIsDraft(website *Website, page *Page) (bool) {
	if page.Draft { return true }
	return strings.HasPrefix(page.URI.Path, website.DraftsDir())
}

