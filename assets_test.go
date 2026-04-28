package hypertexting

import (
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/herdworks/hypertemplates-go"
	htplugins "github.com/herdworks/hypertemplates-go/plugins"
	"github.com/herdworks/hypertexting-go/href"
)

func writeAssetTestFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir asset test file dir: %v", err)
	}
	if err := os.WriteFile(path, []byte("asset"), 0644); err != nil {
		t.Fatalf("write asset test file: %v", err)
	}
}

func writeAssetTestScript(t *testing.T, path string, source string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir script test file dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(source), 0644); err != nil {
		t.Fatalf("write script test file: %v", err)
	}
}

func testWebsite(t *testing.T) (*Website, *HyperBuild) {
	t.Helper()
	root := t.TempDir()
	website := &Website{
		URI:     &href.Href{URL: url.URL{Path: filepath.Join(root, "site.yaml")}},
		BaseURL: "https://example.com/",
		Theme: hypertemplates.Theme{
			URI: &url.URL{Path: filepath.Join(root, "theme", "theme.json")},
		},
	}
	build := &HyperBuild{
		URI:     &url.URL{Path: filepath.Join(root, "builds", "test")},
		Website: website,
	}
	return website, build
}

func TestWebsiteAssetPaths(t *testing.T) {
	website, build := testWebsite(t)
	staticAsset := "css/site.css"
	themeAsset := "css/theme.css"
	pageAsset := "about/photo.jpg"
	for _, asset := range []string{
		filepath.Join(website.StaticDirPath(), "css", "site.css"),
		filepath.Join(website.Theme.StaticDirPath(), "css", "theme.css"),
		filepath.Join(website.ContentDirPath(), "about", "photo.jpg"),
	} {
		writeAssetTestFile(t, asset)
	}

	tests := []struct {
		name       string
		relPath    string
		publicPath string
		buildPath  string
	}{
		{
			name:       "website static",
			relPath:    staticAsset,
			publicPath: "/css/site.css",
			buildPath:  filepath.Join(build.URI.Path, "css", "site.css"),
		},
		{
			name:       "theme static",
			relPath:    themeAsset,
			publicPath: "/css/theme.css",
			buildPath:  filepath.Join(build.URI.Path, "css", "theme.css"),
		},
		{
			name:       "page asset",
			relPath:    pageAsset,
			publicPath: "/about/photo.jpg",
			buildPath:  filepath.Join(build.URI.Path, "about", "photo.jpg"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			publicPath, err := website.AssetPublicPath(tc.relPath)
			if err != nil {
				t.Fatalf("AssetPublicPath returned error: %v", err)
			}
			if publicPath != tc.publicPath {
				t.Fatalf("AssetPublicPath = %q, want %q", publicPath, tc.publicPath)
			}

			buildPath, err := website.AssetBuildPath(build, tc.relPath)
			if err != nil {
				t.Fatalf("AssetBuildPath returned error: %v", err)
			}
			if buildPath != tc.buildPath {
				t.Fatalf("AssetBuildPath = %q, want %q", buildPath, tc.buildPath)
			}
		})
	}
}

func TestWebsiteAssetPublicPaths(t *testing.T) {
	website, _ := testWebsite(t)
	website.Theme.Assets = []string{"shared.css", "theme.css"}
	website.Assets = []string{"shared.css", "site.css"}

	got, err := AssetPublicPaths(&website.Theme, website, nil)
	if err != nil {
		t.Fatalf("AssetPublicPaths returned error: %v", err)
	}
	want := []string{"/shared.css", "/theme.css", "/site.css"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("AssetPublicPaths = %#v, want %#v", got, want)
	}
}

