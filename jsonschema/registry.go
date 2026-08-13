package jsonschema

import (
	"fmt"
	"sort"
)

// SchemaProvider identifies a first-class schema and supplies its human-facing
// metadata.
type SchemaProvider interface {
	JSONSchemaName() string
	JSONSchemaTitle() string
	JSONSchemaSummary() string
	JSONSchemaDescription() string
}

// Registry is a declarative collection of first-class JSON Schema providers.
type Registry []SchemaProvider

// Schema is the stable listing metadata for one registered schema.
type Schema struct {
	Name    string
	Summary string
}

type Schemas []Schema

// ColumnValues returns the values for a supported schema catalog column.
func (schemas Schemas) ColumnValues(field string) (values []string) {
	for _, schema := range schemas {
		switch field {
		case "Name":
			values = append(values, schema.Name)
		case "Description":
			values = append(values, schema.Summary)
		}
	}
	return values
}

// Validate reports empty and duplicate schema names.
func (registry Registry) Validate() error {
	names := map[string]struct{}{}
	for _, provider := range registry {
		if provider == nil {
			return fmt.Errorf("jsonschema: registry contains a nil provider")
		}
		name := provider.JSONSchemaName()
		if name == "" {
			return fmt.Errorf("jsonschema: registry contains a provider with an empty name")
		}
		if _, exists := names[name]; exists {
			return fmt.Errorf("jsonschema: registry contains duplicate schema name %q", name)
		}
		names[name] = struct{}{}
	}
	return nil
}

// Entries returns schema names and descriptions in registry order.
func (registry Registry) Entries() Schemas {
	entries := make(Schemas, 0, len(registry))
	for _, provider := range registry {
		if provider != nil {
			entries = append(entries, Schema{Name: provider.JSONSchemaName(), Summary: provider.JSONSchemaSummary()})
		}
	}
	return entries
}

// Names returns the registered schema names in sorted order.
func (registry Registry) Names() []string {
	names := make([]string, 0, len(registry))
	for _, provider := range registry {
		if provider != nil {
			names = append(names, provider.JSONSchemaName())
		}
	}
	sort.Strings(names)
	return names
}

// Lookup returns the provider registered with name.
func (registry Registry) Lookup(name string) (SchemaProvider, bool) {
	for _, provider := range registry {
		if provider != nil && provider.JSONSchemaName() == name {
			return provider, true
		}
	}
	return nil, false
}

// Describe generates the schema registered with name.
func (registry Registry) Describe(name string) ([]byte, error) {
	if err := registry.Validate(); err != nil {
		return nil, err
	}
	provider, ok := registry.Lookup(name)
	if !ok {
		return nil, fmt.Errorf("jsonschema: unknown schema %q", name)
	}
	document, err := Generate(provider)
	return []byte(document), err
}
