package htxt

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	hsync "github.com/herdworks/hsync"
	"github.com/herdworks/httt"
	"github.com/herdworks/htup"
)

func TestWebsite_ResolveEnvironmentsDefaultsToDefaultProvider(t *testing.T) {
	website := &Website{}

	website.ResolveEnvironments()

	env := website.Environments["default"]
	if env == nil {
		t.Fatal("default environment missing")
	}
	if len(*env) != 1 || (*env)[0] != "default" {
		t.Fatalf("default environment = %#v, want [default]", *env)
	}
}

func TestWebsite_DeploymentEnvironment(t *testing.T) {
	production := htup.Environment{"pages"}
	website := &Website{
		Providers: htup.HostingProviders{
			"pages": {Kind: "git", Endpoint: "https://example.com"},
		},
		Environments: map[string]*htup.Environment{
			"production": &production,
		},
	}

	env, err := website.DeploymentEnvironment("production")
	if err != nil {
		t.Fatalf("DeploymentEnvironment: %v", err)
	}
	if len(*env) != 1 || (*env)[0] != "pages" {
		t.Fatalf("environment = %#v, want [pages]", *env)
	}

	if _, err := website.DeploymentEnvironment(""); err == nil {
		t.Fatal("DeploymentEnvironment empty name error = nil, want error")
	}
	if _, err := website.DeploymentEnvironment("missing"); err == nil {
		t.Fatal("DeploymentEnvironment missing name error = nil, want error")
	}

	website.Environments["empty"] = nil
	if _, err := website.DeploymentEnvironment("empty"); err == nil {
		t.Fatal("DeploymentEnvironment empty error = nil, want error")
	}

	missing := htup.Environment{"missing"}
	website.Environments["broken"] = &missing
	if _, err := website.DeploymentEnvironment("broken"); err == nil {
		t.Fatal("DeploymentEnvironment missing provider error = nil, want error")
	}
}

func TestWebsite_DeploymentProviders(t *testing.T) {
	website := &Website{
		Providers: htup.HostingProviders{
			"pages":  {Kind: "git", Endpoint: "https://example.com"},
			"assets": {Kind: "s3", Endpoint: "https://example.com"},
		},
	}

	env, err := website.DeploymentProviders([]string{"pages", "assets"})
	if err != nil {
		t.Fatalf("DeploymentProviders: %v", err)
	}
	if len(env) != 2 || env[0] != "pages" || env[1] != "assets" {
		t.Fatalf("environment = %#v, want pages/assets", env)
	}
}

func TestWebsiteResolveHrefUsesURLReferenceSemantics(t *testing.T) {
	website := &Website{BaseURL: "https://example.com/blog/"}
	tests := []struct {
		name string
		href string
		want string
	}{
		{name: "absolute", href: "https://elsewhere.example/post/?q=1#top", want: "https://elsewhere.example/post/?q=1#top"},
		{name: "protocol relative", href: "//cdn.example/avatar.png", want: "https://cdn.example/avatar.png"},
		{name: "root relative", href: "/about/?q=1#top", want: "https://example.com/about/?q=1#top"},
		{name: "path relative", href: "assets/avatar.png?q=1#top", want: "https://example.com/blog/assets/avatar.png?q=1#top"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := website.ResolveHref(test.href)
			if err != nil {
				t.Fatalf("ResolveHref() error = %v", err)
			}
			if got.String() != test.want {
				t.Fatalf("ResolveHref() = %q, want %q", got.String(), test.want)
			}
		})
	}
}

func TestWebsiteValidateRequiresAbsoluteBaseURL(t *testing.T) {
	website := &Website{URI: mustHref(t, "site.yaml"), BaseURL: "/relative", Title: "Example", Description: "Example", Byline: Byline{Name: "Example"}}
	if err := website.Validate(); err == nil {
		t.Fatal("Validate accepted a relative base_url")
	}
}