func TestWebsiteAssetEntriesUseWinningAssetSource(t *testing.T) {
	website, _ := testWebsite(t)
	page := &Page{Assets: []string{"shared.css"}}
	website.Theme.Assets = []string{"shared.css"}
	website.Assets = []string{"shared.css"}
	writeAssetTestFile(t, filepath.Join(website.Theme.StaticDirPath(), "shared.css"))
	writeAssetTestFile(t, filepath.Join(website.StaticDirPath(), "shared.css"))
	writeAssetTestFile(t, filepath.Join(website.ContentDirPath(), "shared.css"))

	got, err := AssetEntries(&website.Theme, website, Pages{page})
	if err != nil {
		t.Fatalf("AssetEntries returned error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("AssetEntries len = %d, want 1", len(got))
	}
	want := Asset{
		Source:     AssetSourcePage,
		RelPath:    "shared.css",
		PublicPath: "/shared.css",
		SourcePath: filepath.Join(website.ContentDirPath(), "shared.css"),
	}
	if !reflect.DeepEqual(got[0], want) {
		t.Fatalf("AssetEntries[0] = %#v, want %#v", got[0], want)
	}
}

func TestWebsiteDiscoverAssetsReplacesExistingAssets(t *testing.T) {
	website, _ := testWebsite(t)
	website.Theme.Assets = []string{"stale-theme.css"}
	website.Assets = []string{"stale-site.css"}
	writeAssetTestFile(t, filepath.Join(website.Theme.StaticDirPath(), "theme.css"))
	writeAssetTestFile(t, filepath.Join(website.StaticDirPath(), "site.css"))

	if err := website.DiscoverAssets(false); err != nil {
		t.Fatalf("DiscoverAssets returned error: %v", err)
	}
	if want := []string{"theme.css"}; !reflect.DeepEqual(website.Theme.Assets, want) {
		t.Fatalf("Theme.Assets = %#v, want %#v", website.Theme.Assets, want)
	}
	if want := []string{"site.css"}; !reflect.DeepEqual(website.Assets, want) {
		t.Fatalf("Website.Assets = %#v, want %#v", website.Assets, want)
	}

	if err := os.Remove(filepath.Join(website.Theme.StaticDirPath(), "theme.css")); err != nil {
		t.Fatalf("remove theme asset: %v", err)
	}
	if err := os.Remove(filepath.Join(website.StaticDirPath(), "site.css")); err != nil {
		t.Fatalf("remove website asset: %v", err)
	}
	writeAssetTestFile(t, filepath.Join(website.Theme.StaticDirPath(), "next-theme.css"))
	writeAssetTestFile(t, filepath.Join(website.StaticDirPath(), "next-site.css"))

	if err := website.DiscoverAssets(false); err != nil {
		t.Fatalf("second DiscoverAssets returned error: %v", err)
	}
	if want := []string{"next-theme.css"}; !reflect.DeepEqual(website.Theme.Assets, want) {
		t.Fatalf("Theme.Assets after rediscovery = %#v, want %#v", website.Theme.Assets, want)
	}
	if want := []string{"next-site.css"}; !reflect.DeepEqual(website.Assets, want) {
		t.Fatalf("Website.Assets after rediscovery = %#v, want %#v", website.Assets, want)
	}
}

func TestWebsiteDiscoverPluginsReplacesExistingPlugins(t *testing.T) {
	website, _ := testWebsite(t)
	website.Theme.Plugins = htplugins.Plugins{"stale-theme": {Name: "stale-theme"}}
	website.Plugins = htplugins.Plugins{"stale-plugin": {Name: "stale-plugin"}}
	writeAssetTestScript(t, filepath.Join(website.Theme.PluginsDirPath(), "theme.js"), "// ht:plugin theme\nreturn input")
	writeAssetTestScript(t, filepath.Join(website.PluginsDirPath(), "site.js"), "// ht:plugin site\nreturn input")

	if err := website.DiscoverPlugins(false); err != nil {
		t.Fatalf("DiscoverPlugins returned error: %v", err)
	}
	if len(website.Theme.Plugins) != 1 || website.Theme.Plugins["theme"].Path != filepath.Join(website.Theme.PluginsDirPath(), "theme.js") {
		t.Fatalf("Theme.Plugins = %#v, want theme plugin", website.Theme.Plugins)
	}
	if len(website.Plugins) != 1 || website.Plugins["site"].Path != filepath.Join(website.PluginsDirPath(), "site.js") {
		t.Fatalf("Website.Plugins = %#v, want site plugin", website.Plugins)
	}

	if err := os.Remove(filepath.Join(website.Theme.PluginsDirPath(), "theme.js")); err != nil {
		t.Fatalf("remove theme plugin: %v", err)
	}
	if err := os.Remove(filepath.Join(website.PluginsDirPath(), "site.js")); err != nil {
		t.Fatalf("remove website plugin: %v", err)
	}
	writeAssetTestScript(t, filepath.Join(website.Theme.PluginsDirPath(), "next-theme.js"), "// ht:plugin nextTheme\nreturn input")
	writeAssetTestScript(t, filepath.Join(website.PluginsDirPath(), "next-site.js"), "// ht:plugin nextSite\nreturn input")

	if err := website.DiscoverPlugins(false); err != nil {
		t.Fatalf("second DiscoverPlugins returned error: %v", err)
	}
	if len(website.Theme.Plugins) != 1 || website.Theme.Plugins["nextTheme"].Path != filepath.Join(website.Theme.PluginsDirPath(), "next-theme.js") {
		t.Fatalf("Theme.Plugins after rediscovery = %#v, want nextTheme plugin", website.Theme.Plugins)
	}
	if len(website.Plugins) != 1 || website.Plugins["nextSite"].Path != filepath.Join(website.PluginsDirPath(), "next-site.js") {
		t.Fatalf("Website.Plugins after rediscovery = %#v, want nextSite plugin", website.Plugins)
	}
}

func TestWebsitePluginsUsesWebsitePrecedence(t *testing.T) {
	website, _ := testWebsite(t)
	writeAssetTestScript(t, filepath.Join(website.Theme.PluginsDirPath(), "label.js"), "// ht:plugin label\nreturn 'theme'")
	writeAssetTestScript(t, filepath.Join(website.PluginsDirPath(), "label.js"), "// ht:plugin label\nreturn 'website'")

	if err := website.DiscoverPlugins(false); err != nil {
		t.Fatalf("DiscoverPlugins returned error: %v", err)
	}
	plugins := htplugins.Merge(website.Theme.Plugins, website.Plugins)
	fn := plugins["label"].Func
	got, err := fn(nil)
	if err != nil {
		t.Fatalf("plugin label returned error: %v", err)
	}
	if got != "website" {
		t.Fatalf("plugin label = %q, want website", got)
	}
}

func TestPageDiscoverAssetsReplacesExistingAssets(t *testing.T) {
	website, _ := testWebsite(t)
	page := &Page{
		URI:    &url.URL{Path: filepath.Join(website.ContentDirPath(), "about", "index.md")},
		Assets: []string{"stale-photo.jpg"},
	}
	writeAssetTestFile(t, page.URI.Path)
	writeAssetTestFile(t, filepath.Join(website.ContentDirPath(), "about", "photo.jpg"))

	if err := page.DiscoverAssets(website, false); err != nil {
		t.Fatalf("DiscoverAssets returned error: %v", err)
	}
	if want := []string{"about/photo.jpg"}; !reflect.DeepEqual(page.Assets, want) {
		t.Fatalf("Page.Assets = %#v, want %#v", page.Assets, want)
	}

	if err := os.Remove(filepath.Join(website.ContentDirPath(), "about", "photo.jpg")); err != nil {
		t.Fatalf("remove page asset: %v", err)
	}
	writeAssetTestFile(t, filepath.Join(website.ContentDirPath(), "about", "next.jpg"))

	if err := page.DiscoverAssets(website, false); err != nil {
		t.Fatalf("second DiscoverAssets returned error: %v", err)
	}
	if want := []string{"about/next.jpg"}; !reflect.DeepEqual(page.Assets, want) {
		t.Fatalf("Page.Assets after rediscovery = %#v, want %#v", page.Assets, want)
	}
}

func TestPageAssetNames(t *testing.T) {
	page := &Page{Assets: []string{
		"about/photo.jpg",
		"about/notes.txt",
	}}

	got := page.AssetNames()
	want := []string{"photo.jpg", "notes.txt"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("AssetNames = %#v, want %#v", got, want)
	}
}

func TestAssetPathRejectsAbsolutePath(t *testing.T) {
	website, build := testWebsite(t)
	if _, err := website.AssetPublicPath("/etc/passwd"); err == nil {
		t.Fatal("AssetPublicPath accepted absolute path")
	}
	if _, err := website.AssetBuildPath(build, "../secrets.txt"); err == nil {
		t.Fatal("AssetBuildPath accepted parent traversal")
	}
	if _, err := website.ResourcePath(website.StaticDirPath(), "/etc/passwd"); err == nil {
		t.Fatal("ResourcePath accepted absolute path")
	}
}

func TestBuildPath(t *testing.T) {
	website, build := testWebsite(t)
	page := &Page{URI: &url.URL{Path: filepath.Join(website.ContentDirPath(), "about", "index.md")}}

	got := build.BuildPath(page)
	want := filepath.Join(build.URI.Path, "about", "index.html")
	if got != want {
		t.Fatalf("BuildPath = %q, want %q", got, want)
	}
}
