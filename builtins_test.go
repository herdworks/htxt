package hypertexting

import (
	"io/fs"
	"strings"
	"testing"

	hypertemplates "github.com/herdworks/hypertemplates-go"
)

// TestBuiltinsOverride confirms that importing this package swapped the
// hypertemplates BuiltinsFS in via init(), and that the override carries
// hypertexting-go's opinionated content.
func TestBuiltinsOverride(t *testing.T) {
	data, err := fs.ReadFile(hypertemplates.BuiltinsFS, "builtins/fragments/builtin/head_start.html")
	if err != nil {
		t.Fatalf("hypertemplates.BuiltinsFS does not have head_start: %v", err)
	}
	if !strings.Contains(string(data), `"hypertexting"`) {
		t.Errorf("expected hypertexting-go override content (with generator default \"hypertexting\"), got:\n%s", data)
	}
}