func TestWebsiteDirectoryGettersDoNotMutateConfig(t *testing.T) {
	website, _ := testWebsite(t)
	root := website.RootDir()

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"BuildsDirPath", website.BuildsDirPath(), filepath.Join(root, "builds")},
		{"ContentDirPath", website.ContentDirPath(), filepath.Join(root, "content")},
		{"DataDirPath", website.DataDirPath(), filepath.Join(root, "data")},
		{"DeploymentsDirPath", website.DeploymentsDirPath(), filepath.Join(root, "deployments")},
		{"PluginsDirPath", website.PluginsDirPath(), filepath.Join(root, "plugins")},
		{"StaticDirPath", website.StaticDirPath(), filepath.Join(root, "static")},
		{"ThemesDirPath", website.ThemesDirPath(), filepath.Join(root, "themes")},
		{"ThemeURI", website.ThemeURI().Path, filepath.Join(root, "theme.json")},
	}
	for _, test := range tests {
		if test.got != test.want {
			t.Fatalf("%s = %q, want %q", test.name, test.got, test.want)
		}
	}
	if website.Config != (WebsiteConfig{}) {
		t.Fatalf("Website path getters mutated config: %#v", website.Config)
	}
}

func TestWebsiteDirectoryGettersUseConfiguredDirs(t *testing.T) {
	website, _ := testWebsite(t)
	website.Config = WebsiteConfig{
		BuildsDirectory:      "dist",
		ContentDirectory:     "pages",
		DataDirectory:        "records",
		DeploymentsDirectory: "releases",
		PluginsDirectory:     "extensions",
		StaticDirectory:      "public",
		ThemesDirectory:      "designs",
		ThemePath:            "themes/main/theme.json",
	}
	website.Sync = &hsync.SyncConfig{Remote: "https://example.com/.sync/latest.json"}
	wantConfig := website.Config
	root := website.RootDir()

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"BuildsDirPath", website.BuildsDirPath(), filepath.Join(root, "dist")},
		{"ContentDirPath", website.ContentDirPath(), filepath.Join(root, "pages")},
		{"DataDirPath", website.DataDirPath(), filepath.Join(root, "records")},
		{"DeploymentsDirPath", website.DeploymentsDirPath(), filepath.Join(root, "releases")},
		{"PluginsDirPath", website.PluginsDirPath(), filepath.Join(root, "extensions")},
		{"StaticDirPath", website.StaticDirPath(), filepath.Join(root, "public")},
		{"ThemesDirPath", website.ThemesDirPath(), filepath.Join(root, "designs")},
		{"ThemeURI", website.ThemeURI().Path, filepath.Join(root, "themes", "main", "theme.json")},
	}
	for _, test := range tests {
		if test.got != test.want {
			t.Fatalf("%s = %q, want %q", test.name, test.got, test.want)
		}
	}
	if website.Config != wantConfig {
		t.Fatalf("Website path getters mutated config: %#v, want %#v", website.Config, wantConfig)
	}
}

func TestWebsiteConfigValidateRejectsUnsafePaths(t *testing.T) {
	tests := []struct {
		name   string
		config WebsiteConfig
	}{
		{name: "builds", config: WebsiteConfig{BuildsDirectory: "../builds"}},
		{name: "content", config: WebsiteConfig{ContentDirectory: "/content"}},
		{name: "data", config: WebsiteConfig{DataDirectory: `data\records`}},
		{name: "deployments", config: WebsiteConfig{DeploymentsDirectory: "."}},
		{name: "plugins", config: WebsiteConfig{PluginsDirectory: "/plugins"}},
		{name: "static", config: WebsiteConfig{StaticDirectory: `static\assets`}},
		{name: "themes", config: WebsiteConfig{ThemesDirectory: "../themes"}},
		{name: "theme", config: WebsiteConfig{ThemePath: "../theme.json"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.config.Validate(); err == nil {
				t.Fatal("WebsiteConfig.Validate() error = nil")
			}
		})
	}
}

