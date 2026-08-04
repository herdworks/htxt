// Package jsonschema generates JSON Schema documents from Go struct types.
package jsonschema

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

const draft202012 = "https://json-schema.org/draft/2020-12/schema"

type generator struct {
	definitions map[string]any
	visited     map[reflect.Type]bool
}

type metadata struct {
	title       string
	description string
}

type tagOptions struct {
	required    bool
	format      string
	typeName    string
	description string
	defaultText string
	pattern     string
}

// Generate returns a pretty-printed JSON Schema Draft 2020-12 document for a
// struct value or pointer to a struct. It returns an error when the reachable
// type graph contains a Go type that cannot be represented by this generator.
func Generate(value any) (string, error) {
	typeOf := reflect.TypeOf(value)
	if typeOf == nil {
		return "", fmt.Errorf("jsonschema: root type is nil")
	}
	for typeOf.Kind() == reflect.Pointer {
		typeOf = typeOf.Elem()
	}
	if typeOf.Kind() != reflect.Struct {
		return "", fmt.Errorf("jsonschema: root type %s is not a struct", typeOf)
	}
	if typeOf.Name() == "" {
		return "", fmt.Errorf("jsonschema: anonymous root structs are unsupported")
	}

	generator := generator{
		definitions: map[string]any{},
		visited:     map[reflect.Type]bool{},
	}
	root, err := generator.schemaFor(typeOf)
	if err != nil {
		return "", err
	}
	document := map[string]any{
		"$schema": draft202012,
		"$ref":    root["$ref"],
		"$defs":   generator.definitions,
	}
	if metadata, ok := metadataFor(typeOf); ok {
		if title := metadata.title; title != "" {
			document["title"] = title
		} else {
			document["title"] = typeOf.Name()
		}
		if description := metadata.description; description != "" {
			document["description"] = description
		}
	} else {
		document["title"] = typeOf.Name()
	}
	bytes, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return "", fmt.Errorf("jsonschema: marshal schema: %w", err)
	}
	return string(bytes) + "\n", nil
}

func (generator *generator) schemaFor(typeOf reflect.Type) (map[string]any, error) {
	for typeOf.Kind() == reflect.Pointer {
		typeOf = typeOf.Elem()
	}

	switch typeOf.Kind() {
	case reflect.Bool:
		return map[string]any{"type": "boolean"}, nil
	case reflect.String:
		return map[string]any{"type": "string"}, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer"}, nil
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}, nil
	case reflect.Interface:
		return map[string]any{}, nil
	case reflect.Array, reflect.Slice:
		items, err := generator.schemaFor(typeOf.Elem())
		if err != nil {
			return nil, err
		}
		return map[string]any{"type": "array", "items": items}, nil
	case reflect.Map:
		if typeOf.Key().Kind() != reflect.String {
			return nil, fmt.Errorf("jsonschema: map %s has non-string keys", typeOf)
		}
		values, err := generator.schemaFor(typeOf.Elem())
		if err != nil {
			return nil, err
		}
		return map[string]any{"type": "object", "additionalProperties": values}, nil
	case reflect.Struct:
		return generator.schemaForStruct(typeOf)
	case reflect.Chan, reflect.Complex64, reflect.Complex128, reflect.Func, reflect.UnsafePointer:
		return nil, fmt.Errorf("jsonschema: unsupported type %s", typeOf)
	default:
		return nil, fmt.Errorf("jsonschema: unsupported type %s", typeOf)
	}
}

