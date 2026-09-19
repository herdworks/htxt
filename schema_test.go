package htxt

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestSchemaRegistryIsJSONAndHasStableNames(t *testing.T) {
	want := []string{"attachment", "byline", "contact", "contact-email", "contact-link", "contact-phone", "contact-social-profile", "feed-config", "feed-data", "feed-selector", "html-link", "html-meta", "markdown", "mention-config", "page", "provider", "provider-filter-rules", "provider-secret", "sync", "sync-rules", "theme", "website", "website-config", "website-favicon"}
	if got := SchemaRegistry.Names(); !slices.Equal(got, want) {
		t.Fatalf("SchemaRegistry.Names() = %v, want %v", got, want)
	}
	for _, entry := range SchemaRegistry.Entries() {
		bytes, err := SchemaRegistry.Describe(entry.Name)
		if err != nil {
			t.Fatalf("%s: %v", entry.Name, err)
		}
		var document map[string]any
		if err = json.Unmarshal(bytes, &document); err != nil {
			t.Fatalf("%s is not JSON: %v", entry.Name, err)
		}
	}
}

func schemaDocument(t *testing.T, name string) []byte {
	t.Helper()
	document, err := SchemaRegistry.Describe(name)
	if err != nil {
		t.Fatal(err)
	}
	return document
}

func TestWebsiteAndPageSchemaDatesUseDateTime(t *testing.T) {
	for _, name := range []string{"website", "page"} {
		bytes := schemaDocument(t, name)
		if !strings.Contains(string(bytes), `"format": "date-time"`) {
			t.Fatalf("%s schema does not include date-time fields", name)
		}
	}
}

func TestDataSchemasExplicitlyAllowCustomProperties(t *testing.T) {
	for _, name := range []string{"website", "theme", "page", "provider"} {
		bytes := schemaDocument(t, name)
		if !strings.Contains(string(bytes), `"additionalProperties": true`) {
			t.Fatalf("%s schema does not explicitly allow custom properties", name)
		}
	}
}

func TestThemeSchemaMatchesThemeJSONContract(t *testing.T) {
	provider, ok := SchemaRegistry.Lookup("theme")
	if !ok {
		t.Fatal("theme schema is not registered")
	}
	if provider.JSONSchemaDescription() != "A theme source configuration file stored as theme.json." {
		t.Fatalf("theme schema description = %q", provider.JSONSchemaDescription())
	}
	content := schemaDocument(t, "theme")
	if !strings.Contains(string(content), `"data"`) || strings.Contains(string(content), `"template"`) {
		t.Fatalf("theme schema has an invalid data contract: %s", content)
	}
}

func TestProviderSchemaMatchesHostingProviderContract(t *testing.T) {
	content := schemaDocument(t, "provider")
	var document map[string]any
	if err := json.Unmarshal(content, &document); err != nil {
		t.Fatal(err)
	}
	provider := document["$defs"].(map[string]any)["github.com/herdworks/htup.HostingProvider"].(map[string]any)
	if got := provider["required"]; !slices.Equal(got.([]any), []any{"kind", "endpoint"}) {
		t.Fatalf("provider required = %#v", got)
	}
	properties := provider["properties"].(map[string]any)
	if got := properties["kind"].(map[string]any)["description"]; !strings.Contains(got.(string), "'git'") || !strings.Contains(got.(string), "'s3'") {
		t.Fatalf("provider kind description = %#v, want documented built-in kinds", got)
	}
	if properties["secrets"].(map[string]any)["type"] != "array" {
		t.Fatalf("provider secrets = %#v, want array", properties["secrets"])
	}
	filter := document["$defs"].(map[string]any)["github.com/herdworks/htup.FilterRules"].(map[string]any)
	for _, name := range []string{"include", "exclude"} {
		property := properties[name].(map[string]any)
		if property["$ref"] == nil {
			t.Fatalf("provider %s = %#v, want a filter-rules reference", name, property)
		}
		if filter["type"] != "object" {
			t.Fatalf("provider %s = %#v, want object", name, property)
		}
		if _, ok := filter["properties"].(map[string]any)["paths"]; !ok {
			t.Fatalf("provider %s has no paths", name)
		}
	}
}

func TestSchemaRegistryDoesNotContainUnknownSchema(t *testing.T) {
	if _, ok := SchemaRegistry.Lookup("unknown"); ok {
		t.Fatal("unknown schema found")
	}
}
