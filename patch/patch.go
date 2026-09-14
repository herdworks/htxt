package patch

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/herdworks/httt"
	"gopkg.in/yaml.v3"
)

type Patch struct {
	Op       Operation `json:"op" yaml:"op"`
	Path     string    `json:"path" yaml:"path"`
	ValueRef string    `json:"value_ref" yaml:"value_ref"`
}

type Patches []Patch

type Operation string

const (
	OperationAdd     Operation = "add"
	OperationReplace Operation = "replace"
	OperationAppend  Operation = "append"
	OperationSet     Operation = "set"

	PagePathPrefix = "page."
)

type Values map[string]any

func (patch Patch) Validate() error {
	switch patch.Op {
	case OperationAdd, OperationReplace, OperationAppend, OperationSet:
	default:
		return fmt.Errorf("patch op %q is not supported", patch.Op)
	}
	if _, err := DataPath(patch.Path); err != nil {
		return err
	}
	if strings.TrimSpace(patch.ValueRef) == "" {
		return fmt.Errorf("patch value_ref is required")
	}
	return nil
}

func validatePatches(patches Patches) error {
	for i, patch := range patches {
		if err := patch.Validate(); err != nil {
			return fmt.Errorf("patch %d invalid: %w", i+1, err)
		}
	}
	return nil
}

func (patches Patches) Validate(template httt.TemplateData) error {
	base := httt.TemplateData{}
	if template != nil {
		base = template.Clone()
	}
	for i, patch := range patches {
		if err := patch.Validate(); err != nil {
			return fmt.Errorf("patch %d invalid: %w", i+1, err)
		}
		if ProtectedPath(patch.Path) {
			return fmt.Errorf("patch %d invalid: path %q is protected", i+1, patch.Path)
		}
		value := validationValue(base, patch)
		values := Values{patch.ValueRef: value}
		if err := Apply(base, Patches{patch}, values); err != nil {
			return fmt.Errorf("patch %d invalid against spec.template: %w", i+1, err)
		}
	}
	return nil
}

func (patches Patches) ValidateReferences(refs map[string]any) error {
	for i, patch := range patches {
		if _, ok := refs[patch.ValueRef]; !ok {
			return fmt.Errorf("patch %d invalid: value_ref %q does not match a prompt", i+1, patch.ValueRef)
		}
	}
	return nil
}

func Apply(data httt.TemplateData, patches Patches, values Values) error {
	if data == nil {
		return fmt.Errorf("patch apply requires template data")
	}
	if err := validatePatches(patches); err != nil {
		return err
	}
	for i, patch := range patches {
		path, err := DataPath(patch.Path)
		if err != nil {
			return fmt.Errorf("patch %d invalid: %w", i+1, err)
		}
		value, ok := values[patch.ValueRef]
		if !ok {
			return fmt.Errorf("patch %d references unknown value_ref %q", i+1, patch.ValueRef)
		}
		switch patch.Op {
		case OperationAdd:
			if err := data.Add(path, value); err != nil {
				return fmt.Errorf("patch %d add %s: %w", i+1, patch.Path, err)
			}
		case OperationReplace:
			if err := data.Replace(path, value); err != nil {
				return fmt.Errorf("patch %d %s %s: %w", i+1, patch.Op, patch.Path, err)
			}
		case OperationAppend:
			if err := data.Append(path, value); err != nil {
				return fmt.Errorf("patch %d append %s: %w", i+1, patch.Path, err)
			}
		case OperationSet:
			if err := data.Set(path, value); err != nil {
				return fmt.Errorf("patch %d set %s: %w", i+1, patch.Path, err)
			}
		}
	}
	return nil
}

func ProtectedPath(path string) bool {
	protected := []string{
		"page.uri",
		"page.path",
		"page.assets",
		"page.canonical_url",
		"page.feed",
	}
	for _, prefix := range protected {
		if path == prefix || strings.HasPrefix(path, prefix+".") {
			return true
		}
	}
	return false
}

func validationValue(data httt.TemplateData, patch Patch) any {
	dataPath, err := DataPath(patch.Path)
	if err == nil && patch.Op == OperationAppend {
		if value := appendValidationValue(data.Get(dataPath)); value != nil {
			return value
		}
	}
	return ""
}

func appendValidationValue(existing any) any {
	switch existing.(type) {
	case []string:
		return ""
	case []bool:
		return false
	case []int:
		return int(0)
	case []int64:
		return int64(0)
	case []float64:
		return float64(0)
	case []float32:
		return float32(0)
	}
	return nil
}

func DataPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if !strings.HasPrefix(path, PagePathPrefix) {
		return "", fmt.Errorf("patch path %q must begin with %q", path, PagePathPrefix)
	}
	dataPath := strings.TrimPrefix(path, PagePathPrefix)
	if dataPath == "" {
		return "", fmt.Errorf("patch path %q must identify a page property", path)
	}
	for _, segment := range strings.Split(dataPath, ".") {
		if segment == "" {
			return "", fmt.Errorf("patch path %q contains an empty segment", path)
		}
		if strings.HasSuffix(segment, "()") {
			return "", fmt.Errorf("patch path %q cannot use function-style accessors", path)
		}
		if segment == "-" {
			return "", fmt.Errorf("patch path %q cannot use '-' segments; use op %q with the array path", path, OperationAppend)
		}
	}
	return dataPath, nil
}

func (patch *Patch) UnmarshalJSON(data []byte) error {
	type rawPatch Patch
	var raw struct {
		rawPatch
		Value string `json:"value"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*patch = Patch(raw.rawPatch)
	if patch.ValueRef == "" {
		patch.ValueRef = raw.Value
	}
	return nil
}

func (patch *Patch) JSON() (string, error) {
	bytes, err := json.Marshal(patch)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func (patches *Patches) JSON() (string, error) {
	bytes, err := json.Marshal(patches)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func (patch *Patch) UnmarshalYAML(value *yaml.Node) error {
	type rawPatch Patch
	var raw struct {
		rawPatch `yaml:",inline"`
		Value    string `yaml:"value"`
	}
	if err := value.Decode(&raw); err != nil {
		return err
	}
	*patch = Patch(raw.rawPatch)
	if patch.ValueRef == "" {
		patch.ValueRef = raw.Value
	}
	return nil
}
