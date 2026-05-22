package hypertexting

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/herdworks/hypertexting-go/href"
)

func TestContentTypeSaveTruncatesExistingFile(t *testing.T) {
	website, _ := testWebsite(t)
	tests := []struct {
		name string
		ext  string
	}{
		{name: "json", ext: ".json"},
		{name: "yaml", ext: ".yaml"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(website.Theme.ContentTypesDirPath(), "article"+tc.ext)
			uri := &href.Href{URL: url.URL{Path: path}}
			first := &ContentType{
				URI: uri,
				Metadata: ContentTypeMetadata{
					Name:        "Article",
					Description: "stale description should disappear after shorter save",
				},
				Spec: ContentTypeSpecification{
					Layout:   "default",
					Path:     "articles-with-longer-path",
					AutoSlug: TitleKabobSlug,
					Format:   "markdown",
				},
			}
			if err := first.Save(website); err != nil {
				t.Fatalf("first Save() error = %v", err)
			}

			second := &ContentType{
				URI:      uri,
				Metadata: ContentTypeMetadata{Name: "Article"},
				Spec: ContentTypeSpecification{
					Layout: "default",
					Path:   "a",
					Format: "markdown",
				},
			}
			if err := second.Save(website); err != nil {
				t.Fatalf("second Save() error = %v", err)
			}

			bytes, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("ReadFile() error = %v", err)
			}
			contents := string(bytes)
			for _, stale := range []string{"stale description", "articles-with-longer-path"} {
				if strings.Contains(contents, stale) {
					t.Fatalf("saved content type still contains stale data %q:\n%s", stale, contents)
				}
			}

			loaded := &ContentType{URI: uri}
			if err := loaded.Load(); err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if loaded.Metadata.Description != "" {
				t.Fatalf("loaded Metadata.Description = %q, want empty", loaded.Metadata.Description)
			}
			if loaded.Spec.Path != "a" {
				t.Fatalf("loaded Spec.Path = %q, want %q", loaded.Spec.Path, "a")
			}
		})
	}
}

func TestContentTypeReadyValidatesDraftPageRequirements(t *testing.T) {
	contenttype := ContentType{
		Spec: ContentTypeSpecification{
			Requires: ContentTypeRequirements{Content: true},
		},
	}
	page := &Page{Draft: true}

	if err := contenttype.Ready(page); err == nil || !strings.Contains(err.Error(), "content-type requires page.content property") {
		t.Fatalf("Ready() error = %v, want content requirement", err)
	}
	if err := contenttype.Validate(page); err != nil {
		t.Fatalf("Validate() error = %v, want draft pages to skip validation", err)
	}
}

func TestContentTypeReadyValidatesSlugInputs(t *testing.T) {
	tests := []struct {
		name     string
		autoSlug ContentTypeSlugFormat
		page     *Page
		want     string
	}{
		{
			name:     "manual slug",
			autoSlug: ManualSlug,
			page:     &Page{Content: "ready"},
			want:     "content-type requires page.slug property",
		},
		{
			name:     "title kabob slug",
			autoSlug: TitleKabobSlug,
			page:     &Page{Content: "ready"},
			want:     "content-type requires page.title property",
		},
		{
			name:     "title snake slug",
			autoSlug: TitleSnakeSlug,
			page:     &Page{Content: "ready"},
			want:     "content-type requires page.title property",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			contenttype := ContentType{Spec: ContentTypeSpecification{AutoSlug: tc.autoSlug}}
			if err := contenttype.Ready(tc.page); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Ready() error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestContentTypeReadyValidatesAttachmentKind(t *testing.T) {
	contenttype := ContentType{
		Spec: ContentTypeSpecification{
			Requires: ContentTypeRequirements{Attachment: AttachmentKindImage},
		},
	}
	page := &Page{
		Draft:       true,
		Attachments: Attachments{{Kind: AttachmentKindLink, Href: "https://example.com"}},
	}

	if err := contenttype.Ready(page); err == nil || !strings.Contains(err.Error(), "content-type requires image attachment") {
		t.Fatalf("Ready() error = %v, want image attachment requirement", err)
	}

	page.Attachments = append(page.Attachments, &Attachment{Kind: AttachmentKindImage, Href: "image.jpg"})
	if err := contenttype.Ready(page); err != nil {
		t.Fatalf("Ready() error = %v, want ready", err)
	}
}
