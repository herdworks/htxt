package hypertexting

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/herdworks/hyperfeed-go/types/atom"
	hypertemplates "github.com/herdworks/hypertemplates-go"
)

type HyperFeed struct {
	Title       string   `json:"title" yaml:"title"`
	Description string   `json:"description" yaml:"description"`
	Byline      *Byline  `json:"author" yaml:"author"`
	Tags        []string `json:"tags" yaml:"tags"`
	Paths       []string `json:"paths" yaml:"paths"`

	Pages Pages `json:"-" yaml:"-"`
}

func (HyperFeed) JSONSchemaTitle() string {
	return "HyperTexting feed"
}
func (HyperFeed) JSONSchemaName() string    { return "feed" }
func (HyperFeed) JSONSchemaSummary() string { return "Configuration for a page feed." }

func (HyperFeed) JSONSchemaDescription() string {
	return "Configuration for a page feed."
}

func (feed *HyperFeed) Clone() *HyperFeed {
	if feed == nil {
		return nil
	}
	clone := *feed
	clone.Byline = feed.Byline.Clone()
	clone.Tags = append([]string(nil), feed.Tags...)
	clone.Paths = append([]string(nil), feed.Paths...)
	clone.Pages = append(Pages(nil), feed.Pages...)
	return &clone
}

func (feed HyperFeed) TemplateData() (data hypertemplates.TemplateData) {
	data = hypertemplates.TemplateData{}
	data["title"] = feed.Title
	data["description"] = feed.Description
	if feed.Byline != nil {
		data["author"] = feed.Byline.TemplateData()
	}
	data["tags"] = feed.Tags
	data["pages"] = feed.Pages.TemplateDataWithout("feed")
	return data
}

func (feed *HyperFeed) Atom(website *Website, path string) (atomFeed *atom.Atom, err error) {
	var htmlURL *url.URL
	// var author *atom.Author
	if htmlURL, err = feed.ResolvedLink(website, path); err != nil {
		return nil, err
	}
	// if author,err = feed.AtomPerson(website, feed.Byline); err != nil { return nil,err }
	atomFeed = &atom.Atom{
		ID:    htmlURL.String(),
		Title: feed.Title,
		// Author: author.Name, // TODO: omit feed author? Should this be configurable?
		Links: []atom.Link{
			{Href: htmlURL.String(), Rel: "alternate", Type: "text/html"},
			{Href: htmlURL.JoinPath("atom.xml").String(), Rel: "self", Type: "application/atom+xml"},
		},
		Updated: atom.TimeStamp(feed.Pages.LastModified().Format(time.RFC3339)),
	}
	for _, page := range feed.Pages {
		entry := page.AtomEntry(website)
		atomFeed.Entries = append(atomFeed.Entries, entry)
	}
	return atomFeed, nil
}

func (feed *HyperFeed) ResolvedLink(website *Website, href string) (*url.URL, error) {
	target, err := url.Parse(href)
	if err != nil {
		return nil, err
	}
	if target.IsAbs() {
		return target, nil
	}
	link := website.URL()
	link.Path = filepath.Join(link.Path, target.Path)
	return link, nil
}

// HyperFeeds.Matching()
//
// Check if pages match a given feed.
func (feed *HyperFeed) MatchesAny(pages Pages, w io.Writer) bool {
	if w == nil {
		w = os.Stdout
	}

	for i := range pages {
		if feed.Matches(pages[i], w) {
			return true
		}
	}
	return false
}
func (feed *HyperFeed) Matches(page *Page, w io.Writer) bool {
	if w == nil {
		w = os.Stdout
	}

	if page.Unlisted {
		return false // never include unlisted pages
	}
	if page.Feed == feed {
		return false // feed pages cannot match themselves
	}
	if len(feed.Tags) == 0 && len(feed.Paths) == 0 {
		return true // no tag or path filters
	}
	if matches(feed.Tags, page.Tags...) {
		return true // feed tags match page tags
	}
	if matchesPatterns(feed.Paths, page.Path, w) {
		return true // feed paths matches prefix of page path
	}
	return false
}
func matches(a []string, b ...string) bool {
	for _, as := range a {
		if slices.ContainsFunc(b, func(bs string) bool {
			return strings.EqualFold(as, bs)
		}) {
			return true
		}
	}
	return false
}
func matchesPrefix(a []string, b ...string) bool {
	for _, as := range a {
		if slices.ContainsFunc(b, func(bs string) bool {
			return strings.HasPrefix(bs, as)
		}) {
			return true
		}
	}
	return false
}
func matchesPatterns(patterns []string, pagePath string, w io.Writer) bool {
	var pattern *regexp.Regexp
	var err error
	for i := range patterns {
		if pattern, err = regexp.Compile(patterns[i]); err != nil {
			fmt.Fprintf(w, "ignoring invalid feed pattern: %s\n\n%s\n", patterns[i], err)
			continue
		}
		if pattern.MatchString(pagePath) {
			return true
		}
	}
	return false
}

func (feed *HyperFeed) AddPages(w io.Writer, pages ...*Page) (err error) {
	if w == nil {
		w = os.Stdout
	}

	for i := range pages {
		if !feed.Matches(pages[i], w) {
			continue
		}
		feed.Pages = append(feed.Pages, pages[i])
	}
	return nil
}

func (feed HyperFeed) AtomPersons(website *Website, bylines []*Byline) (persons []*atom.Author, err error) {
	persons = []*atom.Author{}
	for _, byline := range bylines {
		person, err := feed.AtomPerson(website, byline)
		if err != nil {
			return nil, err
		}
		persons = append(persons, person)
	}
	return persons, nil
}

func (feed HyperFeed) AtomPerson(website *Website, byline *Byline) (person *atom.Author, err error) {
	uri, err := website.ResolveHref(byline.Href)
	if err != nil {
		return nil, err
	}
	favicon, err := website.ResolveHref(byline.Favicon)
	if err != nil {
		return nil, err
	}
	return &atom.Author{
		Name:    byline.Name,
		URI:     uri.String(),
		Favicon: favicon.String(),
		Email:   byline.Email,
	}, nil
}

func (feed *HyperFeed) WriteAtom(build *Build, path string, w io.Writer) (err error) {
	if w == nil {
		w = os.Stdout
	}

	var atomFeed *atom.Atom
	atomRelPath := strings.TrimPrefix(strings.ToLower(filepath.ToSlash(filepath.Join(path, "atom.xml"))), "/")
	atomPath, err := build.AbsolutePath(atomRelPath)
	if err != nil {
		return err
	}
	var atomBytes []byte
	if atomFeed, err = feed.Atom(build.Website, path); err != nil {
		return err
	}
	atomFeed.XMLNS = "https://hypergraph.network/xmlns" // TODO: make this configurable?
	if atomBytes, err = xml.MarshalIndent(atomFeed, "", "\t"); err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(atomPath), 0755); err != nil {
		return err
	}
	build.Status.Bytes.Increment(len(atomBytes), "feeds")
	return build.WriteFile(atomPath, atomBytes, w)
}
