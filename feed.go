package hypertexting

import (
	"encoding/xml"
	"fmt"
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

func (feed HyperFeed) TemplateData() (data hypertemplates.TemplateData) {
	data = hypertemplates.TemplateData{}
	data["title"] = feed.Title
	data["description"] = feed.Description
	if feed.Byline != nil {
		data["author"] = feed.Byline.TemplateData()
	}
	data["tags"] = feed.Tags
	data["pages"] = feed.Pages.TemplateData()
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
func (feed *HyperFeed) MatchesAny(pages Pages) bool {
	for i := range pages {
		if feed.Matches(pages[i]) {
			return true
		}
	}
	return false
}
func (feed *HyperFeed) Matches(page *Page) bool {
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
	if matchesPatterns(feed.Paths, page.Path) {
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
func matchesPatterns(patterns []string, pagePath string) bool {
	var pattern *regexp.Regexp
	var err error
	for i := range patterns {
		if pattern, err = regexp.Compile(patterns[i]); err != nil {
			fmt.Printf("ignoring invalid feed pattern: %s\n\n%s\n", patterns[i], err)
			continue
		}
		if pattern.MatchString(pagePath) {
			return true
		}
	}
	return false
}

func (feed *HyperFeed) AddPages(pages ...*Page) (err error) {
	for i := range pages {
		if !feed.Matches(pages[i]) {
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

func (feed *HyperFeed) WriteAtom(build *HyperBuild, path string) (err error) {
	var atomFeed *atom.Atom
	var atomPath string = filepath.Join(build.URI.Path, strings.ToLower(filepath.Join(path, "atom.xml")))
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
	build.Bytes.Increment(len(atomBytes), "feeds")
	return build.WriteFile(atomPath, atomBytes)
}
