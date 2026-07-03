package hypertexting

import (
	"strings"

	hypertemplates "github.com/herdworks/hypertemplates-go"
)

// Page
//
// Page objects do the heavy lifting to parse content data files
// and convert them into rendered pages.
type Byline struct {
	Name     string `json:"name" yaml:"name"`
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

func (byline *Byline) TemplateData() (data hypertemplates.TemplateData) {
	data = hypertemplates.TemplateData{}
	data["name"] = byline.Name
	data["username"] = byline.Username
	data["favicon"] = byline.Favicon
	data["href"] = byline.Href
	data["email"] = byline.Email
	return data
}

func (bylines Bylines) TemplateData() (data []map[string]any) {
	for _, byline := range bylines {
		data = append(data, byline.TemplateData())
	}
	return data
}
