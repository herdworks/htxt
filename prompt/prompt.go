package prompt

import (
	"encoding/json"
	"fmt"
	"strings"
)

type Type string

const (
	TypeText     Type = "text"
	TypeNumber   Type = "number"
	TypeBoolean  Type = "boolean"
	TypeDropDown Type = "drop-down"
)

type Prompt struct {
	ID          string  `json:"id,omitempty" yaml:"id,omitempty"`
	Key         string  `json:"key,omitempty" yaml:"key,omitempty"`
	Name        string  `json:"name,omitempty" yaml:"name,omitempty"`
	Type        Type    `json:"type,omitempty" yaml:"type,omitempty"`
	Label       string  `json:"label,omitempty" yaml:"label,omitempty"`
	Placeholder string  `json:"placeholder,omitempty" yaml:"placeholder,omitempty"`
	Help        string  `json:"help,omitempty" yaml:"help,omitempty"`
	Required    bool    `json:"required,omitempty" yaml:"required,omitempty"`
	Default     any     `json:"default,omitempty" yaml:"default,omitempty"`
	Options     Options `json:"options,omitempty" yaml:"options,omitempty"`
}

type Prompts []Prompt

type Option struct {
	Label string `json:"label" yaml:"label"`
	Value any    `json:"value" yaml:"value"`
}

type Options []Option

func (prompts Prompts) Validate() error {
	seen := map[string]struct{}{}
	for i, prompt := range prompts {
		ref, err := prompt.Ref()
		if err != nil {
			return fmt.Errorf("prompt %d invalid: %w", i+1, err)
		}
		if _, exists := seen[ref]; exists {
			return fmt.Errorf("prompt %d invalid: duplicate prompt id %q", i+1, ref)
		}
		seen[ref] = struct{}{}
		if err := prompt.Validate(); err != nil {
			return fmt.Errorf("prompt %q invalid: %w", ref, err)
		}
	}
	return nil
}

func (prompts Prompts) Map() (map[string]any, error) {
	refs := map[string]any{}
	for i, prompt := range prompts {
		ref, err := prompt.Ref()
		if err != nil {
			return nil, fmt.Errorf("prompt %d invalid: %w", i+1, err)
		}
		if _, exists := refs[ref]; exists {
			return nil, fmt.Errorf("prompt %d invalid: duplicate prompt id %q", i+1, ref)
		}
		refs[ref] = prompt
	}
	return refs, nil
}

func (prompts Prompts) Defaults() map[string]any {
	defaults := map[string]any{}
	for _, prompt := range prompts {
		if prompt.Default == nil {
			continue
		}
		ref, err := prompt.Ref()
		if err != nil {
			continue
		}
		defaults[ref] = prompt.Default
	}
	return defaults
}

func (prompts Prompts) ValidateUserData(userData map[string]any) error {
	refs := map[string]Prompt{}
	for i, prompt := range prompts {
		ref, err := prompt.Ref()
		if err != nil {
			return fmt.Errorf("prompt %d invalid: %w", i+1, err)
		}
		if _, exists := refs[ref]; exists {
			return fmt.Errorf("prompt %d invalid: duplicate prompt id %q", i+1, ref)
		}
		refs[ref] = prompt
	}
	for ref, value := range userData {
		prompt, ok := refs[ref]
		if !ok {
			return fmt.Errorf("user_data %q does not match a prompt", ref)
		}
		if err := prompt.ValidateUserValue(value); err != nil {
			return fmt.Errorf("user_data %q invalid: %w", ref, err)
		}
	}
	for ref, prompt := range refs {
		if !prompt.Required {
			continue
		}
		if value, ok := userData[ref]; !ok || value == nil {
			return fmt.Errorf("user_data %q is required", ref)
		}
	}
	return nil
}

func (prompts Prompts) RefSet() (map[string]struct{}, error) {
	refs := map[string]struct{}{}
	for i, prompt := range prompts {
		ref, err := prompt.Ref()
		if err != nil {
			return nil, fmt.Errorf("prompt %d invalid: %w", i+1, err)
		}
		if _, exists := refs[ref]; exists {
			return nil, fmt.Errorf("prompt %d invalid: duplicate prompt id %q", i+1, ref)
		}
		refs[ref] = struct{}{}
	}
	return refs, nil
}

