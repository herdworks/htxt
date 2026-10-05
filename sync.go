package htxt

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/herdworks/hsync"
	"github.com/herdworks/htxt/href"
)

type SyncConfigManager struct{}

const DefaultSyncConfigPath = "site.yaml"

var _ hsync.ConfigManager = SyncConfigManager{}

func (SyncConfigManager) DefaultConfigPath() string { return DefaultSyncConfigPath }

func (SyncConfigManager) Read(directory, filename string) (hsync.RepositoryConfig, error) {
	explicitFilename := filename != ""
	filename, err := websiteConfigFilename(directory, filename)
	if err != nil {
		return hsync.RepositoryConfig{}, err
	}
	config, _, err := hsync.ReadConfig(filename)
	if err != nil {
		return hsync.RepositoryConfig{}, err
	}
	if config.Sync != nil && config.Sync.Remote != "" {
		website := &Website{URI: &href.Href{URL: url.URL{Path: config.Path}}}
		if err = website.Load(false, nil, io.Discard); err != nil {
			return hsync.RepositoryConfig{}, err
		}
		effective, err := websiteEffectiveSyncConfig(website, *config.Sync)
		if err != nil {
			return hsync.RepositoryConfig{}, err
		}
		config.Sync = &effective
		if err = config.Sync.Validate(); err != nil {
			return hsync.RepositoryConfig{}, err
		}
	}
	if explicitFilename && directory != "" {
		provided, err := filepath.Abs(directory)
		if err != nil {
			return hsync.RepositoryConfig{}, err
		}
		configured, err := filepath.Abs(config.Directory())
		if err != nil {
			return hsync.RepositoryConfig{}, err
		}
		if provided != configured {
			return hsync.RepositoryConfig{}, fmt.Errorf("config %s belongs to %s, not %s", config.Path, configured, provided)
		}
	}
	return config, nil
}

func (SyncConfigManager) Initialize(directory, filename, remote string, verbose bool) error {
	filename, err := websiteConfigFilename(directory, filename)
	if err != nil {
		return err
	}
	config, raw, err := hsync.ReadConfig(filename)
	if err != nil {
		return err
	}
	if config.Sync == nil {
		config.Sync = &hsync.SyncConfig{}
	}
	config.Sync.Remote = remote
	if verbose {
		website := &Website{URI: &href.Href{URL: url.URL{Path: config.Path}}}
		if err = website.Load(false, nil, io.Discard); err != nil {
			return err
		}
		effective, err := websiteEffectiveSyncConfig(website, *config.Sync)
		if err != nil {
			return err
		}
		config.Sync = &effective
	}
	if err = config.Sync.Validate(); err != nil {
		return err
	}
	var document map[string]any
	switch filepath.Ext(filename) {
	case ".json":
		if err = json.Unmarshal(raw, &document); err != nil {
			return err
		}
		document["sync"] = syncDocument(document["sync"], *config.Sync, verbose)
		raw, err = json.MarshalIndent(document, "", "  ")
	case ".yaml", ".yml":
		raw, err = updateSyncYAML(raw, *config.Sync, verbose)
	default:
		return fmt.Errorf("unsupported site configuration type: %s", filepath.Ext(filename))
	}
	if err != nil {
		return err
	}
	return os.WriteFile(filename, append(raw, '\n'), 0o644)
}

func websiteEffectiveSyncConfig(website *Website, config hsync.SyncConfig) (hsync.SyncConfig, error) {
	var err error
	if len(config.Include.Paths) == 0 && len(config.Include.Mimetypes) == 0 {
		config.Include.Paths, err = websiteSyncIncludePaths(website)
		if err != nil {
			return hsync.SyncConfig{}, err
		}
	}
	if len(config.Ignore.Paths) == 0 && len(config.Ignore.Mimetypes) == 0 {
		config.Ignore.Paths, err = websiteSyncIgnorePaths(website)
		if err != nil {
			return hsync.SyncConfig{}, err
		}
	}
	if config.MaxFileSize == "" && config.MaxFileBytes == 0 {
		config.MaxFileSize = hsync.DefaultMaxFileSize
	}
	return config, nil
}

