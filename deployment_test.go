package htxt

import (
	"bytes"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/herdworks/htup"
	href "github.com/herdworks/htxt/href"
)

func newDeploymentTestWebsite(t *testing.T) *Website {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "site.yaml")
	writeBuildSelectionFile(t, path, `base_url: https://example.com/
title: Test Site
description: Test description
byline:
  name: Test Author
config:
  theme: theme.json
sync:
  remote: https://example.com
`)
	writeBuildSelectionFile(t, filepath.Join(root, "theme.json"), "{}")
	return &Website{URI: &href.Href{URL: url.URL{Path: path}}}
}

func TestDeploymentSelectsEnvironment(t *testing.T) {
	production := htup.Environment{"pages"}
	website := newDeploymentTestWebsite(t)
	website.Providers = htup.HostingProviders{"pages": {Kind: "git", Endpoint: "https://example.com"}}
	website.Environments = map[string]*htup.Environment{"production": &production}
	build := &Build{
		Config:  BuildConfig{URI: &url.URL{Path: "/tmp/site/builds/production"}},
		Website: website,
	}

	deployment, err := (&Deployment{Build: build, Config: DeploymentConfig{Environment: "production"}}).configure()
	if err != nil {
		t.Fatalf("configure() error = %v", err)
	}
	if deployment.Deployment.Environment != "production" {
		t.Fatalf("environment = %q, want production", deployment.Deployment.Environment)
	}
	if len(deployment.Deployment.Providers) != 0 {
		t.Fatalf("providers = %#v, want environment selection", deployment.Deployment.Providers)
	}
	if deployment.Deployment.Kind != htup.CompleteDeployment {
		t.Fatalf("kind = %q, want complete", deployment.Deployment.Kind)
	}
	if _, err := (&Deployment{Build: build, Config: DeploymentConfig{Environment: "missing"}}).configure(); err == nil {
		t.Fatal("configure() missing environment error = nil")
	}
}

func TestDeploymentSelectsDefaultEnvironment(t *testing.T) {
	defaultEnvironment := htup.Environment{"pages"}
	website := newDeploymentTestWebsite(t)
	website.Providers = htup.HostingProviders{"pages": {Kind: "git", Endpoint: "https://example.com"}}
	website.Environments = map[string]*htup.Environment{"default": &defaultEnvironment}
	build := &Build{
		Config:  BuildConfig{URI: &url.URL{Path: "/tmp/site/builds/default"}},
		Website: website,
	}

	configured, err := (&Deployment{Build: build}).configure()
	if err != nil {
		t.Fatalf("configure() error = %v", err)
	}
	if configured.Deployment.Environment != "default" {
		t.Fatalf("environment = %q, want default", configured.Deployment.Environment)
	}
}

func TestDeploymentSelectsProvidersAndIncrementalKind(t *testing.T) {
	website := newDeploymentTestWebsite(t)
	website.Providers = htup.HostingProviders{
		"pages":  {Kind: "git", Endpoint: "https://example.com"},
		"assets": {Kind: "s3", Endpoint: "https://example.com"},
	}
	build := &Build{
		Config:  BuildConfig{URI: &url.URL{Path: "/tmp/site/builds/incremental"}, Incremental: true},
		Website: website,
	}

	deployment, err := (&Deployment{Build: build, Config: DeploymentConfig{Providers: []string{"pages", "assets"}}}).configure()
	if err != nil {
		t.Fatalf("configure() error = %v", err)
	}
	if deployment.Deployment.Environment != "" {
		t.Fatalf("environment = %q, want empty direct-provider selection", deployment.Deployment.Environment)
	}
	if len(deployment.Deployment.Providers) != 2 {
		t.Fatalf("providers = %#v, want pages and assets", deployment.Deployment.Providers)
	}
	if deployment.Deployment.Kind != htup.IncrementalDeployment {
		t.Fatalf("kind = %q, want incremental", deployment.Deployment.Kind)
	}
}

func TestDeploymentRejectsInvalidTarget(t *testing.T) {
	build := &Build{Config: BuildConfig{URI: &url.URL{Path: "/tmp/site/builds/test"}}, Website: &Website{URI: &href.Href{URL: url.URL{Path: "/tmp/site/site.yaml"}}}}
	if _, err := (&Deployment{Build: build, Config: DeploymentConfig{Environment: "production", Providers: []string{"pages"}}}).configure(); err == nil {
		t.Fatal("configure() combined target error = nil")
	}
}

func TestDeploymentAllowsPreviewBuild(t *testing.T) {
	website := newDeploymentTestWebsite(t)
	website.Providers = htup.HostingProviders{"pages": {Kind: "git", Endpoint: "https://example.com"}}
	website.Environments = map[string]*htup.Environment{"default": ptr(htup.Environment{"pages"})}
	build := &Build{
		Config:  BuildConfig{URI: &url.URL{Path: "/tmp/site/builds/preview"}, Preview: true},
		Website: website,
	}
	if _, err := (&Deployment{Build: build}).configure(); err != nil {
		t.Fatalf("configure() error = %v", err)
	}
}

func TestDeploymentRejectsDraftsDirectory(t *testing.T) {
	deployment := &Deployment{Config: DeploymentConfig{Build: BuildConfig{DraftsDirectory: "drafts"}}}
	if err := deployment.Execute(t.Context()); err == nil || !strings.Contains(err.Error(), "does not support build.config.drafts_dir") {
		t.Fatalf("Deployment.Execute() error = %v, want drafts directory rejection", err)
	}
}

func TestDeploymentWritesBuildRelativeLog(t *testing.T) {
	fixture := newBuildExecutionFixture(t)
	writeBuildSelectionFile(t, fixture.sitePath, `base_url: https://example.com/
title: Test Site
description: Test description
byline:
  name: Test Author
config:
  theme: theme.json
sync:
  remote: https://example.com
providers:
  default:
    kind: missing
    endpoint: https://example.com
`)
	var output bytes.Buffer
	deployment := &Deployment{Config: DeploymentConfig{Build: BuildConfig{
		WebsiteConfig: WebsiteConfig{Path: fixture.sitePath},
		ID:            "deployment",
	}}, Writer: &output}
	if err := deployment.Execute(t.Context()); err == nil {
		t.Fatal("Execute() error = nil, want missing provider error")
	}
	if got, want := output.String(), "No deployment target specified; using default environment \"default\"\n"; !strings.Contains(got, want) {
		t.Fatalf("deployment output = %q, want %q", got, want)
	}
	entries, err := os.ReadDir(filepath.Join(deployment.Build.Config.URI.Path, htup.ProviderWorkspaceDir, "logs"))
	if err != nil {
		t.Fatalf("ReadDir(deployment logs) error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("deployment logs = %d, want 1", len(entries))
	}
}

func ptr[T any](value T) *T { return &value }
