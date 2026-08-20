package htxt

import (
	"context"
	"fmt"
	"net/url"
	"os"

	"github.com/herdworks/htxt/href"
	"github.com/herdworks/htxtd"
)

// Server builds, watches, and serves a preview website.
type Server struct {
	htxtd.Server
	Watcher

	WebsiteURI  *href.Href
	BuildID     string
	BuildConfig BuildConfig
	BaseURL     string
}

// Run builds the website, serves it, and rebuilds after source changes.
func (server *Server) Run(ctx context.Context) (err error) {
	return server.run(ctx, nil)
}

// RunWithReady builds the website, starts serving it, and rebuilds after source
// changes. It invokes ready after the initial build succeeds, the listener is
// bound, and the server and watcher goroutines have been started. ready is
// called synchronously and must return promptly.
func (server *Server) RunWithReady(ctx context.Context, ready func(*url.URL)) (err error) {
	return server.run(ctx, ready)
}

func (server *Server) run(ctx context.Context, ready func(*url.URL)) (err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if err := server.Validate(); err != nil {
		return err
	}
	website, err := server.Build(ctx)
	if err != nil {
		return err
	}
	interval := server.Watcher.Interval
	server.Watcher = NewWatcher(website)
	if interval != 0 {
		server.Watcher.Interval = interval
	}

	server.LiveReload = true
	if contextCancellationCheckpoint(ctx) != nil {
		return nil
	}
	listener, err := server.Server.Listen()
	if err != nil {
		return err
	}
	server.writeListening()
	errors := make(chan error, 2)
	completed := 0
	go func() { errors <- server.Server.Serve(ctx, listener) }()
	go func() { errors <- server.Watcher.Run(ctx) }()
	defer func() {
		cancel()
		err = drainServerErrors(errors, completed, err)
	}()
	select {
	case err = <-errors:
		completed++
		cancel()
		return err
	default:
	}
	if ready != nil {
		ready(server.readyURL())
	}
	for {
		select {
		case <-server.Watcher.Changes():
			if contextCancellationCheckpoint(ctx) != nil {
				return nil
			}
			website, err = server.Build(ctx)
			if err != nil {
				fmt.Fprintf(os.Stderr, "build failed: %v\n", err)
				continue
			}
			server.Watcher.SetWebsite(website)
			server.Reload()
			server.writeListening()
		case err = <-errors:
			completed++
			cancel()
			return err
		case <-ctx.Done():
			return nil
		}
	}
}

// Build executes one fresh preview build using the server configuration.
func (server *Server) Build(ctx context.Context) (*Website, error) {
	if server.WebsiteURI == nil {
		return nil, fmt.Errorf("server.website_uri is required")
	}
	if server.BuildID == "" {
		return nil, fmt.Errorf("server.build_id is required")
	}
	websiteURI := &href.Href{
		URL: server.WebsiteURI.URL,
	}
	config := server.BuildConfig
	config.Verbose = true
	if server.BaseURL != "" {
		config.BaseURL = server.BaseURL
	} else {
		config.BaseURL = fmt.Sprintf("http://localhost:%d/", server.Port)
	}
	config.WebsiteConfig.Path = websiteURI.Path
	config.ID = server.BuildID
	build := &Build{
		Config: config,
	}
	build.Config.Preview = true
	if err := build.Execute(ctx); err != nil {
		return nil, err
	}
	server.Directory = build.Config.URI.Path
	return build.Website, nil
}

func (server *Server) writeListening() {
	baseURL := server.BaseURL
	if baseURL == "" {
		baseURL = fmt.Sprintf("http://localhost:%d/", server.Port)
	}
	fmt.Fprintln(os.Stdout)
	fmt.Fprintf(os.Stdout, "listening at %s\n\n", baseURL)
}

func (server *Server) readyURL() *url.URL {
	return &url.URL{Scheme: "http", Host: server.Address(), Path: "/"}
}

// Validate verifies the server's build configuration.
func (server *Server) Validate() error {
	if server.WebsiteURI == nil || server.WebsiteURI.Path == "" {
		return fmt.Errorf("server.website_uri is required")
	}
	if server.BuildID == "" {
		return fmt.Errorf("server.build_id is required")
	}
	if server.Port < 1 || server.Port > 65535 {
		return fmt.Errorf("server.port %d is outside the valid range", server.Port)
	}
	if server.Bind == "" {
		return fmt.Errorf("server.bind is required")
	}
	return nil
}

// drainServerErrors waits for all server-owned components and preserves the
// first failure when no earlier result has been selected.
func drainServerErrors(errors <-chan error, completed int, err error) error {
	for completed < 2 {
		componentErr := <-errors
		completed++
		if err == nil && componentErr != nil {
			err = componentErr
		}
	}
	return err
}