func websiteConfigFilename(directory, filename string) (string, error) {
	var err error
	if directory == "" {
		if directory, err = os.Getwd(); err != nil {
			return "", err
		}
	}
	if filename == "" {
		filename = DefaultSyncConfigPath
	}
	if !filepath.IsAbs(filename) {
		filename = filepath.Join(directory, filename)
	}
	return filename, nil
}

func websiteSyncIncludePaths(website *Website) ([]string, error) {
	paths := []string{website.URI.Path, website.Theme.URI.Path}
	directories := []string{
		website.ContentDirPath(),
		website.DataDirPath(),
		website.StaticDirPath(),
		website.PluginsDirPath(),
		website.ThemesDirPath(),
	}
	directories = append(directories, website.Theme.Directories()...)
	return syncPaths(website.RootDir(), paths, directories)
}

func websiteSyncIgnorePaths(website *Website) ([]string, error) {
	return syncPaths(website.RootDir(), []string{".DS_Store"}, []string{
		website.BuildsDirPath(),
		website.DeploymentsDirPath(),
	})
}

func syncPaths(root string, files, directories []string) ([]string, error) {
	paths := make(map[string]struct{}, len(files)+len(directories))
	for _, filename := range files {
		path, err := relativeSyncPath(root, filename)
		if err != nil {
			return nil, err
		}
		paths[path] = struct{}{}
	}
	for _, directory := range directories {
		path, err := relativeSyncPath(root, directory)
		if err != nil {
			return nil, err
		}
		paths[path+"/**"] = struct{}{}
	}
	result := make([]string, 0, len(paths))
	for path := range paths {
		result = append(result, path)
	}
	sort.Strings(result)
	return result, nil
}

func relativeSyncPath(root, target string) (string, error) {
	if target == ".DS_Store" {
		return target, nil
	}
	path, err := filepath.Rel(root, target)
	if err != nil {
		return "", err
	}
	path = filepath.ToSlash(path)
	if path == "." || path == ".." || len(path) > 3 && path[:3] == "../" {
		return "", fmt.Errorf("sync path %q escapes website root %q", target, root)
	}
	return path, nil
}

func syncDocument(value any, config hsync.SyncConfig, verbose bool) map[string]any {
	sync, ok := value.(map[string]any)
	if !ok || sync == nil {
		sync = map[string]any{}
	}
	sync["remote"] = config.Remote
	if verbose {
		sync["include"] = config.Include
		sync["ignore"] = config.Ignore
		if config.MaxFileSize != "" {
			sync["max_file_size"] = config.MaxFileSize
			delete(sync, "max_file_bytes")
		} else {
			delete(sync, "max_file_size")
			sync["max_file_bytes"] = config.MaxFileBytes
		}
	}
	return sync
}

func updateSyncYAML(raw []byte, config hsync.SyncConfig, verbose bool) ([]byte, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(raw, &document); err != nil {
		return nil, err
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("site configuration must be a YAML mapping")
	}
	root := document.Content[0]
	key := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "sync"}
	value, err := syncYAMLNode(config, verbose)
	if err != nil {
		return nil, err
	}
	for index := 0; index < len(root.Content); index += 2 {
		if root.Content[index].Value != "sync" {
			continue
		}
		key = root.Content[index]
		if !verbose && root.Content[index+1].Kind == yaml.MappingNode {
			value = root.Content[index+1]
			setYAMLMappingValue(value, "remote", config.Remote)
		}
		root.Content = append(root.Content[:index], root.Content[index+2:]...)
		break
	}
	root.Content = append(root.Content, key, value)
	return yaml.Marshal(&document)
}

func syncYAMLNode(config hsync.SyncConfig, verbose bool) (*yaml.Node, error) {
	if !verbose {
		return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: "remote"},
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: config.Remote},
		}}, nil
	}
	raw, err := yaml.Marshal(config)
	if err != nil {
		return nil, err
	}
	var document yaml.Node
	if err = yaml.Unmarshal(raw, &document); err != nil {
		return nil, err
	}
	return document.Content[0], nil
}

func setYAMLMappingValue(mapping *yaml.Node, key, value string) {
	for index := 0; index < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == key {
			mapping.Content[index+1] = &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
			return
		}
	}
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value},
	)
}
