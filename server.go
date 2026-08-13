package htxt

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

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
func (server *Server) Run(ctx context.Context) error {
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
	listener, err := server.Server.Listen()
	if err != nil {
		return err
	}
	server.writeListening()
	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- server.Server.Serve(ctx, listener)
	}()
	watcherErrors := make(chan error, 1)
	go func() {
		watcherErrors <- server.Watcher.Run(ctx)
	}()
	for {
		select {
		case <-server.Watcher.Changes():
			website, err = server.Build(ctx)
			if err != nil {
				fmt.Fprintf(os.Stderr, "build failed: %v\n", err)
				continue
			}
			server.Watcher.SetWebsite(website)
			server.Reload()
			server.writeListening()
		case err = <-watcherErrors:
			return err
		case err = <-serverErrors:
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
	website := &Website{
		URI: websiteURI,
	}
	if err := website.Load(false, &config.WebsiteConfig, nil); err != nil {
		return nil, err
	}
	server.Directory = filepath.Join(website.BuildsDirPath(), server.BuildID)
	build := &Build{
		URI: &url.URL{
			Path: server.Directory,
		},
		ID:      server.BuildID,
		Website: website,
		Config:  config,
		Preview: true,
	}
	if err := build.Execute(ctx); err != nil {
		return nil, err
	}
	return website, nil
}

func (server *Server) writeListening() {
	baseURL := server.BaseURL
	if baseURL == "" {
		baseURL = fmt.Sprintf("http://localhost:%d/", server.Port)
	}
	fmt.Fprintln(os.Stdout)
	fmt.Fprintf(os.Stdout, "listening at %s\n\n", baseURL)
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
