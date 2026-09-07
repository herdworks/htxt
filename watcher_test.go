package htxt

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatcherEmitsAfterChangesSettle(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "content", "index.html")
	writeBuildSelectionFile(t, path, "first")
	website := &Website{URI: mustHref(t, filepath.Join(root, "site.yaml"))}
	writeBuildSelectionFile(t, website.URI.Path, `base_url: https://example.com/
title: Test Site
description: Test description
byline:
  name: Test Author
config:
  theme: theme.json
`)
	writeBuildSelectionFile(t, filepath.Join(root, "theme.json"), "{}")
	if err := website.Load(false, nil, nil); err != nil {
		t.Fatal(err)
	}
	watcher := NewWatcher(website)
	watcher.Interval = 20 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errors := make(chan error, 1)
	go func() {
		errors <- watcher.Run(ctx)
	}()

	time.Sleep(2 * watcherPollInterval)
	if err := os.WriteFile(path, []byte("second"), 0644); err != nil {
		t.Fatal(err)
	}
	select {
	case <-watcher.Changes():
	case err := <-errors:
		t.Fatalf("Run() error = %v", err)
	case <-time.After(time.Second):
		t.Fatal("watcher did not emit a change")
	}
}

func TestWatcherExcludesBuildDirectory(t *testing.T) {
	root := t.TempDir()
	website := &Website{URI: mustHref(t, filepath.Join(root, "site.yaml"))}
	writeBuildSelectionFile(t, website.URI.Path, `base_url: https://example.com/
title: Test Site
description: Test description
byline:
  name: Test Author
config:
  theme: theme.json
`)
	writeBuildSelectionFile(t, filepath.Join(root, "theme.json"), "{}")
	path := filepath.Join(root, "builds", "development", "index.html")
	writeBuildSelectionFile(t, path, "first")
	writeBuildSelectionFile(t, filepath.Join(root, "deployments", "development.json"), "{}")
	if err := website.Load(false, nil, nil); err != nil {
		t.Fatal(err)
	}
	watcher := NewWatcher(website)
	watcher.Interval = 20 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errors := make(chan error, 1)
	go func() {
		errors <- watcher.Run(ctx)
	}()

	time.Sleep(2 * watcherPollInterval)
	writeBuildSelectionFile(t, path, "second")
	writeBuildSelectionFile(t, filepath.Join(root, "deployments", "development.json"), `{"changed":true}`)
	select {
	case <-watcher.Changes():
		t.Fatal("watcher emitted a change from the excluded build directory")
	case err := <-errors:
		t.Fatalf("Run() error = %v", err)
	case <-time.After(3 * watcherPollInterval):
	}
}

func TestWatcherIncludesExplicitDraftsDirectory(t *testing.T) {
	root := t.TempDir()
	website := &Website{URI: mustHref(t, filepath.Join(root, "site.yaml"))}
	writeBuildSelectionFile(t, website.URI.Path, `base_url: https://example.com/
title: Test Site
description: Test description
byline:
  name: Test Author
config:
  theme: theme.json
`)
	writeBuildSelectionFile(t, filepath.Join(root, "theme.json"), "{}")
	contentDirectory := filepath.Join(root, "content")
	draftsDirectory := filepath.Join(root, "drafts")
	draftPath := filepath.Join(draftsDirectory, "index.md")
	writeBuildSelectionFile(t, filepath.Join(contentDirectory, "index.html"), "first")
	writeBuildSelectionFile(t, draftPath, "first")
	if err := website.Load(false, nil, nil); err != nil {
		t.Fatal(err)
	}
	watcher := NewWatcher(website, contentDirectory, draftsDirectory)
	watcher.Interval = 20 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errors := make(chan error, 1)
	go func() {
		errors <- watcher.Run(ctx)
	}()

	time.Sleep(2 * watcherPollInterval)
	if err := os.WriteFile(draftPath, []byte("second"), 0644); err != nil {
		t.Fatal(err)
	}
	select {
	case <-watcher.Changes():
	case err := <-errors:
		t.Fatalf("Run() error = %v", err)
	case <-time.After(time.Second):
		t.Fatal("watcher did not emit a change for a draft file")
	}
}
