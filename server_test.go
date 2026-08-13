package htxt

import (
	"context"
	"net"
	"path/filepath"
	"strconv"
	"testing"

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
