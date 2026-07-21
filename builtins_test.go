package hypertexting

import (
	"io/fs"
	"strings"
	"testing"

	httheme "github.com/herdworks/hypertemplates-go/theme"
)

// TestBuiltinsOverride confirms that importing this package swapped the
// theme BuiltinsFS in via init(), and that the override carries
// hypertexting-go's opinionated content.
func TestBuiltinsOverride(t *testing.T) {
	data, err := fs.ReadFile(httheme.BuiltinsFS, "builtins/fragments/builtin/head_start.html")
	if err != nil {
		t.Fatalf("theme.BuiltinsFS does not have head_start: %v", err)
	}
	if !strings.Contains(string(data), `"hypertexting"`) {
		t.Errorf("expected hypertexting-go override content (with generator default \"hypertexting\"), got:\n%s", data)
	}
}
