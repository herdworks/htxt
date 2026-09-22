package htxt

import (
	"strings"

	"github.com/herdworks/httt"
	"github.com/herdworks/hyperfeed"
	"github.com/herdworks/hyperfeed/href"
)

// Page
//
// Page objects do the heavy lifting to parse content data files
// and convert them into rendered pages.
type Byline struct {
	Name     string `json:"name" yaml:"name" jsonschema:"required,description=The byline display name."`
	Username string `json:"username,omitempty" yaml:"username,omitempty"`
	Favicon  string `json:"favicon,omitempty" yaml:"favicon,omitempty"`
	Href     string `json:"href,omitempty" yaml:"href,omitempty"`
	Email    string `json:"email,omitempty" yaml:"email,omitempty"`
}
type Bylines []*Byline

func (byline *Byline) Clone() *Byline {
	if byline == nil {
		return nil
	}
	clone := *byline
	return &clone
}

func (bylines Bylines) Clone() Bylines {
	if bylines == nil {
		return nil
	}
	clones := make(Bylines, len(bylines))
	for i, byline := range bylines {
		clones[i] = byline.Clone()
	}
	return clones
}

func (byline *Byline) Ready() bool {
	return len(strings.TrimSpace(byline.Name)) > 0
}

func (byline *Byline) TemplateData() (data httt.TemplateData) {
	data = httt.TemplateData{}
	data["name"] = byline.Name
	data["username"] = byline.Username
	data["favicon"] = byline.Favicon
	data["href"] = byline.Href
	data["email"] = byline.Email
	return data
}

func (byline Byline) JSONSchemaTitle() string {
	return "HyperTexting byline"
}
func (Byline) JSONSchemaName() string    { return "byline" }
func (Byline) JSONSchemaSummary() string { return "Attribution details." }

func (byline Byline) JSONSchemaDescription() string {
	return "Attribution details for a website, page, or feed."
}

func (bylines Bylines) TemplateData() (data []map[string]any) {
	for _, byline := range bylines {
		data = append(data, byline.TemplateData())
	}
	return data
}

func (byline *Byline) toHyperFeedByline(website *Website, role string, order int) (hyperfeed.Byline, error) {
	if byline == nil {
		return hyperfeed.Byline{}, nil
	}
	personURL, err := website.ResolveHref(byline.Href)
	if err != nil {
		return hyperfeed.Byline{}, err
	}
	personHref, err := href.Parse(personURL)
	if err != nil {
		return hyperfeed.Byline{}, err
	}
	faviconURL, err := website.ResolveHref(byline.Favicon)
	if err != nil {
		return hyperfeed.Byline{}, err
	}
	faviconHref, err := href.Parse(faviconURL)
	if err != nil {
		return hyperfeed.Byline{}, err
	}
	return hyperfeed.Byline{Href: personHref, Name: byline.Name, Email: byline.Email, Favicon: faviconHref, Role: role, Order: order}, nil
}
