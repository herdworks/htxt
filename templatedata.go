package hypertexting

import (
	"slices"
	"strings"

	"github.com/herdworks/hypertemplates-go/data"
	"gopkg.in/yaml.v3"
)

// NamespacedData wraps a data.TemplateData with SSG-flavored helpers that
// understand the "namespace" property and CLI ColumnListable conventions.
// Convert at the call site:
//
//	hypertexting.NamespacedData(website.GlobalData).Namespace()
type NamespacedData data.TemplateData

// Namespace returns the conventional top-level namespace for the wrapped data.
// Resolution mirrors the package-level Namespace function.
func (n NamespacedData) Namespace() string {
	return Namespace(data.TemplateData(n))
}

// NamespaceData returns the contents of the namespace identified by the
// receiver. See the package-level NamespaceData function.
func (n NamespacedData) NamespaceData() any {
	return NamespaceData(data.TemplateData(n))
}

// ColumnValues implements cli.ColumnListable by returning the wrapped data's
// top-level keys when field is "namespace" or "namespaces". All other field
// names return nil.
func (n NamespacedData) ColumnValues(field string) []string {
	switch strings.ToLower(field) {
	case "namespace", "namespaces":
		return data.TemplateData(n).Keys("")
	}
	return nil
}

// Namespace returns the SSG-conventional top-level namespace for d.
//
// Resolution order:
//
//  1. If d has a top-level "namespace" string property, that value is used.
//  2. Otherwise, if d has exactly one top-level key, that key is used.
//  3. Otherwise, the literal "default" is returned.
//
// Namespace is a HyperTexting (SSG) concept and lives here rather than in
// the data package, which is intentionally domain-agnostic.
func Namespace(d data.TemplateData) string {
	if ns, ok := d["namespace"].(string); ok {
		return ns
	}
	keys := d.Keys("")
	if len(keys) == 1 {
		return keys[0]
	}
	return "default"
}

// NamespaceData returns the contents of the namespace identified by
// Namespace(d). When d already declares a "namespace" key, the entire map is
// returned as-is. Otherwise the value at Namespace(d) is returned.
func NamespaceData(d data.TemplateData) any {
	if slices.Contains(d.Keys(""), "namespace") {
		return d
	}
	return d[Namespace(d)]
}

// OrderedTemplateData wraps a TemplateData to control YAML marshal field
// ordering. Known Page-schema fields are emitted first, in declaration order,
// followed by remaining fields. Use it as a one-shot conversion at the
// marshal site:
//
//	yaml.Marshal(hypertexting.OrderedTemplateData(page.TemplateData))
type OrderedTemplateData data.TemplateData

// orderedFields is the canonical Page-schema field order. Fields not in this
// list are emitted after the ordered ones in map iteration order.
var orderedFields = []string{
	"created_at", "updated_at", "layout", "path", "slug",
	"byline", "contributors", "title", "description", "summary",
	"more", "tags", "unlisted", "redirect", "attachments",
}

// MarshalYAML implements yaml.Marshaler so that yaml.Marshal emits known
// Page-schema fields first in canonical order.
func (d OrderedTemplateData) MarshalYAML() (any, error) {
	output := &yaml.Node{Kind: yaml.MappingNode}
	for _, k := range orderedFields {
		v, ok := d[k]
		if !ok {
			continue
		}
		key := &yaml.Node{Kind: yaml.ScalarNode, Value: k}
		value := &yaml.Node{}
		if err := value.Encode(v); err != nil {
			return nil, err
		}
		output.Content = append(output.Content, key, value)
	}
	for k, v := range d {
		if slices.Contains(orderedFields, k) {
			continue
		}
		key := &yaml.Node{Kind: yaml.ScalarNode, Value: k}
		value := &yaml.Node{}
		if err := value.Encode(v); err != nil {
			return nil, err
		}
		output.Content = append(output.Content, key, value)
	}
	return output, nil
}
