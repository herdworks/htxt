package prompt

import (
	"reflect"
	"strings"
	"testing"
)

func TestPromptsValidate(t *testing.T) {
	prompts := Prompts{
		{ID: "title", Type: TypeText, Default: "Hello"},
		{ID: "rating", Type: TypeNumber, Default: 5},
		{ID: "featured", Type: TypeBoolean, Default: true},
		{ID: "category", Type: TypeDropDown, Default: "essay", Options: Options{
			{Label: "Essay", Value: "essay"},
			{Label: "Link", Value: "link"},
		}},
		{ID: "priority", Type: TypeDropDown, Default: 1, Options: Options{
			{Label: "Low", Value: float64(1)},
			{Label: "High", Value: float64(2)},
		}},
	}

	if err := prompts.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	prompts = append(prompts, Prompt{Key: "title"})
	if err := prompts.Validate(); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("Validate() error = %v, want duplicate prompt error", err)
	}

	prompts = Prompts{{ID: "bad", Type: TypeDropDown, Options: Options{
		{Label: "Object", Value: map[string]any{"kind": "object"}},
	}}}
	if err := prompts.Validate(); err == nil || !strings.Contains(err.Error(), "JSON-like scalar") {
		t.Fatalf("Validate() error = %v, want scalar option error", err)
	}

	prompts = Prompts{{ID: "bad-default", Type: TypeDropDown, Default: []any{"list"}, Options: Options{
		{Label: "Essay", Value: "essay"},
	}}}
	if err := prompts.Validate(); err == nil || !strings.Contains(err.Error(), "want drop-down") {
		t.Fatalf("Validate() error = %v, want scalar default error", err)
	}
}

func TestPromptsMap(t *testing.T) {
	prompts := Prompts{
		{ID: "title", Type: TypeText},
		{Key: "featured", Type: TypeBoolean},
	}

	refs, err := prompts.Map()
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}
	if refs["title"].(Prompt).ID != "title" {
		t.Fatalf("refs[title] = %#v, want title prompt", refs["title"])
	}
	if refs["featured"].(Prompt).Key != "featured" {
		t.Fatalf("refs[featured] = %#v, want featured prompt", refs["featured"])
	}

	prompts = append(prompts, Prompt{Name: "title"})
	if _, err := prompts.Map(); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("Map() error = %v, want duplicate prompt error", err)
	}
}

func TestPromptsDefaults(t *testing.T) {
	prompts := Prompts{
		{ID: "title", Type: TypeText, Default: ""},
		{ID: "summary", Type: TypeText, Default: "Default summary"},
		{ID: "rating", Type: TypeNumber, Default: 0},
		{ID: "featured", Type: TypeBoolean, Default: false},
		{ID: "category", Type: TypeDropDown, Options: Options{{Label: "Essay", Value: "essay"}}},
	}

	defaults := prompts.Defaults()
	want := map[string]any{
		"title":    "",
		"summary":  "Default summary",
		"rating":   0,
		"featured": false,
	}
	if !reflect.DeepEqual(defaults, want) {
		t.Fatalf("Defaults() = %#v, want %#v", defaults, want)
	}
}

func TestPromptsValidateUserData(t *testing.T) {
	prompts := Prompts{
		{ID: "title", Type: TypeText, Required: true},
		{ID: "rating", Type: TypeNumber},
		{ID: "featured", Type: TypeBoolean},
		{ID: "category", Type: TypeDropDown, Options: Options{{Label: "Essay", Value: "essay"}}},
	}

	if err := prompts.ValidateUserData(map[string]any{
		"title":    "Runtime Title",
		"rating":   5,
		"featured": true,
		"category": "essay",
	}); err != nil {
		t.Fatalf("ValidateUserData() error = %v", err)
	}

	tests := []struct {
		name     string
		userData map[string]any
		want     string
	}{
		{name: "unknown", userData: map[string]any{"title": "Runtime Title", "unknown": true}, want: "does not match a prompt"},
		{name: "wrong type", userData: map[string]any{"title": "Runtime Title", "featured": "true"}, want: "want boolean"},
		{name: "drop-down option", userData: map[string]any{"title": "Runtime Title", "category": "link"}, want: "does not match a drop-down option"},
		{name: "missing required", userData: map[string]any{"rating": 5}, want: "is required"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := prompts.ValidateUserData(tc.userData); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ValidateUserData() error = %v, want %q", err, tc.want)
			}
		})
	}
}
