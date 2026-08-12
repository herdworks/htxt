package htxt

import (
	"encoding/json"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestAttachmentHrefUnmarshalsAsHref(t *testing.T) {
	tests := []struct {
		name   string
		decode func(*Attachment) error
	}{
		{
			name: "json",
			decode: func(attachment *Attachment) error {
				return json.Unmarshal([]byte(`{"kind":"link","href":"photo.png"}`), attachment)
			},
		},
		{
			name: "yaml",
			decode: func(attachment *Attachment) error {
				return yaml.Unmarshal([]byte("kind: link\nhref: photo.png\n"), attachment)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var attachment Attachment
			if err := test.decode(&attachment); err != nil {
				t.Fatalf("decode error = %v", err)
			}
			if attachment.Href == nil || attachment.Href.String() != "photo.png" {
				t.Fatalf("Href = %#v, want photo.png", attachment.Href)
			}
			if got := attachment.TemplateData()["href"]; got != "photo.png" {
				t.Fatalf("TemplateData()[href] = %#v, want photo.png", got)
			}
		})
	}
}

func TestAttachmentDataUnmarshalsJSONObject(t *testing.T) {
	var attachment Attachment
	if err := json.Unmarshal([]byte(`{
		"kind":"link",
		"href":"https://example.org/article",
		"data":{"hypergraph":{"title":"Example Article","description":"Metadata description"}}
	}`), &attachment); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if got := attachment.Data.Get("hypergraph.title"); got != "Example Article" {
		t.Fatalf("Data.Get(hypergraph.title) = %#v, want Example Article", got)
	}
	if got := attachment.Data.Get("hypergraph.description"); got != "Metadata description" {
		t.Fatalf("Data.Get(hypergraph.description) = %#v, want Metadata description", got)
	}
}

func TestAttachmentDataRejectsJSONString(t *testing.T) {
	var attachment Attachment
	if err := json.Unmarshal([]byte(`{
		"kind":"link",
		"href":"https://example.org/article",
		"data":"{\"hypergraph\":{\"title\":\"Example Article\",\"description\":\"Metadata description\"}}"
	}`), &attachment); err == nil {
		t.Fatal("json.Unmarshal() error = nil, want error")
	}
}