func (generator *generator) schemaForStruct(typeOf reflect.Type) (map[string]any, error) {
	if typeOf.Name() == "" {
		return nil, fmt.Errorf("jsonschema: anonymous struct %s is unsupported", typeOf)
	}
	name := definitionName(typeOf)
	if generator.visited[typeOf] {
		return ref(name), nil
	}
	generator.visited[typeOf] = true

	object := map[string]any{
		"type":                 "object",
		"properties":           map[string]any{},
		"additionalProperties": true,
	}
	metadata, provided := metadataFor(typeOf)
	if metadata.title != "" {
		object["title"] = metadata.title
	} else {
		object["title"] = typeOf.Name()
	}
	if provided && metadata.description != "" {
		object["description"] = metadata.description
	}
	generator.definitions[name] = object
	properties := object["properties"].(map[string]any)
	required := []string{}

	for index := 0; index < typeOf.NumField(); index++ {
		field := typeOf.Field(index)
		if !field.IsExported() {
			continue
		}
		if field.Anonymous {
			return nil, fmt.Errorf("jsonschema: anonymous field %s.%s is unsupported", typeOf, field.Name)
		}
		name, included := jsonFieldName(field)
		if !included || field.Tag.Get("jsonschema") == "-" {
			continue
		}

		options, err := parseTag(field.Tag.Get("jsonschema"))
		if err != nil {
			return nil, fmt.Errorf("jsonschema: field %s.%s: %w", typeOf, field.Name, err)
		}
		property := map[string]any{}
		if options.typeName != "" {
			property["type"] = options.typeName
		} else if property, err = generator.schemaFor(field.Type); err != nil {
			return nil, fmt.Errorf("jsonschema: field %s.%s: %w", typeOf, field.Name, err)
		}
		applyOptions(property, options)
		properties[name] = property
		if options.required {
			required = append(required, name)
		}
	}
	if len(required) > 0 {
		object["required"] = required
	}
	return ref(name), nil
}

func metadataFor(typeOf reflect.Type) (metadata, bool) {
	provider, ok := reflect.New(typeOf).Interface().(SchemaProvider)
	if !ok {
		return metadata{}, false
	}
	return metadata{
		title:       provider.JSONSchemaTitle(),
		description: provider.JSONSchemaDescription(),
	}, true
}

func definitionName(typeOf reflect.Type) string {
	return typeOf.PkgPath() + "." + typeOf.Name()
}

func ref(name string) map[string]any {
	token := strings.NewReplacer("~", "~0", "/", "~1").Replace(name)
	return map[string]any{"$ref": "#/$defs/" + token}
}

func jsonFieldName(field reflect.StructField) (string, bool) {
	tag := field.Tag.Get("json")
	if tag == "-" {
		return "", false
	}
	name, _, _ := strings.Cut(tag, ",")
	if name == "" {
		name = field.Name
	}
	return name, true
}

func parseTag(tag string) (tagOptions, error) {
	if tag == "" {
		return tagOptions{}, nil
	}
	options := tagOptions{}
	for _, option := range strings.Split(tag, ",") {
		switch {
		case option == "required":
			options.required = true
		case strings.HasPrefix(option, "format="):
			format := strings.TrimPrefix(option, "format=")
			if format == "" {
				return tagOptions{}, fmt.Errorf("format cannot be empty")
			}
			options.format = format
		case strings.HasPrefix(option, "type="):
			typeName := strings.TrimPrefix(option, "type=")
			switch typeName {
			case "array", "boolean", "integer", "null", "number", "object", "string":
				options.typeName = typeName
			default:
				return tagOptions{}, fmt.Errorf("unsupported JSON Schema type %q", typeName)
			}
		case strings.HasPrefix(option, "description="):
			description := strings.TrimPrefix(option, "description=")
			if description == "" {
				return tagOptions{}, fmt.Errorf("description cannot be empty")
			}
			options.description = description
		case strings.HasPrefix(option, "default="):
			defaultText := strings.TrimPrefix(option, "default=")
			if defaultText == "" {
				return tagOptions{}, fmt.Errorf("default cannot be empty")
			}
			options.defaultText = defaultText
		case strings.HasPrefix(option, "pattern="):
			pattern := strings.TrimPrefix(option, "pattern=")
			if pattern == "" {
				return tagOptions{}, fmt.Errorf("pattern cannot be empty")
			}
			options.pattern = pattern
		default:
			return tagOptions{}, fmt.Errorf("unsupported tag option %q", option)
		}
	}
	return options, nil
}

func applyOptions(schema map[string]any, options tagOptions) {
	if options.format != "" {
		schema["format"] = options.format
	}
	if options.description != "" {
		schema["description"] = options.description
	}
	if options.defaultText != "" {
		schema["default"] = options.defaultText
	}
	if options.pattern != "" {
		schema["pattern"] = options.pattern
	}
}
