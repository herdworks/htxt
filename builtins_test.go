package htxt

import (
	"io/fs"
	"strings"
	"testing"

	httheme "github.com/herdworks/httt/theme"
)

// TestBuiltinsOverride confirms that importing this package swapped the
// theme BuiltinsFS in via init(), and that the override carries
// htxt's opinionated content.
func TestBuiltinsOverride(t *testing.T) {
	data, err := fs.ReadFile(httheme.BuiltinsFS, "builtins/fragments/builtin/head_start.html")
	if err != nil {
		t.Fatalf("theme.BuiltinsFS does not have head_start: %v", err)
	}
	if !strings.Contains(string(data), `"hypertexting"`) {
		t.Errorf("expected htxt override content (with generator default \"hypertexting\"), got:\n%s", data)
	}
}

func TestBuiltinsIncludeStatusLayout(t *testing.T) {
	data, err := fs.ReadFile(httheme.BuiltinsFS, "builtins/layouts/builtin/status.html")
	if err != nil {
		t.Fatalf("status layout is not embedded: %v", err)
	}
	if !strings.Contains(string(data), "site.config.status_redirect") {
		t.Fatalf("status layout does not use status redirect:\n%s", data)
	}
}
