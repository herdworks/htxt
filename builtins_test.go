package htxt

import (
	"strings"
	"testing"

	httheme "github.com/herdworks/httt/theme"
)

func htxtBuiltinTheme() *httheme.Theme {
	theme := &httheme.Theme{}
	hydrateThemeBuiltins(theme)
	return theme
}

func TestBuiltinThemeUsesHTXTPack(t *testing.T) {
	data, found, err := htxtBuiltinTheme().BuiltinResourceBytes("fragments/builtin/head_start")
	if err != nil {
		t.Fatalf("BuiltinResourceBytes returned error: %v", err)
	}
	if !found {
		t.Fatal("BuiltinResourceBytes did not find head_start")
	}
	if !strings.Contains(string(data), `"hypertexting"`) {
		t.Errorf("expected htxt override content (with generator default \"hypertexting\"), got:\n%s", data)
	}
	if !strings.Contains(string(data), `href='/css/chroma-palette.css?v=${ site.version, build.id }'`) {
		t.Errorf("expected builtin chroma palette link, got:\n%s", data)
	}
	if !strings.Contains(string(data), `type='${ site.config.feed_mimetype }'`) {
		t.Errorf("expected configured feed MIME type, got:\n%s", data)
	}
}

func TestBuiltinsIncludeStatusLayout(t *testing.T) {
	data, found, err := htxtBuiltinTheme().BuiltinResourceBytes("layouts/builtin/status")
	if err != nil {
		t.Fatalf("BuiltinResourceBytes returned error: %v", err)
	}
	if !found {
		t.Fatal("BuiltinResourceBytes did not find status layout")
	}
	if !strings.Contains(string(data), "site.config.status_redirect") {
		t.Fatalf("status layout does not use status redirect:\n%s", data)
	}
}

func TestBuiltinResourceBytesIncludesChromaPalette(t *testing.T) {
	data, found, err := htxtBuiltinTheme().BuiltinResourceBytes("static/css/chroma-palette.css")
	if err != nil {
		t.Fatalf("BuiltinResourceBytes returned error: %v", err)
	}
	if !found {
		t.Fatal("BuiltinResourceBytes did not find chroma palette")
	}
	if !strings.Contains(string(data), ".chroma") {
		t.Fatalf("chroma palette is missing chroma styles:\n%s", data)
	}
}