func (prompt Prompt) Validate() error {
	if _, err := prompt.Ref(); err != nil {
		return err
	}
	promptType := prompt.PromptType()
	switch promptType {
	case TypeText, TypeNumber, TypeBoolean, TypeDropDown:
	default:
		return fmt.Errorf("prompt type %q is not supported", prompt.Type)
	}
	if prompt.Default != nil && !ValueMatchesType(prompt.Default, promptType) {
		return fmt.Errorf("default value has type %T, want %s", prompt.Default, promptType)
	}
	if promptType != TypeDropDown {
		return nil
	}
	if len(prompt.Options) == 0 {
		return fmt.Errorf("drop-down prompt requires options")
	}
	for i, option := range prompt.Options {
		if strings.TrimSpace(option.Label) == "" {
			return fmt.Errorf("drop-down option %d requires label", i+1)
		}
		if option.Value == nil {
			return fmt.Errorf("drop-down option %d requires value", i+1)
		}
		if !ValueIsJSONScalar(option.Value) {
			return fmt.Errorf("drop-down option %d value has type %T, want JSON-like scalar", i+1, option.Value)
		}
	}
	if prompt.Default != nil && !prompt.Options.ContainsValue(prompt.Default) {
		return fmt.Errorf("default value %#v does not match a drop-down option", prompt.Default)
	}
	return nil
}

func (prompt Prompt) ValidateUserValue(value any) error {
	promptType := prompt.PromptType()
	if !ValueMatchesType(value, promptType) {
		return fmt.Errorf("value has type %T, want %s", value, promptType)
	}
	if promptType == TypeDropDown && !prompt.Options.ContainsValue(value) {
		return fmt.Errorf("value %#v does not match a drop-down option", value)
	}
	return nil
}

func (prompt Prompt) Ref() (string, error) {
	refs := []struct {
		field string
		value string
	}{
		{field: "id", value: strings.TrimSpace(prompt.ID)},
		{field: "key", value: strings.TrimSpace(prompt.Key)},
		{field: "name", value: strings.TrimSpace(prompt.Name)},
	}
	ref := ""
	refField := ""
	for _, candidate := range refs {
		if candidate.value == "" {
			continue
		}
		if ref == "" {
			ref = candidate.value
			refField = candidate.field
			continue
		}
		if candidate.value != ref {
			return "", fmt.Errorf("prompt %s %q conflicts with %s %q", candidate.field, candidate.value, refField, ref)
		}
	}
	if ref == "" {
		return "", fmt.Errorf("prompt requires id, key, or name")
	}
	return ref, nil
}

func (prompt Prompt) PromptType() Type {
	if prompt.Type == "" {
		return TypeText
	}
	return prompt.Type
}

func (options Options) ContainsValue(value any) bool {
	for _, option := range options {
		if ValuesEqual(option.Value, value) {
			return true
		}
	}
	return false
}

func (prompts *Prompts) JSON() (string, error) {
	bytes, err := json.Marshal(prompts)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func (prompt *Prompt) JSON() (string, error) {
	bytes, err := json.Marshal(prompt)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func ValueMatchesType(value any, promptType Type) bool {
	switch promptType {
	case TypeText:
		_, ok := value.(string)
		return ok
	case TypeNumber:
		_, ok := NumericValue(value)
		return ok
	case TypeBoolean:
		_, ok := value.(bool)
		return ok
	case TypeDropDown:
		return ValueIsJSONScalar(value)
	default:
		return false
	}
}

func ValuesEqual(a any, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if av, ok := NumericValue(a); ok {
		if bv, ok := NumericValue(b); ok {
			return av == bv
		}
	}
	switch av := a.(type) {
	case string:
		bv, ok := b.(string)
		return ok && av == bv
	case bool:
		bv, ok := b.(bool)
		return ok && av == bv
	}
	return false
}

func NumericValue(value any) (float64, bool) {
	switch v := value.(type) {
	case int:
		return float64(v), true
	case int8:
		return float64(v), true
	case int16:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint8:
		return float64(v), true
	case uint16:
		return float64(v), true
	case uint32:
		return float64(v), true
	case uint64:
		return float64(v), true
	case float32:
		return float64(v), true
	case float64:
		return v, true
	case json.Number:
		parsed, err := v.Float64()
		return parsed, err == nil
	}
	return 0, false
}

func ValueIsJSONScalar(value any) bool {
	if _, ok := NumericValue(value); ok {
		return true
	}
	switch value.(type) {
	case string, bool:
		return true
	}
	return false
}
