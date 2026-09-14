package patch

import (
	"reflect"
	"strings"
	"testing"

	"github.com/herdworks/httt"
)

func TestApplyAddAndReplaceSetTemplateData(t *testing.T) {
	data := httt.TemplateData{"featured": false}
	values := Values{"title": "Hello", "featured": true}
	patches := Patches{
		{Op: OperationAdd, Path: "page.title", ValueRef: "title"},
		{Op: OperationReplace, Path: "page.featured", ValueRef: "featured"},
	}

	if err := Apply(data, patches, values); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if got := data.Get("title"); got != "Hello" {
		t.Fatalf("title = %v, want Hello", got)
	}
	if got := data.Get("featured"); got != true {
		t.Fatalf("featured = %v, want true", got)
	}
}

func TestApplyReplaceRejectsMissingTargets(t *testing.T) {
	data := httt.TemplateData{}
	patches := Patches{{Op: OperationReplace, Path: "page.title", ValueRef: "title"}}
	err := Apply(data, patches, Values{"title": "Hello"})

	if err == nil || !strings.Contains(err.Error(), "key does not exist") {
		t.Fatalf("Apply() error = %v, want missing replace target error", err)
	}
}

func TestApplySetCreatesAndOverwritesValues(t *testing.T) {
	data := httt.TemplateData{
		"title": "Original",
		"metadata": httt.TemplateData{
			"author": "Original Author",
		},
	}
	patches := Patches{
		{Op: OperationSet, Path: "page.title", ValueRef: "title"},
		{Op: OperationSet, Path: "page.summary", ValueRef: "summary"},
		{Op: OperationSet, Path: "page.metadata.author", ValueRef: "author"},
		{Op: OperationSet, Path: "page.metadata.category", ValueRef: "category"},
	}
	values := Values{
		"title":    "Updated",
		"summary":  "New summary",
		"author":   "Updated Author",
		"category": "News",
	}

	if err := Apply(data, patches, values); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if got := data.Get("title"); got != "Updated" {
		t.Fatalf("title = %v, want Updated", got)
	}
	if got := data.Get("summary"); got != "New summary" {
		t.Fatalf("summary = %v, want New summary", got)
	}
	if got := data.Get("metadata.author"); got != "Updated Author" {
		t.Fatalf("metadata.author = %v, want Updated Author", got)
	}
	if got := data.Get("metadata.category"); got != "News" {
		t.Fatalf("metadata.category = %v, want News", got)
	}
}

func TestApplySetRejectsScalarIntermediateWithoutMutation(t *testing.T) {
	data := httt.TemplateData{"metadata": "not a map"}
	patches := Patches{{Op: OperationSet, Path: "page.metadata.author", ValueRef: "author"}}
	err := Apply(data, patches, Values{"author": "Ada"})

	if err == nil || !strings.Contains(err.Error(), "not a map") {
		t.Fatalf("Apply() error = %v, want intermediate non-map error", err)
	}
	if got := data.Get("metadata"); got != "not a map" {
		t.Fatalf("metadata = %v, want original scalar", got)
	}
}

func TestApplyAppendCreatesAndAppendsArrays(t *testing.T) {
	data := httt.TemplateData{"tags": []string{"existing"}}
	values := Values{"one": "new", "two": "created"}
	patches := Patches{
		{Op: OperationAppend, Path: "page.tags", ValueRef: "one"},
		{Op: OperationAppend, Path: "page.categories", ValueRef: "two"},
	}

	if err := Apply(data, patches, values); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if got := data.Get("tags"); !reflect.DeepEqual(got, []string{"existing", "new"}) {
		t.Fatalf("tags = %#v, want existing and new", got)
	}
	if got := data.Get("categories"); !reflect.DeepEqual(got, []any{"created"}) {
		t.Fatalf("categories = %#v, want created array", got)
	}
}

func TestApplyAppendRejectsScalarTargets(t *testing.T) {
	data := httt.TemplateData{"tags": "not an array"}
	patches := Patches{{Op: OperationAppend, Path: "page.tags", ValueRef: "tag"}}
	err := Apply(data, patches, Values{"tag": "new"})

	if err == nil || !strings.Contains(err.Error(), "not a supported JSON-like array") {
		t.Fatalf("Apply() error = %v, want scalar target error", err)
	}
}

func TestValidateRejectsDashAppendSyntax(t *testing.T) {
	patch := Patch{Op: OperationAppend, Path: "page.tags.-", ValueRef: "tag"}
	err := patch.Validate()

	if err == nil || !strings.Contains(err.Error(), "use op \"append\"") {
		t.Fatalf("Validate() error = %v, want dash segment rejection", err)
	}
}

func TestPatchesValidateAgainstTemplate(t *testing.T) {
	patches := Patches{{Op: OperationAppend, Path: "page.tags", ValueRef: "tag"}}
	if err := patches.Validate(httt.TemplateData{"tags": []string{"base"}}); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	patches = Patches{{Op: OperationReplace, Path: "page.title", ValueRef: "title"}}
	if err := patches.Validate(nil); err == nil || !strings.Contains(err.Error(), "key does not exist") {
		t.Fatalf("Validate() error = %v, want missing replace target error", err)
	}

	patches = Patches{{Op: OperationAdd, Path: "page.title", ValueRef: "title"}}
	if err := patches.Validate(httt.TemplateData{"title": "Base Title"}); err == nil || !strings.Contains(err.Error(), "key already exists") {
		t.Fatalf("Validate() error = %v, want existing add target error", err)
	}

	patches = Patches{{Op: OperationSet, Path: "page.title", ValueRef: "title"}}
	if err := patches.Validate(nil); err != nil {
		t.Fatalf("Validate() error = %v, want set to create missing target", err)
	}
	if err := patches.Validate(httt.TemplateData{"title": "Base Title"}); err != nil {
		t.Fatalf("Validate() error = %v, want set to overwrite target", err)
	}

	patches = Patches{{Op: OperationAppend, Path: "page.tags", ValueRef: "tag"}}
	if err := patches.Validate(httt.TemplateData{"tags": "not an array"}); err == nil || !strings.Contains(err.Error(), "not a supported JSON-like array") {
		t.Fatalf("Validate() error = %v, want scalar append target error", err)
	}
}

func TestPatchesValidateRejectsProtectedPaths(t *testing.T) {
	patches := Patches{{Op: OperationAdd, Path: "page.uri", ValueRef: "uri"}}
	err := patches.Validate(nil)

	if err == nil || !strings.Contains(err.Error(), "protected") {
		t.Fatalf("Validate() error = %v, want protected path error", err)
	}
}

func TestPatchesValidateReferences(t *testing.T) {
	patches := Patches{{Op: OperationAdd, Path: "page.title", ValueRef: "title"}}
	prompts := map[string]any{"title": true}

	if err := patches.ValidateReferences(prompts); err != nil {
		t.Fatalf("ValidateReferences() error = %v", err)
	}

	prompts = map[string]any{}
	if err := patches.ValidateReferences(prompts); err == nil || !strings.Contains(err.Error(), "does not match a prompt") {
		t.Fatalf("ValidateReferences() error = %v, want missing prompt error", err)
	}
}

func TestPatchDecodesLegacyValueAsValueRef(t *testing.T) {
	var patch Patch
	if err := patch.UnmarshalJSON([]byte(`{"op":"replace","path":"page.title","value":"title"}`)); err != nil {
		t.Fatalf("UnmarshalJSON() error = %v", err)
	}
	if patch.ValueRef != "title" {
		t.Fatalf("ValueRef = %q, want title", patch.ValueRef)
	}
}
