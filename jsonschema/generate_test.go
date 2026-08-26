package jsonschema_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/herdworks/htxt"
	"github.com/herdworks/htxt/jsonschema"
)

type child struct {
	Name string `json:"name" jsonschema:"required"`
}

type example struct {
	ID       int            `json:"id" jsonschema:"required"`
	Created  string         `json:"created" jsonschema:"format=date-time,description=Creation timestamp"`
	Status   string         `json:"status" jsonschema:"default=draft"`
	Child    *child         `json:"child,omitempty"`
	Children []child        `json:"children,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
	Ignored  string         `json:"-"`
}

func (example) JSONSchemaTitle() string       { return "Example" }
func (example) JSONSchemaDescription() string { return "An example schema." }
func (example) JSONSchemaName() string        { return "example" }
func (example) JSONSchemaSummary() string     { return "Example schema." }

func TestRegistryDescribesNamedProviders(t *testing.T) {
	registry := jsonschema.Registry{example{}}
	if err := registry.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := registry.Names(); len(got) != 1 || got[0] != "example" {
		t.Fatalf("Names() = %#v", got)
	}
	if entries := registry.Entries(); len(entries) != 1 || entries[0].Summary != "Example schema." {
		t.Fatalf("Entries() = %#v", entries)
	}
	entries := jsonschema.Schemas{{Name: "example", Summary: "Example schema."}}
	if got := entries.ColumnValues("Name"); len(got) != 1 || got[0] != "example" {
		t.Fatalf("ColumnValues(Name) = %#v", got)
	}
	if got := entries.ColumnValues("Description"); len(got) != 1 || got[0] != "Example schema." {
		t.Fatalf("ColumnValues(Description) = %#v", got)
	}
	if _, err := registry.Describe("example"); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Describe("missing"); err == nil {
		t.Fatal("Describe accepted an unknown schema")
	}
}

func TestGenerateBuildsSchemaFromStructTags(t *testing.T) {
	output, err := jsonschema.Generate(example{})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	var schema map[string]any
	if err := json.Unmarshal([]byte(output), &schema); err != nil {
		t.Fatalf("generated schema is invalid JSON: %v", err)
	}
	if schema["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
		t.Fatalf("$schema = %v", schema["$schema"])
	}
	if schema["title"] != "Example" || schema["description"] != "An example schema." {
		t.Fatalf("root metadata = %#v", schema)
	}
	defs := schema["$defs"].(map[string]any)
	if got := schema["$ref"]; got != "#/$defs/github.com~1herdworks~1htxt~1jsonschema_test.example" {
		t.Fatalf("$ref = %q", got)
	}
	root := defs["github.com/herdworks/htxt/jsonschema_test.example"].(map[string]any)
	if root["title"] != "Example" || root["description"] != "An example schema." {
		t.Fatalf("definition metadata = %#v", root)
	}
	if root["additionalProperties"] != true {
		t.Fatalf("root additionalProperties = %#v, want true", root["additionalProperties"])
	}
	properties := root["properties"].(map[string]any)
	if _, exists := properties["ignored"]; exists {
		t.Fatal("ignored JSON field was emitted")
	}
	if properties["created"].(map[string]any)["format"] != "date-time" {
		t.Fatalf("created format = %#v", properties["created"])
	}
	if properties["created"].(map[string]any)["description"] != "Creation timestamp" {
		t.Fatalf("created description = %#v", properties["created"])
	}
	if properties["status"].(map[string]any)["default"] != "draft" {
		t.Fatalf("status default = %#v", properties["status"])
	}
	required := root["required"].([]any)
	if len(required) != 1 || required[0] != "id" {
		t.Fatalf("required = %#v, want [id]", required)
	}
}

func TestGenerateRejectsUnsupportedTypes(t *testing.T) {
	type invalid struct {
		Callback func() `json:"callback"`
	}
	_, err := jsonschema.Generate(invalid{})
	if err == nil || !strings.Contains(err.Error(), "unsupported type") {
		t.Fatalf("Generate() error = %v, want unsupported-type error", err)
	}
}

func TestGenerateRejectsNonStructRoot(t *testing.T) {
	_, err := jsonschema.Generate("not a struct")
	if err == nil || !strings.Contains(err.Error(), "not a struct") {
		t.Fatalf("Generate() error = %v, want root-type error", err)
	}
}

func TestGenerateSupportsWebsiteAndPage(t *testing.T) {
	for _, test := range []struct {
		value        any
		definition   string
		required     []string
		dateProperty []string
	}{
		{
			value:        htxt.Website{},
			definition:   "github.com/herdworks/htxt.Website",
			required:     []string{"base_url", "title", "description", "byline"},
			dateProperty: []string{"created_at", "updated_at"},
		},
		{
			value:        htxt.Page{},
			definition:   "github.com/herdworks/htxt.Page",
			required:     nil,
			dateProperty: []string{"created_at", "updated_at"},
		},
	} {
		output, err := jsonschema.Generate(test.value)
		if err != nil {
			t.Fatalf("Generate(%T) error = %v", test.value, err)
		}
		if !json.Valid([]byte(output)) {
			t.Fatalf("Generate(%T) returned invalid JSON", test.value)
		}
		var schema map[string]any
		if err := json.Unmarshal([]byte(output), &schema); err != nil {
			t.Fatalf("Generate(%T) unmarshal error = %v", test.value, err)
		}
		definition := schema["$defs"].(map[string]any)[test.definition].(map[string]any)
		if definition["additionalProperties"] != true {
			t.Fatalf("Generate(%T) additionalProperties = %#v, want true", test.value, definition["additionalProperties"])
		}
		var required []any
		if value, ok := definition["required"]; ok {
			required = value.([]any)
		}
		if len(required) != len(test.required) {
			t.Fatalf("Generate(%T) required = %#v, want %#v", test.value, required, test.required)
		}
		for index, want := range test.required {
			if required[index] != want {
				t.Fatalf("Generate(%T) required = %#v, want %#v", test.value, required, test.required)
			}
		}
		properties := definition["properties"].(map[string]any)
		if test.definition == "github.com/herdworks/htxt.Website" {
			if schema["title"] != "HyperTexting website" {
				t.Fatalf("Website title = %#v", schema["title"])
			}
			if schema["description"] != "A website source configuration file. Use site.yaml, site.yml, or site.json." {
				t.Fatalf("Website description = %#v", schema["description"])
			}
			if definition["title"] != "HyperTexting website" {
				t.Fatalf("Website definition title = %#v", definition["title"])
			}
			byline := schema["$defs"].(map[string]any)["github.com/herdworks/htxt.Byline"].(map[string]any)
			if got := byline["required"]; len(got.([]any)) != 1 || got.([]any)[0] != "name" {
				t.Fatalf("Website byline required = %#v", got)
			}
			feeds := properties["feeds"].(map[string]any)["additionalProperties"].(map[string]any)
			if got, want := feeds["$ref"], "#/$defs/github.com~1herdworks~1htxt.FeedConfig"; got != want {
				t.Fatalf("Website feeds schema = %#v, want %q", feeds, want)
			}
			feedConfig := schema["$defs"].(map[string]any)["github.com/herdworks/htxt.FeedConfig"].(map[string]any)
			metadata := feedConfig["properties"].(map[string]any)["metadata"].(map[string]any)
			if got, want := metadata["$ref"], "#/$defs/github.com~1herdworks~1htxt.FeedMeta"; got != want {
				t.Fatalf("Website feed metadata schema = %#v, want %q", metadata, want)
			}
			for name, want := range map[string]string{
				"github.com/herdworks/htxt.Byline":         "Attribution details for a website, page, or feed.",
				"github.com/herdworks/htxt.FeedConfig":     "Defines Atom metadata, selectors, ordering, and an HTML alternate URL for a site feed.",
				"github.com/herdworks/htxt.FeedMeta":       "Optional Atom feed metadata that overrides the website defaults.",
				"github.com/herdworks/htxt.FeedSelector":   "Matches pages by path expression or tag for a site feed.",
				"github.com/herdworks/htxt.HTMLLink":       "An HTML link element represented as source data.",
				"github.com/herdworks/htxt.HTMLMeta":       "An HTML meta element represented as source data.",
				"github.com/herdworks/htxt.WebsiteConfig":  "Optional website paths and rendering settings.",
				"github.com/herdworks/htxt.WebsiteFavicon": "A favicon link included in the website HTML head.",
			} {
				definition := schema["$defs"].(map[string]any)[name].(map[string]any)
				if definition["description"] != want {
					t.Fatalf("%s description = %#v", name, definition["description"])
				}
			}
		}
		for _, name := range test.dateProperty {
			if properties[name].(map[string]any)["format"] != "date-time" {
				t.Fatalf("Generate(%T) %s format = %#v, want RFC3339 date-time", test.value, name, properties[name])
			}
		}
		if test.definition == "github.com/herdworks/htxt.Page" {
			if schema["title"] != "HyperTexting page" {
				t.Fatalf("Page title = %#v", schema["title"])
			}
			if schema["description"] != "A page source file may use Markdown, YAML, or JSON. For Markdown files, object properties form YAML front matter and the Markdown body is the 'content' field." {
				t.Fatalf("Page description = %#v", schema["description"])
			}
			if definition["title"] != "HyperTexting page" {
				t.Fatalf("Page definition title = %#v", definition["title"])
			}
			if properties["created_at"].(map[string]any)["description"] != "Defaults to 'site.created_at'." {
				t.Fatalf("Page created_at description = %#v", properties["created_at"])
			}
			if properties["layout"].(map[string]any)["default"] != "default" {
				t.Fatalf("Page layout default = %#v", properties["layout"])
			}
			if properties["content"].(map[string]any)["description"] != "In Markdown files this is the Markdown body not a YAML front-matter property." {
				t.Fatalf("Page content description = %#v", properties["content"])
			}
			for name, want := range map[string]string{
				"github.com/herdworks/htxt.Attachment": "A linked or embedded resource associated with a page.",
				"github.com/herdworks/htxt.FeedData":   "References a configured site feed or automatic tag feed for page template data. An empty entry uses its alias as the site feed identifier.",
			} {
				definition := schema["$defs"].(map[string]any)[name].(map[string]any)
				if definition["description"] != want {
					t.Fatalf("%s description = %#v", name, definition["description"])
				}
			}
		}
	}
}
