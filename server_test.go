package htxt

import (
	"context"
	"errors"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/herdworks/htxtd"
)

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
