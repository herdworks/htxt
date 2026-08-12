package htxt

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/herdworks/httt"
)

func writeDataFile(t *testing.T, root, rel, content string) {
	t.Helper()
	abs := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("mkdir %q: %v", abs, err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatalf("write %q: %v", abs, err)
	}
}

func TestLoadStaticDataFlatNamespaces(t *testing.T) {
	root := t.TempDir()
	writeDataFile(t, root, "site.yaml", "title: Hello\n")
	writeDataFile(t, root, "nav.json", `{"items":[{"label":"Home"}]}`)

	got, err := loadStaticData(nil, root, nil, false, io.Discard)
	if err != nil {
		t.Fatalf("loadStaticData: %v", err)
	}

	if v := got.Get("site.title"); v != "Hello" {
		t.Errorf("site.title = %v, want Hello", v)
	}
	if v := got.Get("nav.items.1.label"); v != "Home" {
		t.Errorf("nav.items.1.label = %v, want Home", v)
	}
}

func TestLoadStaticDataCSV(t *testing.T) {
	root := t.TempDir()
	writeDataFile(t, root, "addresses.csv", "Name,Address\nAda,123 Example St\nBob,456 Sample Ave\n")

	got, err := loadStaticData(nil, root, nil, false, io.Discard)
	if err != nil {
		t.Fatalf("loadStaticData: %v", err)
	}

	if v := got.Len("addresses.rows"); v != 2 {
		t.Errorf("addresses.rows length = %v, want 2", v)
	}
	if v := got.Get("addresses.rows.1.Name"); v != "Ada" {
		t.Errorf("addresses.rows.1.Name = %v, want Ada", v)
	}
	if v := got.Get("addresses.rows.2.Address"); v != "456 Sample Ave" {
		t.Errorf("addresses.rows.2.Address = %v, want 456 Sample Ave", v)
	}
}

func TestLoadStaticDataSubdirsNest(t *testing.T) {
	root := t.TempDir()
	writeDataFile(t, root, "social/twitter.yaml", "handle: htmarkdown\n")
	writeDataFile(t, root, "social/opengraph.yaml", "type: website\n")

	got, err := loadStaticData(nil, root, nil, false, io.Discard)
	if err != nil {
		t.Fatalf("loadStaticData: %v", err)
	}

	if v := got.Get("social.twitter.handle"); v != "htmarkdown" {
		t.Errorf("social.twitter.handle = %v, want htmarkdown", v)
	}
	if v := got.Get("social.opengraph.type"); v != "website" {
		t.Errorf("social.opengraph.type = %v, want website", v)
	}
}

func TestLoadStaticDataRejectsInvalidFilenames(t *testing.T) {
	cases := []string{
		"foo.bar.yaml", // intermediate dot
		"has space.yaml",
		"-leading.yaml",
		"mid-hyphen.yaml",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeDataFile(t, root, name, "x: 1\n")
			_, err := loadStaticData(nil, root, nil, false, io.Discard)
			if err == nil {
				t.Fatalf("loadStaticData(%q): want error, got nil", name)
			}
			if !strings.Contains(err.Error(), "invalid namespace segment") {
				t.Fatalf("loadStaticData(%q): error = %v, want substring 'invalid namespace segment'", name, err)
			}
		})
	}
}

func TestLoadStaticDataCrossFormatCollisionErrors(t *testing.T) {
	root := t.TempDir()
	writeDataFile(t, root, "foo.yaml", "x: 1\n")
	writeDataFile(t, root, "foo.json", `{"y":2}`)

	_, err := loadStaticData(nil, root, nil, false, io.Discard)
	if err == nil {
		t.Fatal("loadStaticData: want collision error, got nil")
	}
	for _, want := range []string{`"foo"`, "foo.yaml", "foo.json"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("loadStaticData error = %v; want substring %q", err, want)
		}
	}
}

func TestLoadStaticDataCSVCrossFormatCollisionErrors(t *testing.T) {
	root := t.TempDir()
	writeDataFile(t, root, "foo.yaml", "x: 1\n")
	writeDataFile(t, root, "foo.csv", "Name\nAda\n")

	_, err := loadStaticData(nil, root, nil, false, io.Discard)
	if err == nil {
		t.Fatal("loadStaticData: want collision error, got nil")
	}
	for _, want := range []string{`"foo"`, "foo.yaml", "foo.csv"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("loadStaticData error = %v; want substring %q", err, want)
		}
	}
}

