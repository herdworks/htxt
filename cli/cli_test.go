package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
		ServerHostOption(),
		ServerPortOption(),
	} {
		if !strings.HasPrefix(option.Meta.EnvName, "{{ .EnvPrefix }}") {
			t.Fatalf("option %q environment name = %q", option.Spec.Key, option.Meta.EnvName)
		}
	}
}

func TestBuildIdentifierOptionDefersIdentifierResolution(t *testing.T) {
	option := ServerCommand().Spec.Options["id"]
	if option == nil {
		t.Fatal("server command has no build identifier option")
	}
	if option.Meta.Default != nil {
		t.Fatalf("server build identifier default = %#v, want nil", option.Meta.Default)
	}
	if got, want := option.Meta.EnvName, "{{ .EnvPrefix }}BUILD_ID"; got != want {
		t.Fatalf("build identifier environment = %q, want %q", got, want)
	}
	if BuildCommand().Spec.Options["id"].Meta.Default != nil {
		t.Fatalf("build command identifier default = %#v, want nil", BuildCommand().Spec.Options["id"].Meta.Default)
	}
	bind := ServerCommand().Spec.Options["bind"]
	if bind == nil || bind.Meta.Default != nil {
		t.Fatalf("server bind default = %#v, want nil", bind)
	}
	port := ServerCommand().Spec.Options["port"]
	if port == nil || port.Meta.Default != nil {
		t.Fatalf("server port default = %#v, want nil", port)
	}
}

func TestCLIUsageTemplatesAreMountable(t *testing.T) {
	command := Command()
	assertUsageTemplates(t, command, true)
}

func TestInitUsesWorkingDirectoryAndDefaults(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "example")
	execution := testExecution(t, filepath.Dir(directory))
	result := CLI.Run(execution, clic.Arguments{"init", filepath.Base(directory)})
	if result.Error != nil {
		t.Fatalf("init error = %v", result.Error)
	}
	if got, want := execution.Stdout.(*bytes.Buffer).String(), `file "content/about/index.md" created
file "content/blog/hello-again/index.md" created
file "content/blog/hello-world/index.md" created
file "content/blog/index.md" created
file "content/index.md" created
file "data/nav.yaml" created
file "static/css/reset.css" created
file "static/css/style.css" created
file "static/favicon.ico" created
file "static/js/components/example-component.js" created
file "static/js/components/relative-time.js" created
file "static/js/main.js" created
file "themes/default/fragments/footer.html" created
file "themes/default/fragments/head.html" created
file "themes/default/fragments/header.html" created
file "themes/default/fragments/icons/feed.svg" created
file "themes/default/layouts/blog.html" created
file "themes/default/layouts/default.html" created
file "themes/default/theme.json" created
website "site.yaml" created
`; got != want {
		t.Errorf("init output =\n%s\nwant:\n%s", got, want)
	}
	content, err := os.ReadFile(filepath.Join(directory, "site.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	head, err := os.ReadFile(filepath.Join(directory, "themes", "default", "fragments", "head.html"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(head), "chroma-base16-snazzy.css") {
		t.Errorf("bootstrap head still links removed Chroma stylesheet:\n%s", head)
	}
	footer, err := os.ReadFile(filepath.Join(directory, "themes", "default", "fragments", "footer.html"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(footer), "href='${ site.config.feed_path }/main.xml'"; !strings.Contains(got, want) {
		t.Errorf("bootstrap footer does not link the generated main feed:\n%s", got)
	}
	helloWorld, err := os.ReadFile(filepath.Join(directory, "content", "blog", "hello-world", "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(helloWorld), "[feed]: /feeds/main.xml"; !strings.Contains(got, want) {
		t.Errorf("bootstrap hello-world post does not link the generated main feed:\n%s", got)
	}
	for _, value := range []string{
		"title: example",
		"name: example",
		"base_url: http://localhost",
		"description: This website is powered by HyperTexting.",
		"feeds:\n    blog:\n        include:\n            - path: /blog/*/**\n    main: {}",
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
