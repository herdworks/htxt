package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/herdworks/clic"
)

func TestServerHostControlsPreviewBuildURLs(t *testing.T) {
	for _, test := range []struct {
		name string
		args clic.Arguments
		env  string
		want string
	}{
		{name: "default", want: "http://localhost:8131/"},
		{name: "flag", args: clic.Arguments{"--host", "blackbook.local", "--port", "5555"}, want: "http://blackbook.local:5555/"},
		{name: "environment", env: "blackbook.local", want: "http://blackbook.local:8131/"},
		{name: "flag overrides environment", args: clic.Arguments{"--host", "blackbook.local"}, env: "other.local", want: "http://blackbook.local:8131/"},
		{name: "IPv4", args: clic.Arguments{"--host", "192.168.1.10"}, want: "http://192.168.1.10:8131/"},
		{name: "IPv6", args: clic.Arguments{"--host", "::1"}, want: "http://[::1]:8131/"},
		{name: "bracketed IPv6", args: clic.Arguments{"--host", "[::1]"}, want: "http://[::1]:8131/"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			for name, content := range map[string]string{
				"site.yaml": `base_url: https://example.com/
title: Preview Test
description: Preview test website
byline:
  name: Test Author
feeds:
  main:
    html_url: /
    include:
      - path: /**
config:
  theme: theme.json
  preview:
    id: host-test
    bind: 0.0.0.0
    port: 8131
`,
				"theme.json":           "{}",
				"content/index.md":     "---\nlayout: default\ntitle: Home\nredirect: /\n---\n\nHome",
				"layouts/default.html": `<html><head><link ht-apply rel="canonical" href="${ page.canonical_url }"></head><body></body></html>`,
			} {
				path := filepath.Join(root, name)
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(content), 0644); err != nil {
					t.Fatal(err)
				}
			}
			command := Command()
			called := false
			command.Commands["server"].Spec.Handler = func(execution *clic.Execution) error {
				called = true
				server, err := serverForExecution(execution)
				if err != nil {
					return err
				}
				if server.Bind != "0.0.0.0" {
					t.Fatalf("Bind = %q, want 0.0.0.0", server.Bind)
				}
				// Two builds cover the initial preview and a live-reload rebuild.
				for range 2 {
					website, err := server.Build(execution.Context)
					if err != nil {
						return err
					}
					if website.BaseURL != test.want {
						t.Fatalf("BaseURL = %q, want %q", website.BaseURL, test.want)
					}
					page, err := os.ReadFile(filepath.Join(server.Directory, "index.html"))
					if err != nil {
						return err
					}
					if !strings.Contains(string(page), `href="`+test.want+`"`) {
						t.Fatalf("page does not use canonical URL %q: %s", test.want, page)
					}
					feed, err := os.ReadFile(filepath.Join(server.Directory, "feeds", "main.xml"))
					if err != nil {
						return err
					}
					if !strings.Contains(string(feed), test.want) || strings.Contains(string(feed), "https://example.com/") {
						t.Fatalf("feed does not use preview origin %q: %s", test.want, feed)
					}
				}
				return nil
			}
			execution := testExecution(t, root)
			execution.LookupEnv = func(key string) (string, bool) {
				return test.env, key == "HTXT_HOST" && test.env != ""
			}
			cli := clic.CLI{Command: command}
			args := append(clic.Arguments{"server", "--config", filepath.Join(root, "site.yaml")}, test.args...)
			if result := cli.Run(execution, args); result.Error != nil {
				t.Fatal(result.Error)
			}
			if !called {
				t.Fatal("server handler was not called")
			}
		})
	}
}

func TestServerBaseURLRejectsInvalidHosts(t *testing.T) {
	for _, host := range []string{"", "http://blackbook.local", "blackbook.local:5555", "blackbook.local/path", "blackbook.local?query", "blackbook.local#fragment", "user@blackbook.local", "blackbook local", "[localhost]", "bad%host"} {
		t.Run(host, func(t *testing.T) {
			if _, err := serverBaseURL(host, 5555); err == nil {
				t.Fatalf("serverBaseURL(%q) unexpectedly succeeded", host)
			}
		})
	}
}
