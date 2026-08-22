package htxt

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/herdworks/hsync"
)

func TestSyncConfigManagerAppliesDefaults(t *testing.T) {
	root := t.TempDir()
	writeSyncTestFile(t, filepath.Join(root, "site.yaml"), `base_url: https://example.com
title: Example
description: Example website
byline:
  name: Example
sync:
  remote: https://example.com
`)
	writeSyncTestFile(t, filepath.Join(root, "theme.json"), `{}`)
	manager := SyncConfigManager{}
	if manager.DefaultConfigPath() != DefaultSyncConfigPath {
		t.Fatalf("DefaultConfigPath() = %q", manager.DefaultConfigPath())
	}
	config, err := manager.Read(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if config.Path != filepath.Join(root, DefaultSyncConfigPath) {
		t.Fatalf("config path = %q", config.Path)
	}
	explicit, err := manager.Read(root, DefaultSyncConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if explicit.Path != config.Path {
		t.Fatalf("explicit config path = %q, want %q", explicit.Path, config.Path)
	}
	_, err = manager.Read(t.TempDir(), config.Path)
	if err == nil || !strings.Contains(err.Error(), "belongs to") {
		t.Fatalf("Read() error = %v", err)
	}
	if config.Sync == nil || config.Sync.MaxFileSize != hsync.DefaultMaxFileSize {
		t.Fatalf("sync = %#v", config.Sync)
	}
	for _, path := range []string{"site.yaml", "theme.json", "content/**", "themes/**", "builds/**", "deployments/**", ".DS_Store"} {
		inInclude := slices.Contains(config.Sync.Include.Paths, path)
		inIgnore := slices.Contains(config.Sync.Ignore.Paths, path)
		if path == "builds/**" || path == "deployments/**" || path == ".DS_Store" {
			if !inIgnore {
				t.Errorf("default ignore missing %q: %v", path, config.Sync.Ignore.Paths)
			}
		} else if !inInclude {
			t.Errorf("default include missing %q: %v", path, config.Sync.Include.Paths)
		}
	}
}

func TestSyncConfigManagerPreservesExplicitFields(t *testing.T) {
	root := t.TempDir()
	writeSyncTestFile(t, filepath.Join(root, "site.yaml"), `base_url: https://example.com
title: Example
description: Example website
byline:
  name: Example
sync:
  remote: https://example.com
  include:
    paths: [custom/**]
  ignore:
    mimetypes: [image/*]
  max_file_bytes: 42
`)
	writeSyncTestFile(t, filepath.Join(root, "theme.json"), `{}`)
	config, err := (SyncConfigManager{}).Read(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := config.Sync.Include.Paths; !slices.Equal(got, []string{"custom/**"}) {
		t.Fatalf("include paths = %v", got)
	}
	if got := config.Sync.Ignore.Mimetypes; !slices.Equal(got, []string{"image/*"}) {
		t.Fatalf("ignore MIME types = %v", got)
	}
	if config.Sync.MaxFileBytes != 42 || config.Sync.MaxFileSize != "" {
		t.Fatalf("size = %#v", config.Sync)
	}
}

func TestSyncConfigManagerAppliesWebsiteDefaultsToExplicitConfigFilename(t *testing.T) {
	root := t.TempDir()
	filename := filepath.Join(root, "hsync.yaml")
	writeSyncTestFile(t, filename, `base_url: https://example.com
title: Example
description: Example website
byline:
  name: Example
sync:
  remote: https://example.com
`)
	writeSyncTestFile(t, filepath.Join(root, "theme.json"), `{}`)
	config, err := (SyncConfigManager{}).Read(root, "hsync.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if config.Path != filename {
		t.Fatalf("config path = %q", config.Path)
	}
	if !slices.Contains(config.Sync.Include.Paths, "content/**") {
		t.Fatalf("include paths = %v", config.Sync.Include.Paths)
	}
}

func TestSyncConfigManagerDefaultsEmptyRules(t *testing.T) {
	root := t.TempDir()
	writeSyncTestFile(t, filepath.Join(root, "site.yaml"), `base_url: https://example.com
title: Example
description: Example website
byline:
  name: Example
sync:
  remote: https://example.com
  include: {}
`)
	writeSyncTestFile(t, filepath.Join(root, "theme.json"), `{}`)
	config, err := (SyncConfigManager{}).Read(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(config.Sync.Include.Paths, "content/**") {
		t.Fatalf("include paths = %v", config.Sync.Include.Paths)
	}
}

func TestSyncConfigManagerUpdatesRemoteAtEndOfYAML(t *testing.T) {
	root := t.TempDir()
	filename := filepath.Join(root, "site.yaml")
	writeSyncTestFile(t, filename, "title: Example\nbefore_sync: preserved\nsync:\n  remote: https://old.example\nafter_sync: preserved\n")
	if err := (SyncConfigManager{}).Initialize(root, "", "https://new.example", false); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	contents := string(raw)
	if !strings.Contains(contents, "title: Example") || !strings.Contains(contents, "https://new.example") {
		t.Fatalf("site config = %s", raw)
	}
	for _, key := range []string{"title:", "before_sync:", "after_sync:", "sync:"} {
		if index := strings.Index(contents, key); index == -1 || (key != "title:" && index < strings.Index(contents, "title:")) {
			t.Fatalf("site config order = %s", raw)
		}
	}
	if strings.LastIndex(contents, "sync:") < strings.Index(contents, "after_sync:") {
		t.Fatalf("sync is not last: %s", raw)
	}
}

func TestSyncConfigManagerWritesVerbosePolicy(t *testing.T) {
	root := t.TempDir()
	filename := filepath.Join(root, "site.yaml")
	writeSyncTestFile(t, filename, "base_url: https://example.com\ntitle: Example\ndescription: Example website\nbyline:\n  name: Example\nsync:\n  remote: https://old.example\n")
	writeSyncTestFile(t, filepath.Join(root, "theme.json"), `{}`)
	if err := (SyncConfigManager{}).Initialize(root, "", "https://new.example", true); err != nil {
		t.Fatal(err)
	}
	config, err := (SyncConfigManager{}).Read(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if config.Sync.Remote != "https://new.example" || !slices.Contains(config.Sync.Include.Paths, "content/**") || config.Sync.MaxFileSize != hsync.DefaultMaxFileSize {
		t.Fatalf("sync = %#v", config.Sync)
	}
}

func writeSyncTestFile(t *testing.T, filename, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
