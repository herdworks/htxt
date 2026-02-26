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
	Name    string `json:"name" yaml:"name"`
	Favicon string `json:"favicon,omitempty" yaml:"favicon,omitempty"`
	Href    string `json:"href,omitempty" yaml:"href,omitempty"`
	Email   string `json:"email,omitempty" yaml:"email,omitempty"`
}
type Bylines []*Byline

func (byline *Byline) Ready() bool {
	return len(strings.TrimSpace(byline.Name)) > 0
}

func (byline *Byline) TemplateData() (data hypertemplates.TemplateData) {
	data = hypertemplates.TemplateData{}
	data["name"] = byline.Name
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
