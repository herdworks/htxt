package hypertexting

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	hyperproviders "github.com/herdworks/hyperproviders-go"
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
	production := hyperproviders.HostingEnvironment{"pages"}
	website := &Website{
		Providers: hyperproviders.HostingProviders{
			"pages": {Kind: "git", Endpoint: "https://example.com"},
		},
		Environments: map[string]*hyperproviders.HostingEnvironment{
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

	missing := hyperproviders.HostingEnvironment{"missing"}
	website.Environments["broken"] = &missing
	if _, err := website.DeploymentEnvironment("broken"); err == nil {
		t.Fatal("DeploymentEnvironment missing provider error = nil, want error")
	}
}

func TestWebsite_DeploymentProviders(t *testing.T) {
	website := &Website{
		Providers: hyperproviders.HostingProviders{
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

func TestWebsiteConfigValidateAllowsEmptyDefaults(t *testing.T) {
	var config WebsiteConfig
	if err := config.Validate(); err != nil {
		t.Fatalf("WebsiteConfig.Validate() error = %v", err)
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
