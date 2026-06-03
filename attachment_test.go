package hypertexting

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
