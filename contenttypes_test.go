package hypertexting

import (
	"io"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	hypertemplates "github.com/herdworks/hypertemplates-go"
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
					Layout:      "default",
					Path:        "articles-with-longer-path",
					URIResolver: TitleKabobURIResolver,
					Format:      "markdown",
				},
			}
			if err := first.Save(website, io.Discard); err != nil {
				t.Fatalf("first Save() error = %v", err)
			}

			second := &ContentType{
				URI:      uri,
				Metadata: ContentTypeMetadata{Name: "Article"},
				Spec: ContentTypeSpecification{
					Layout:      "default",
					Path:        "a",
					URIResolver: TitleKabobURIResolver,
					Format:      "markdown",
				},
			}
			if err := second.Save(website, io.Discard); err != nil {
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

func TestContentTypeLoadRequiresURIResolver(t *testing.T) {
	website, _ := testWebsite(t)
	path := filepath.Join(website.Theme.ContentTypesDirPath(), "article.yaml")
	writePageTestFile(t, path, "metadata:\n  name: Article\nspec:\n  layout: default\n  path: posts\n  format: markdown\n")
	contenttype := &ContentType{URI: &href.Href{URL: url.URL{Path: path}}}

	err := contenttype.Load()
	if err == nil || !strings.Contains(err.Error(), "content-type spec.uri_resolver is required") {
		t.Fatalf("Load() error = %v, want uri_resolver requirement", err)
	}
}

func TestContentTypeSpecValidatesPathTemplateFunctions(t *testing.T) {
	base := ContentTypeSpecification{
		Layout:      "default",
		URIResolver: TimestampURIResolver,
		Format:      "markdown",
	}
	valid := base
	valid.Path = "/blog/${year}/${month}/${day}/"
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want valid path template", err)
	}

	tests := []struct {
		name string
		path string
	}{
		{name: "unknown function", path: "/blog/${hour}/"},
		{name: "builtin printf", path: `/blog/${printf "x"}/`},
		{name: "dot", path: "/blog/${.}/"},
		{name: "field", path: "/blog/${.Title}/"},
		{name: "function argument", path: `/blog/${year "x"}/`},
		{name: "pipeline", path: `/blog/${year | printf "%s"}/`},
		{name: "unsafe rendered path", path: "/blog/${year}/../../../x/"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			spec := base
			spec.Path = tc.path
			if err := spec.Validate(); err == nil {
				t.Fatalf("Validate() error = nil, want invalid path template")
			}
		})
	}
}

func TestContentTypeApplyLayersPageTemplateDataOverSpecDefaults(t *testing.T) {
	contenttype := &ContentType{
		Spec: ContentTypeSpecification{
			Layout: "default",
			TemplateData: hypertemplates.TemplateData{
				"title":   "Default Title",
				"summary": "Default summary",
				"tags":    []string{"default"},
				"settings": hypertemplates.TemplateData{
					"theme":  "default",
					"accent": "blue",
				},
			},
		},
	}
	page := &Page{
		TemplateData: hypertemplates.TemplateData{
			"title": "Page Title",
			"tags":  []string{"page"},
			"settings": hypertemplates.TemplateData{
				"accent": "red",
				"custom": true,
			},
		},
	}

	if err := contenttype.Apply(nil, page); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	if page.Title != "Page Title" {
		t.Fatalf("page.Title = %q, want page template data to override content type default", page.Title)
	}
	if page.Summary != "Default summary" {
		t.Fatalf("page.Summary = %q, want content type default", page.Summary)
	}
	if got := page.TemplateData.Get("title"); got != "Page Title" {
		t.Fatalf("template title = %v, want page value", got)
	}
	if got := page.TemplateData.Get("summary"); got != "Default summary" {
		t.Fatalf("template summary = %v, want content type default", got)
	}
	if got := page.TemplateData.Get("settings.theme"); got != "default" {
		t.Fatalf("settings.theme = %v, want content type default", got)
	}
	if got := page.TemplateData.Get("settings.accent"); got != "red" {
		t.Fatalf("settings.accent = %v, want page override", got)
	}
	if got := page.TemplateData.Get("settings.custom"); got != true {
		t.Fatalf("settings.custom = %v, want page value", got)
	}
	tags, ok := page.TemplateData.Get("tags").([]string)
	if !ok || !slices.Equal(tags, []string{"default", "page"}) {
		t.Fatalf("tags = %#v, want content type defaults followed by page tags", page.TemplateData.Get("tags"))
	}
}

func TestContentTypeReadyValidatesDraftPageRequirements(t *testing.T) {
	contenttype := ContentType{
		Spec: ContentTypeSpecification{
			Layout:      "default",
			URIResolver: TimestampURIResolver,
			Format:      "markdown",
			Requires:    ContentTypeRequirements{Content: true},
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
		resolver PageURIResolverName
		page     *Page
		want     string
	}{
		{
			name:     "manual slug",
			resolver: ManualURIResolver,
			page:     &Page{Content: "ready"},
			want:     "content-type requires page.slug property",
		},
		{
			name:     "title kabob slug",
			resolver: TitleKabobURIResolver,
			page:     &Page{Content: "ready"},
			want:     "content-type requires page.title property",
		},
		{
			name:     "title snake slug",
			resolver: TitleSnakeURIResolver,
			page:     &Page{Content: "ready"},
			want:     "content-type requires page.title property",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			contenttype := ContentType{Spec: ContentTypeSpecification{Layout: "default", URIResolver: tc.resolver, Format: "markdown"}}
			if err := contenttype.Ready(tc.page); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Ready() error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestContentTypeReadyValidatesAttachmentKind(t *testing.T) {
	contenttype := ContentType{
		Spec: ContentTypeSpecification{
			Layout:      "default",
			URIResolver: TimestampURIResolver,
			Format:      "markdown",
			Requires:    ContentTypeRequirements{Attachment: AttachmentKindImage},
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
