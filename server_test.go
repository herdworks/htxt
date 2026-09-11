package htxt

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/herdworks/htxtd"
)

func TestServerResolveConfigUsesConfiguredPreviewDefaults(t *testing.T) {
	fixture := newBuildExecutionFixture(t)
	writeBuildSelectionFile(t, fixture.sitePath, `base_url: https://example.com/
title: Test Site
description: Test description
byline:
  name: Test Author
config:
  theme: theme.json
  preview:
    bind: 127.0.0.1
    id: preview-site
    port: 8131
`)
	server := Server{WebsiteURI: mustHref(t, fixture.sitePath)}
	if err := server.ResolveConfig(); err != nil {
		t.Fatalf("ResolveConfig() error = %v", err)
	}
	if got, want := server.BuildID, "preview-site"; got != want {
		t.Fatalf("BuildID = %q, want %q", got, want)
	}
	if got, want := server.Port, 8131; got != want {
		t.Fatalf("Port = %d, want %d", got, want)
	}
	if got, want := server.Bind, "127.0.0.1"; got != want {
		t.Fatalf("Bind = %q, want %q", got, want)
	}
}

func TestServerResolveConfigPreservesExplicitPreviewOverrides(t *testing.T) {
	fixture := newBuildExecutionFixture(t)
	writeBuildSelectionFile(t, fixture.sitePath, `base_url: https://example.com/
title: Test Site
description: Test description
byline:
  name: Test Author
config:
  theme: theme.json
  preview:
    bind: configured-bind
    id: configured-preview
    port: 8131
`)
	server := Server{
		Server:     htxtd.Server{Bind: "explicit-bind", Port: 9131},
		WebsiteURI: mustHref(t, fixture.sitePath),
		BuildID:    "explicit-preview",
	}
	if err := server.ResolveConfig(); err != nil {
		t.Fatalf("ResolveConfig() error = %v", err)
	}
	if got, want := server.BuildID, "explicit-preview"; got != want {
		t.Fatalf("BuildID = %q, want %q", got, want)
	}
	if got, want := server.Port, 9131; got != want {
		t.Fatalf("Port = %d, want %d", got, want)
	}
	if got, want := server.Bind, "explicit-bind"; got != want {
		t.Fatalf("Bind = %q, want %q", got, want)
	}
}

func TestServerResolveConfigPreservesLegacyDefaults(t *testing.T) {
	fixture := newBuildExecutionFixture(t)
	server := Server{WebsiteURI: mustHref(t, fixture.sitePath)}
	if err := server.ResolveConfig(); err != nil {
		t.Fatalf("ResolveConfig() error = %v", err)
	}
	if _, err := uuid.Parse(server.BuildID); err != nil {
		t.Fatalf("BuildID = %q, want UUID: %v", server.BuildID, err)
	}
	if got, want := server.Port, 8080; got != want {
		t.Fatalf("Port = %d, want %d", got, want)
	}
}

func TestServerBuildUsesStablePreviewDirectory(t *testing.T) {
	fixture := newBuildExecutionFixture(t)
	server := Server{
		Server: htxtd.Server{
			Bind: "0.0.0.0",
			Port: 8080,
		},
		WebsiteURI: mustHref(t, fixture.sitePath),
		BuildID:    "development",
	}

	website, err := server.Build(context.Background())
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got, want := server.Directory, filepath.Join(fixture.root, "builds", "development"); got != want {
		t.Fatalf("Directory = %q, want %q", got, want)
	}
	if got, want := website.BaseURL, "http://localhost:8080/"; got != want {
		t.Fatalf("BaseURL = %q, want %q", got, want)
	}
	content, err := os.ReadFile(filepath.Join(server.Directory, ".well-known", "hypertexting", "build"))
	if err != nil {
		t.Fatalf("ReadFile(build marker) error = %v", err)
	}
	if got, want := string(content), "development\n"; got != want {
		t.Fatalf("build marker = %q, want %q", got, want)
	}
}

func TestServerBuildRefreshesThemeResourceCache(t *testing.T) {
	fixture := newBuildExecutionFixture(t)
	if err := os.Remove(filepath.Join(fixture.root, "content", "index.html")); err != nil {
		t.Fatalf("Remove(index.html) error = %v", err)
	}
	writeBuildSelectionFile(t, filepath.Join(fixture.root, "content", "index.md"), "---\ntitle: Home\nredirect: /\n---\n\nHome")
	layoutPath := filepath.Join(fixture.root, "layouts", "default.html")
	writeBuildSelectionFile(t, layoutPath, "<html><body>first</body></html>")
	server := Server{
		Server: htxtd.Server{
			Bind: "0.0.0.0",
			Port: 8080,
		},
		WebsiteURI: mustHref(t, fixture.sitePath),
		BuildID:    "development",
	}

	if _, err := server.Build(context.Background()); err != nil {
		t.Fatalf("first Build() error = %v", err)
	}
	outputPath := filepath.Join(server.Directory, "index.html")
	output, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile(first index.html) error = %v", err)
	}
	if !strings.Contains(string(output), "<body>first</body>") {
		t.Fatalf("first output = %q", output)
	}

	writeBuildSelectionFile(t, layoutPath, "<html><body>second</body></html>")
	if _, err := server.Build(context.Background()); err != nil {
		t.Fatalf("second Build() error = %v", err)
	}
	output, err = os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile(second index.html) error = %v", err)
	}
	if !strings.Contains(string(output), "<body>second</body>") || strings.Contains(string(output), "<body>first</body>") {
		t.Fatalf("second output = %q", output)
	}
}