func TestLoadStaticDataDirVsFileCollisionErrors(t *testing.T) {
	root := t.TempDir()
	writeDataFile(t, root, "foo.yaml", "x: 1\n")
	if err := os.MkdirAll(filepath.Join(root, "foo"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	_, err := loadStaticData(nil, root, nil, false, io.Discard)
	if err == nil {
		t.Fatal("loadStaticData: want dir-vs-file collision error, got nil")
	}
	if !strings.Contains(err.Error(), "namespace") {
		t.Fatalf("loadStaticData error = %v; want namespace-claim error", err)
	}
}

func TestLoadStaticDataDirVsFileCollisionDetectedFirst(t *testing.T) {
	// data/foo.yaml alongside data/foo/bar.yaml: the directory `foo/` and
	// the file `foo.yaml` both claim the namespace `foo`, which the scanner
	// catches as a dir-vs-file collision before any nested write happens.
	root := t.TempDir()
	writeDataFile(t, root, "foo.yaml", "x: 1\n")
	writeDataFile(t, root, "foo/bar.yaml", "y: 2\n")

	_, err := loadStaticData(nil, root, nil, false, io.Discard)
	if err == nil {
		t.Fatal("loadStaticData: want dir-vs-file collision error, got nil")
	}
}

func TestNamespaceSegmentsFromRelPath(t *testing.T) {
	cases := []struct {
		rel       string
		want      []string
		wantError bool
	}{
		{"site.yaml", []string{"site"}, false},
		{"addresses.csv", []string{"addresses"}, false},
		{"social/twitter.yaml", []string{"social", "twitter"}, false},
		{"a/b/c/d.json", []string{"a", "b", "c", "d"}, false},
		{"2digits.yaml", []string{"2digits"}, false},
		{"foo.bar.yaml", nil, true},
		{"has space.yaml", nil, true},
		{"-leading.yaml", nil, true},
		{"mid-hyphen.yaml", nil, true},
		{"", nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.rel, func(t *testing.T) {
			got, err := namespaceSegmentsFromRelPath(tc.rel)
			if tc.wantError {
				if err == nil {
					t.Fatalf("want error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("segments = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("segments = %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestGenerateComputedDataDiscoversAndExecutes(t *testing.T) {
	website, build := testWebsite(t)

	// Place the static and computed files under the website's data dir.
	dataDir := website.DataDirPath()
	writeDataFile(t, dataDir, "site.yaml", "title: Hello\n")
	writeDataFile(t, dataDir, "feed.js", `
export default function() {
	return { title: site.title + "!" };
}
`)

	// Run static loader first so GlobalData reflects the static layout.
	static, err := loadStaticData(website, dataDir, nil, false, io.Discard)
	if err != nil {
		t.Fatalf("loadStaticData: %v", err)
	}
	website.GlobalData = static

	// Provide a minimal site TemplateData so the seed has site.title.
	siteData := httt.TemplateData{"title": "Hello"}
	website.TemplateData = &siteData

	if err := generateComputedData(build, false, io.Discard); err != nil {
		t.Fatalf("generateComputedData: %v", err)
	}

	if got := website.GlobalData.Get("feed.title"); got != "Hello!" {
		t.Errorf("feed.title = %v, want Hello!", got)
	}
}

func TestBuildContextExposesAllNamespaces(t *testing.T) {
	// Context() is the unified view that page templates and computed
	// scripts see; hyperctl cms data ls/inspect rely on it. Wire up a
	// build with each namespace populated and verify all six keys are
	// reachable, with values flowing through unchanged.
	website, build := testWebsite(t)
	build.ID = "test-build"

	siteData := httt.TemplateData{"title": "My Site"}
	website.TemplateData = &siteData
	website.GlobalData = httt.TemplateData{
		"social": httt.TemplateData{
			"twitter": httt.TemplateData{"handle": "htmarkdown"},
		},
	}
	website.Theme.TemplateData = httt.TemplateData{"name": "default"}
	build.Environment.TemplateData = httt.TemplateData{"HOST": "ci"}

	ctx := build.Context()
	for _, key := range []string{"ht", "build", "env", "site", "data", "theme"} {
		if _, ok := ctx[key]; !ok {
			t.Errorf("Context() missing %q key", key)
		}
	}
	if got := ctx.Get("ht.version"); got != LibVersion {
		t.Errorf("ht.version = %v, want %v", got, LibVersion)
	}
	if got := ctx.Get("build.id"); got != "test-build" {
		t.Errorf("build.id = %v, want test-build", got)
	}
	if got := ctx.Get("env.HOST"); got != "ci" {
		t.Errorf("env.HOST = %v, want ci", got)
	}
	if got := ctx.Get("site.title"); got != "My Site" {
		t.Errorf("site.title = %v, want My Site", got)
	}
	if got := ctx.Get("data.social.twitter.handle"); got != "htmarkdown" {
		t.Errorf("data.social.twitter.handle = %v, want htmarkdown", got)
	}
	if got := ctx.Get("theme.name"); got != "default" {
		t.Errorf("theme.name = %v, want default", got)
	}
}
