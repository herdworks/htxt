package hypertexting

import (
	"encoding/json"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	hypertemplates "github.com/herdworks/hypertemplates-go"
	"github.com/herdworks/hypertexting-go/href"
	htpatch "github.com/herdworks/hypertexting-go/patch"
	htprompt "github.com/herdworks/hypertexting-go/prompt"
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

func TestContentTypeSaveUsesSpecPromptsAndPatches(t *testing.T) {
	website, _ := testWebsite(t)
	path := filepath.Join(website.Theme.ContentTypesDirPath(), "prompted.json")
	contenttype := &ContentType{
		URI:      &href.Href{URL: url.URL{Path: path}},
		Metadata: ContentTypeMetadata{Name: "Prompted"},
		Spec: ContentTypeSpecification{
			Layout:      "default",
			Path:        "posts",
			URIResolver: TitleKabobURIResolver,
			Format:      "markdown",
			Prompts:     htprompt.Prompts{{ID: "title", Type: htprompt.TypeText}},
			Patches:     htpatch.Patches{{Op: "add", Path: "page.title", ValueRef: "title"}},
		},
		State: ContentTypeState{UserData: map[string]any{"title": "Runtime Title"}},
	}

	if err := contenttype.Save(website, io.Discard); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(bytes, &raw); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if _, ok := raw["prompts"]; ok {
		t.Fatalf("saved content type emitted top-level prompts: %s", bytes)
	}
	if _, ok := raw["patches"]; ok {
		t.Fatalf("saved content type emitted top-level patches: %s", bytes)
	}
	if _, ok := raw["state"]; ok {
		t.Fatalf("saved content type emitted state: %s", bytes)
	}
	spec, ok := raw["spec"].(map[string]any)
	if !ok {
		t.Fatalf("saved spec = %#v, want object", raw["spec"])
	}
	if _, ok := spec["prompts"]; !ok {
		t.Fatalf("saved content type missing spec.prompts: %s", bytes)
	}
	if _, ok := spec["patches"]; !ok {
		t.Fatalf("saved content type missing spec.patches: %s", bytes)
	}
	prompts, ok := spec["prompts"].([]any)
	if !ok || len(prompts) != 1 {
		t.Fatalf("saved prompts = %#v, want one prompt", spec["prompts"])
	}
	prompt, ok := prompts[0].(map[string]any)
	if !ok {
		t.Fatalf("saved prompt = %#v, want object", prompts[0])
	}
	if _, ok := prompt["value"]; ok {
		t.Fatalf("saved content type persisted runtime prompt value: %s", bytes)
	}
	if strings.Contains(string(bytes), "Runtime Title") || strings.Contains(string(bytes), "user_data") {
		t.Fatalf("saved content type persisted state: %s", bytes)
	}

	loaded := &ContentType{URI: &href.Href{URL: url.URL{Path: path}}}
	if err := loaded.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(loaded.Spec.Prompts) != 1 || len(loaded.Spec.Patches) != 1 {
		t.Fatalf("loaded prompts/patches = %d/%d, want 1/1", len(loaded.Spec.Prompts), len(loaded.Spec.Patches))
	}
	if loaded.State.UserData != nil {
		t.Fatalf("loaded State.UserData = %#v, want nil", loaded.State.UserData)
	}
}

func TestContentTypeSaveDoesNotSerializeStateUserData(t *testing.T) {
	website, _ := testWebsite(t)
	path := filepath.Join(website.Theme.ContentTypesDirPath(), "prompted.json")
	contenttype := &ContentType{
		URI:      &href.Href{URL: url.URL{Path: path}},
		Metadata: ContentTypeMetadata{Name: "Prompted"},
		Spec: ContentTypeSpecification{
			Layout:      "default",
			Path:        "posts",
			URIResolver: TitleKabobURIResolver,
			Format:      "markdown",
			Prompts:     htprompt.Prompts{{ID: "title", Type: htprompt.TypeText}},
			Patches:     htpatch.Patches{{Op: "add", Path: "page.title", ValueRef: "title"}},
		},
		State: ContentTypeState{UserData: map[string]any{"title": "Runtime Title"}},
	}

	if err := contenttype.Save(website, io.Discard); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if strings.Contains(string(bytes), "Runtime Title") || strings.Contains(string(bytes), "user_data") || strings.Contains(string(bytes), "state") {
		t.Fatalf("saved content type persisted state: %s", bytes)
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
		Attachments: Attachments{{Kind: AttachmentKindLink, Href: mustHref(t, "https://example.com")}},
	}

	if err := contenttype.Ready(page); err == nil || !strings.Contains(err.Error(), "content-type requires image attachment") {
		t.Fatalf("Ready() error = %v, want image attachment requirement", err)
	}

	page.Attachments = append(page.Attachments, &Attachment{Kind: AttachmentKindImage, Href: mustHref(t, "image.jpg")})
	if err := contenttype.Ready(page); err != nil {
		t.Fatalf("Ready() error = %v, want ready", err)
	}

	contenttype.Spec.Requires.Attachment = AttachmentKindDocument
	if err := contenttype.Ready(page); err == nil || !strings.Contains(err.Error(), "content-type requires document attachment") {
		t.Fatalf("Ready() error = %v, want document attachment requirement", err)
	}
	page.Attachments = append(page.Attachments, &Attachment{Kind: AttachmentKindDocument, Href: mustHref(t, "document.pdf")})
	if err := contenttype.Ready(page); err != nil {
		t.Fatalf("Ready() error = %v, want ready", err)
	}
}

func TestContentTypeValidateDefinitionChecksPatchPromptRefs(t *testing.T) {
	contenttype := ContentType{
		Spec: ContentTypeSpecification{
			Layout:      "default",
			URIResolver: TimestampURIResolver,
			Format:      "markdown",
			Prompts:     htprompt.Prompts{{ID: "tag", Type: htprompt.TypeText}},
			Patches:     htpatch.Patches{{Op: "append", Path: "page.tags", ValueRef: "tag"}},
		},
	}

	if err := contenttype.ValidateDefinition(); err != nil {
		t.Fatalf("ValidateDefinition() error = %v", err)
	}

	contenttype.Spec.Patches = htpatch.Patches{{Op: "add", Path: "page.title", ValueRef: "missing"}}
	if err := contenttype.ValidateDefinition(); err == nil || !strings.Contains(err.Error(), "does not match a prompt") {
		t.Fatalf("ValidateDefinition() error = %v, want missing prompt error", err)
	}
}

func TestContentTypeSpecificationMIMEType(t *testing.T) {
	tests := []struct {
		format string
		want   string
	}{
		{format: "", want: "text/markdown; charset=utf-8"},
		{format: "markdown", want: "text/markdown; charset=utf-8"},
		{format: "md", want: "text/markdown; charset=utf-8"},
		{format: "json", want: "application/json"},
		{format: "yaml", want: "application/yaml"},
		{format: "yml", want: "application/yaml"},
		{format: "binary", want: "application/octet-stream"},
	}

	for _, tc := range tests {
		t.Run(tc.format, func(t *testing.T) {
			got := (ContentTypeSpecification{Format: tc.format}).MIMEType()
			if got != tc.want {
				t.Fatalf("MIMEType() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestContentTypeTemplateDataAppliesPatchesToSpecTemplateClone(t *testing.T) {
	contenttype := ContentType{
		Spec: ContentTypeSpecification{
			Layout:      "default",
			URIResolver: TimestampURIResolver,
			Format:      "markdown",
			TemplateData: hypertemplates.TemplateData{
				"title": "Base Title",
				"tags":  []string{"base"},
			},
			Prompts: htprompt.Prompts{
				{ID: "title", Type: htprompt.TypeText},
				{ID: "tag", Type: htprompt.TypeText},
				{ID: "featured", Type: htprompt.TypeBoolean},
			},
			Patches: htpatch.Patches{
				{Op: "replace", Path: "page.title", ValueRef: "title"},
				{Op: "append", Path: "page.tags", ValueRef: "tag"},
				{Op: "add", Path: "page.featured", ValueRef: "featured"},
			},
		},
		State: ContentTypeState{UserData: map[string]any{
			"title":    "Prompt Title",
			"tag":      "prompt-tag",
			"featured": true,
		}},
	}

	resolved, err := contenttype.TemplateData()
	if err != nil {
		t.Fatalf("TemplateData() error = %v", err)
	}
	if got := resolved.Get("title"); got != "Prompt Title" {
		t.Fatalf("resolved title = %v, want prompt title", got)
	}
	if got := resolved.Get("tags"); !reflect.DeepEqual(got, []string{"base", "prompt-tag"}) {
		t.Fatalf("resolved tags = %#v, want base and prompt tag", got)
	}
	if got := resolved.Get("featured"); got != true {
		t.Fatalf("resolved featured = %v, want true", got)
	}
	if got := contenttype.Spec.TemplateData.Get("title"); got != "Base Title" {
		t.Fatalf("spec title mutated to %v, want Base Title", got)
	}
	if got := contenttype.Spec.TemplateData.Get("tags"); !reflect.DeepEqual(got, []string{"base"}) {
		t.Fatalf("spec tags mutated to %#v, want base only", got)
	}
	if got := contenttype.Spec.TemplateData.Get("featured"); got != nil {
		t.Fatalf("spec featured mutated to %v, want unset", got)
	}

	contenttype.State.UserData["tag"] = "other-tag"
	resolved, err = contenttype.TemplateData()
	if err != nil {
		t.Fatalf("second TemplateData() error = %v", err)
	}
	if got := resolved.Get("tags"); !reflect.DeepEqual(got, []string{"base", "other-tag"}) {
		t.Fatalf("resolved tags after prompt change = %#v, want base and other-tag", got)
	}
}

func TestContentTypeTemplateDataUsesSeededPromptDefault(t *testing.T) {
	contenttype := ContentType{
		Spec: ContentTypeSpecification{
			Layout:      "default",
			URIResolver: TimestampURIResolver,
			Format:      "markdown",
			Prompts:     htprompt.Prompts{{ID: "title", Type: htprompt.TypeText, Default: "Default Title"}},
			Patches:     htpatch.Patches{{Op: "add", Path: "page.title", ValueRef: "title"}},
		},
	}
	contenttype.State.SeedDefaults(contenttype.Spec.Prompts)

	resolved, err := contenttype.TemplateData()
	if err != nil {
		t.Fatalf("TemplateData() error = %v", err)
	}
	if got := resolved.Get("title"); got != "Default Title" {
		t.Fatalf("resolved title = %q, want Default Title", got)
	}
}

func TestContentTypeTemplateDataReadsStateUserDataDirectly(t *testing.T) {
	contenttype := ContentType{
		Spec: ContentTypeSpecification{
			Layout:      "default",
			URIResolver: TimestampURIResolver,
			Format:      "markdown",
			Prompts: htprompt.Prompts{
				{ID: "title", Type: htprompt.TypeText},
				{ID: "featured", Type: htprompt.TypeBoolean},
			},
			Patches: htpatch.Patches{
				{Op: "add", Path: "page.title", ValueRef: "title"},
				{Op: "add", Path: "page.featured", ValueRef: "featured"},
			},
		},
	}

	if _, err := contenttype.TemplateData(); err == nil || !strings.Contains(err.Error(), "references unknown value_ref") {
		t.Fatalf("TemplateData() error = %v, want missing user_data value_ref error", err)
	}

	contenttype.State.UserData = map[string]any{"title": true, "featured": true}
	if _, err := contenttype.TemplateData(); err == nil || !strings.Contains(err.Error(), "want text") {
		t.Fatalf("TemplateData() error = %v, want invalid user_data type error", err)
	}

	contenttype.State.UserData = map[string]any{"title": "State Title", "featured": true, "unknown": true}
	if _, err := contenttype.TemplateData(); err == nil || !strings.Contains(err.Error(), "does not match a prompt") {
		t.Fatalf("TemplateData() error = %v, want unknown user_data key error", err)
	}

	delete(contenttype.State.UserData, "unknown")
	resolved, err := contenttype.TemplateData()
	if err != nil {
		t.Fatalf("TemplateData() error = %v", err)
	}
	if got := resolved.Get("title"); got != "State Title" {
		t.Fatalf("resolved title = %q, want State Title", got)
	}
	if got := resolved.Get("featured"); got != true {
		t.Fatalf("resolved featured = %v, want true", got)
	}
}

func TestContentTypeTemplateDataAddIgnoresPageDataAndReplaceRequiresSpecTemplate(t *testing.T) {
	contenttype := ContentType{
		Spec: ContentTypeSpecification{
			Layout:      "default",
			URIResolver: TimestampURIResolver,
			Format:      "markdown",
			Prompts:     htprompt.Prompts{{ID: "title", Type: htprompt.TypeText}},
			Patches:     htpatch.Patches{{Op: "add", Path: "page.title", ValueRef: "title"}},
		},
		State: ContentTypeState{UserData: map[string]any{"title": "Prompt Title"}},
	}
	page := &Page{Title: "Page Title", TemplateData: hypertemplates.TemplateData{"title": "Page Title"}}

	candidate, err := contenttype.OverlayPage(page)
	if err != nil {
		t.Fatalf("OverlayPage() error = %v", err)
	}
	if candidate.Title != "Prompt Title" {
		t.Fatalf("candidate.Title = %q, want Prompt Title", candidate.Title)
	}
	if page.Title != "Page Title" {
		t.Fatalf("page.Title = %q, want source page unchanged", page.Title)
	}

	contenttype.Spec.Patches = htpatch.Patches{{Op: "replace", Path: "page.title", ValueRef: "title"}}
	if _, err := contenttype.TemplateData(); err == nil || !strings.Contains(err.Error(), "key does not exist") {
		t.Fatalf("TemplateData() error = %v, want missing replace target", err)
	}
}

func TestContentTypeOverlayPageUsesOverlayPrecedenceWithoutMutatingPage(t *testing.T) {
	contenttype := ContentType{
		Spec: ContentTypeSpecification{
			Layout:      "default",
			URIResolver: TimestampURIResolver,
			Format:      "markdown",
			TemplateData: hypertemplates.TemplateData{
				"title":   "Spec Title",
				"summary": "Spec summary",
				"tags":    []string{"spec"},
				"settings": hypertemplates.TemplateData{
					"theme":  "default",
					"accent": "blue",
				},
			},
			Prompts: htprompt.Prompts{
				{ID: "title", Type: htprompt.TypeText},
				{ID: "tag", Type: htprompt.TypeText},
			},
			Patches: htpatch.Patches{
				{Op: "replace", Path: "page.title", ValueRef: "title"},
				{Op: "append", Path: "page.tags", ValueRef: "tag"},
			},
		},
		State: ContentTypeState{UserData: map[string]any{
			"title": "Prompt Title",
			"tag":   "prompt-tag",
		}},
	}
	page := &Page{
		Title:   "Page Title",
		Summary: "Page summary",
		Tags:    []string{"page"},
		TemplateData: hypertemplates.TemplateData{
			"title":   "Page Title",
			"summary": "Page summary",
			"tags":    []string{"page"},
			"settings": hypertemplates.TemplateData{
				"accent": "red",
				"custom": true,
			},
		},
	}

	candidate, err := contenttype.OverlayPage(page)
	if err != nil {
		t.Fatalf("OverlayPage() error = %v", err)
	}
	if candidate.Title != "Prompt Title" {
		t.Fatalf("candidate.Title = %q, want prompt title", candidate.Title)
	}
	if candidate.Summary != "Spec summary" {
		t.Fatalf("candidate.Summary = %q, want spec summary", candidate.Summary)
	}
	if got := candidate.TemplateData.Get("tags"); !reflect.DeepEqual(got, []string{"spec", "prompt-tag"}) {
		t.Fatalf("candidate tags = %#v, want spec and prompt tag", got)
	}
	if got := candidate.TemplateData.Get("settings.theme"); got != "default" {
		t.Fatalf("settings.theme = %v, want default", got)
	}
	if got := candidate.TemplateData.Get("settings.accent"); got != "blue" {
		t.Fatalf("settings.accent = %v, want content-type overlay", got)
	}
	if got := candidate.TemplateData.Get("settings.custom"); got != true {
		t.Fatalf("settings.custom = %v, want page custom value preserved", got)
	}
	if page.Title != "Page Title" || page.Summary != "Page summary" || !slices.Equal(page.Tags, []string{"page"}) {
		t.Fatalf("source page mutated: %#v", page)
	}
}

func TestContentTypeCandidateReadyUsesOverlayWithoutMutatingPage(t *testing.T) {
	contenttype := ContentType{
		Spec: ContentTypeSpecification{
			Layout:      "default",
			URIResolver: TimestampURIResolver,
			Format:      "markdown",
			Requires:    ContentTypeRequirements{Title: true},
			Prompts:     htprompt.Prompts{{ID: "title", Type: htprompt.TypeText}},
			Patches:     htpatch.Patches{{Op: "add", Path: "page.title", ValueRef: "title"}},
		},
		State: ContentTypeState{UserData: map[string]any{"title": "Prompt Title"}},
	}
	page := &Page{TemplateData: hypertemplates.TemplateData{"tags": []string{"existing"}}}

	if err := contenttype.Ready(page); err == nil || !strings.Contains(err.Error(), "content-type requires page.title property") {
		t.Fatalf("Ready() error = %v, want missing title requirement", err)
	}
	if err := contenttype.CandidateReady(page); err != nil {
		t.Fatalf("CandidateReady() error = %v, want ready preview", err)
	}
	if err := contenttype.CandidateReady(page); err != nil {
		t.Fatalf("second CandidateReady() error = %v, want ready preview", err)
	}
	if page.Title != "" {
		t.Fatalf("page.Title = %q, want unchanged empty title", page.Title)
	}
	if got := page.TemplateData.Get("title"); got != nil {
		t.Fatalf("template title = %v, want unset", got)
	}
	if got := page.TemplateData.Get("tags"); !reflect.DeepEqual(got, []string{"existing"}) {
		t.Fatalf("template tags = %#v, want unchanged existing tag", got)
	}
}
