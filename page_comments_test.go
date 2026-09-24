package htxt

import (
	"strings"
	"testing"

	"github.com/herdworks/httt"
)

func TestPageRenderDataPreservesAndClearsComments(t *testing.T) {
	for _, format := range []string{"md", "yaml", "json"} {
		t.Run(format, func(t *testing.T) {
			page := &Page{Links: []HTMLLink{{Href: "#comments", Rel: "comments", Title: "Discussion"}}}
			output, err := page.RenderData(nil, format)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(output), "#comments") || !strings.Contains(string(output), "Discussion") {
				t.Fatalf("links missing from serialized page: %s", output)
			}
			restored := &Page{}
			if err := restored.Parse(page.TemplateData); err != nil {
				t.Fatal(err)
			}
			if len(restored.Links) != 1 || restored.Links[0] != page.Links[0] {
				t.Fatalf("restored links = %#v", restored.Links)
			}
			restored.Links = nil
			output, err = restored.RenderData(nil, format)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(output), "comments") {
				t.Fatalf("cleared links remain in serialized page: %s", output)
			}
		})
	}
}

func TestPageResolveTemplateDataReplacesStaleLinks(t *testing.T) {
	page := &Page{
		Links:        []HTMLLink{{Href: "mailto:replies@example.com", Rel: "comments"}},
		TemplateData: httt.TemplateData{"links": []HTMLLink{{Href: "#old", Rel: "comments"}}},
	}
	output, err := page.RenderData(nil, "md")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(output), "#old") || !strings.Contains(string(output), "mailto:replies@example.com") {
		t.Fatalf("serialized links are stale: %s", output)
	}
}
