package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/herdworks/clic"
)

func TestCLIComposesWebsiteCommands(t *testing.T) {
	if err := CLI.Validate(); err != nil {
		t.Fatalf("CLI.Validate() error = %v", err)
	}
	for _, key := range []string{"init", "build", "deploy", "server", "data", "page", "asset", "content-type", "schema"} {
		command, ok := CLI.SubCommand(key)
		if !ok {
			t.Errorf("CLI command %q is not registered", key)
			continue
		}
		if command.Spec.Key != key {
			t.Errorf("CLI command key = %q, want %q", command.Spec.Key, key)
		}
	}
	if _, ok := PageCommand().SubCommand("new"); ok {
		t.Fatal("page new command is registered")
	}
}

func TestCLIUsesHTXTEnvironmentPrefix(t *testing.T) {
	if CLI.Command.Meta.EnvPrefix != "HTXT_" {
		t.Fatalf("CLI environment prefix = %q", CLI.Command.Meta.EnvPrefix)
	}
	for _, option := range []clic.Option{
		WebsiteConfigOption(),
		BuildContentDirOption(),
		BuildDataDirOption(),
		BuildStaticDirOption(),
		BuildThemeOption(),
		DeploymentEnvironmentOption(),
		DeploymentProvidersOption(),
		ServerBindOption(),
		ServerPortOption(),
	} {
		if !strings.HasPrefix(option.Meta.EnvName, "{{ .EnvPrefix }}") {
			t.Fatalf("option %q environment name = %q", option.Spec.Key, option.Meta.EnvName)
		}
	}
}

func TestServerCommandDefaultsBuildIdentifier(t *testing.T) {
	option := ServerCommand().Spec.Options["id"]
	if option == nil {
		t.Fatal("server command has no build identifier option")
	}
	identifier, ok := option.Meta.Default.(string)
	if !ok || identifier == "" {
		t.Fatalf("server build identifier default = %#v, want UUID", option.Meta.Default)
	}
	if _, err := uuid.Parse(identifier); err != nil {
		t.Fatalf("server build identifier default = %q, want UUID: %v", identifier, err)
	}
}

func TestCLIUsageTemplatesAreMountable(t *testing.T) {
	command := Command()
	assertUsageTemplates(t, command, true)
}

func TestInitUsesWorkingDirectoryAndDefaults(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "example")
	result := CLI.Run(testExecution(t, filepath.Dir(directory)), clic.Arguments{"init", filepath.Base(directory)})
	if result.Error != nil {
		t.Fatalf("init error = %v", result.Error)
	}
	content, err := os.ReadFile(filepath.Join(directory, "site.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{
		"title: example",
		"name: example",
		"base_url: http://localhost",
		"description: This website is powered by HyperTemplates.",
	} {
		if !strings.Contains(string(content), value) {
			t.Errorf("site.yaml does not contain %q:\n%s", value, content)
		}
	}
}

func TestInitRejectsExistingWebsiteConfigurations(t *testing.T) {
	for _, name := range []string{"site.yaml", "site.yml", "site.json"} {
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			if err := os.WriteFile(filepath.Join(directory, name), []byte("{}"), 0644); err != nil {
				t.Fatal(err)
			}
			result := CLI.Run(testExecution(t, directory), clic.Arguments{"init"})
			if result.Error == nil || !strings.Contains(result.Error.Error(), "website configuration already exists") {
				t.Fatalf("init error = %v, want existing-config error", result.Error)
			}
		})
	}
}

func TestInitValidatesArgumentsAndBaseURL(t *testing.T) {
	execution := testExecution(t, t.TempDir())
	for _, arguments := range []clic.Arguments{
		{"init", "one", "two"},
		{"init", "--baseurl", "ftp://example.com"},
	} {
		result := CLI.Run(execution, arguments)
		if result.Error == nil {
			t.Fatalf("init %v unexpectedly succeeded", arguments)
		}
	}
}

func TestRootCommandReturnsFreshCommands(t *testing.T) {
	first, second := Command(), Command()
	if first.Commands["init"] == second.Commands["init"] {
		t.Fatal("RootCommand() returned shared commands")
	}
}

func testExecution(t *testing.T, workingDirectory string) clic.Execution {
	t.Helper()
	return clic.Execution{
		Context:          context.Background(),
		Stdin:            strings.NewReader(""),
		Stdout:           &bytes.Buffer{},
		Stderr:           io.Discard,
		LookupEnv:        func(string) (string, bool) { return "", false },
		WorkingDirectory: workingDirectory,
	}
}

func assertUsageTemplates(t *testing.T, command clic.Command, root bool) {
	t.Helper()
	prefix := "{{ .Command }}"
	if root {
		prefix = "{{ .CLI }}"
	}
	for _, usage := range command.Meta.Usage {
		if !strings.HasPrefix(usage, prefix) {
			t.Errorf("%q usage = %q, want prefix %q", command.Spec.Key, usage, prefix)
		}
	}
	for _, child := range command.Commands {
		assertUsageTemplates(t, *child, false)
	}
}