func TestWebsiteValidateRejectsInvalidSyncConfig(t *testing.T) {
	tests := []hsync.SyncConfig{
		{},
		{Remote: "/.sync/latest.json"},
		{Remote: "https://example.com/.sync/latest.json?preview=1"},
		{Remote: "https://example.com/.sync/latest.json#head"},
	}
	for _, sync := range tests {
		website := &Website{
			URI:         mustHref(t, "site.yaml"),
			BaseURL:     "https://example.com",
			Title:       "Example",
			Description: "Example website",
			Byline:      Byline{Name: "Example"},
			Sync:        &sync,
		}
		if err := website.Validate(); err == nil {
			t.Fatalf("Website.Validate() error = nil for sync %#v", sync)
		}
	}
}

func TestWebsiteSyncIsOptionalAndDecodesConfiguration(t *testing.T) {
	encoded, err := json.Marshal(Website{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), `"sync"`) {
		t.Fatalf("empty website unexpectedly encoded sync: %s", encoded)
	}
	var website Website
	if err := json.Unmarshal([]byte(`{"sync":{"remote":"https://example.com/.sync/latest.json"}}`), &website); err != nil {
		t.Fatal(err)
	}
	if website.Sync == nil || website.Sync.Remote != "https://example.com/.sync/latest.json" {
		t.Fatalf("decoded sync = %#v", website.Sync)
	}
}

func TestWebsiteConfigValidateAllowsEmptyDefaults(t *testing.T) {
	var config WebsiteConfig
	if err := config.Validate(); err != nil {
		t.Fatalf("WebsiteConfig.Validate() error = %v", err)
	}
}

func TestWebsiteConfigResolveStatusDefaultsAndOverrides(t *testing.T) {
	config := WebsiteConfig{}
	config.Resolve(nil)
	if config.StatusLayout != "builtin/status.html" || config.StatusRedirect != "/" {
		t.Fatalf("status config = %#v, want builtin layout and root redirect", config)
	}
	config.Resolve(&WebsiteConfig{StatusLayout: "custom-status", StatusRedirect: "https://example.com/follow"})
	if config.StatusLayout != "custom-status" || config.StatusRedirect != "https://example.com/follow" {
		t.Fatalf("status overrides = %#v", config)
	}
}

func TestWebsiteConfigTemplateDataPreservesCustomProperties(t *testing.T) {
	website := &Website{
		Config: WebsiteConfig{ThemePath: "resolved-theme.json", StatusRedirect: "/following/"},
		TemplateData: &httt.TemplateData{
			"config": httt.TemplateData{"custom": "retained", "theme": "source-theme.json"},
		},
	}

	data := website.configTemplateData()
	if got := data.Get("custom"); got != "retained" {
		t.Fatalf("custom config = %#v, want retained", got)
	}
	if got := data.Get("theme"); got != "resolved-theme.json" {
		t.Fatalf("config.theme = %#v, want resolved value", got)
	}
	if got := data.Get("status_redirect"); got != "/following/" {
		t.Fatalf("config.status_redirect = %#v, want resolved value", got)
	}
}

func TestWebsiteLoadRejectsUnsafeThemePath(t *testing.T) {
	website, _ := testWebsite(t)
	source := []byte(`
base_url: https://example.com/
title: Test Site
description: Test description
byline:
  name: Test Author
config:
  theme: ../theme.json
`)
	if err := os.WriteFile(website.URI.Path, source, 0644); err != nil {
		t.Fatalf("write site config: %v", err)
	}

	err := website.Load(false, nil, io.Discard)
	if err == nil {
		t.Fatal("Load() error = nil")
	}
	if !strings.Contains(err.Error(), "site.config.theme") {
		t.Fatalf("Load() error = %v, want site.config.theme", err)
	}
}