func TestServerRunReturnsListenErrorBeforeStartingWatcher(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	_, value, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(value)
	if err != nil {
		t.Fatal(err)
	}
	fixture := newBuildExecutionFixture(t)
	server := Server{
		Server: htxtd.Server{
			Bind: "127.0.0.1",
			Port: port,
		},
		WebsiteURI: mustHref(t, fixture.sitePath),
		BuildID:    "development",
	}
	if err = server.Run(context.Background()); err == nil {
		t.Fatal("Run() unexpectedly succeeded")
	}
}

func TestServerRunCancellationJoinsPreviewServerAndWatcher(t *testing.T) {
	fixture := newBuildExecutionFixture(t)
	port := reserveServerPort(t)
	server := newPreviewServer(t, fixture, port)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errors := make(chan error, 1)
	go func() { errors <- server.Run(ctx) }()
	waitForPreviewServer(t, port, errors)

	cancel()
	if err := waitForServerRun(t, errors); err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
	assertPreviewListenerReusable(t, port)
}

func TestServerRunWithReadySignalsBoundListener(t *testing.T) {
	fixture := newBuildExecutionFixture(t)
	port := reserveServerPort(t)
	server := newPreviewServer(t, fixture, port)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan *url.URL, 1)
	errors := make(chan error, 1)
	go func() {
		errors <- server.RunWithReady(ctx, func(value *url.URL) {
			ready <- value
		})
	}()

	select {
	case value := <-ready:
		if want := "http://127.0.0.1:" + strconv.Itoa(port) + "/"; value.String() != want {
			t.Fatalf("ready URL = %q, want %q", value, want)
		}
	case err := <-errors:
		t.Fatalf("RunWithReady() returned before readiness: %v", err)
	case <-time.After(time.Second):
		t.Fatal("RunWithReady() did not report readiness")
	}

	cancel()
	if err := waitForServerRun(t, errors); err != nil {
		t.Fatalf("RunWithReady() error = %v, want nil", err)
	}
	select {
	case value := <-ready:
		t.Fatalf("unexpected second readiness: %#v", value)
	default:
	}
	assertPreviewListenerReusable(t, port)
}

func TestServerRunWatcherFailureJoinsPreviewServer(t *testing.T) {
	fixture := newBuildExecutionFixture(t)
	port := reserveServerPort(t)
	server := newPreviewServer(t, fixture, port)
	errors := make(chan error, 1)
	go func() { errors <- server.Run(context.Background()) }()
	waitForPreviewServer(t, port, errors)

	server.Watcher.SetWebsite(&Website{})
	if err := waitForServerRun(t, errors); err == nil {
		t.Fatal("Run() error = nil, want watcher failure")
	}
	assertPreviewListenerReusable(t, port)
}

func TestDrainServerErrorsReturnsForcedShutdownErrorAfterCancellation(t *testing.T) {
	forcedShutdown := errors.New("forced HTTP shutdown")
	serverErrors := make(chan error, 2)
	serverErrors <- nil
	serverErrors <- forcedShutdown

	if err := drainServerErrors(serverErrors, 0, nil); !errors.Is(err, forcedShutdown) {
		t.Fatalf("drainServerErrors() error = %v, want forced shutdown error", err)
	}
}

func newPreviewServer(t *testing.T, fixture buildExecutionFixture, port int) *Server {
	t.Helper()
	return &Server{
		Server: htxtd.Server{
			Bind: "127.0.0.1",
			Port: port,
		},
		WebsiteURI: mustHref(t, fixture.sitePath),
		BuildID:    "development",
	}
}

func reserveServerPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	_, value, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(value)
	if err != nil {
		t.Fatal(err)
	}
	return port
}

func waitForPreviewServer(t *testing.T, port int, errors <-chan error) {
	t.Helper()
	client := &http.Client{Timeout: 100 * time.Millisecond}
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	for {
		response, err := client.Get("http://127.0.0.1:" + strconv.Itoa(port) + "/")
		if err == nil {
			response.Body.Close()
			if response.StatusCode != http.StatusOK {
				t.Fatalf("GET / status = %d, want %d", response.StatusCode, http.StatusOK)
			}
			return
		}
		select {
		case runErr := <-errors:
			t.Fatalf("Run() returned before listening: %v", runErr)
		case <-deadline.C:
			t.Fatalf("preview server did not listen: %v", err)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func waitForServerRun(t *testing.T, errors <-chan error) error {
	t.Helper()
	select {
	case err := <-errors:
		return err
	case <-time.After(time.Second):
		t.Fatal("Run() did not return")
		return nil
	}
}

func assertPreviewListenerReusable(t *testing.T, port int) {
	t.Helper()
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		t.Fatalf("preview listener was not released: %v", err)
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
}
