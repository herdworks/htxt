package hypertexting

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	hsync "github.com/herdworks/hsync"
	hyperproviders "github.com/herdworks/hyperproviders-go"
	hypertemplates "github.com/herdworks/hypertemplates-go"
	htfiles "github.com/herdworks/hypertemplates-go/files"
	httheme "github.com/herdworks/hypertemplates-go/theme"
	htvariable "github.com/herdworks/hypertemplates-go/variable"
	bootstrap "github.com/herdworks/hypertexting-go/bootstrap"
	href "github.com/herdworks/hypertexting-go/href"
)

var (
	websiteSettingsFilePattern *regexp.Regexp = regexp.MustCompile(`^site\.(yaml|yml|json)$`)
)

// TYPES =======================================================================

type Website struct {
	URI *href.Href `json:"uri,omitempty" yaml:"-" jsonschema:"-"` // path to website config file

	CreatedAt    string                                 `json:"created_at,omitempty" yaml:"created_at,omitempty" jsonschema:"format=date-time"` // RFC3339 timestamp
	UpdatedAt    string                                 `json:"updated_at,omitempty" yaml:"updated_at,omitempty" jsonschema:"format=date-time"` // RFC3339 timestamp
	BaseURL      string                                 `json:"base_url" yaml:"base_url" jsonschema:"required,format=uri,pattern=^[A-Za-z][A-Za-z0-9+.-]*://[^/?#]+,description=An absolute URL with a scheme and host."`
	Title        string                                 `json:"title" yaml:"title" jsonschema:"required"`
	Description  string                                 `json:"description" yaml:"description" jsonschema:"required"`
	Byline       Byline                                 `json:"byline,omitempty" yaml:"byline,omitempty" jsonschema:"required"`
	Favicon      WebsiteFavicon                         `json:"favicon,omitempty" yaml:"favicon,omitempty"`
	Links        []HTMLLink                             `json:"links,omitempty" yaml:"links,omitempty"`
	Metadata     []HTMLMeta                             `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Config       WebsiteConfig                          `json:"config" yaml:"config"`
	Sync         *hsync.SyncConfig                      `json:"sync,omitempty" yaml:"sync,omitempty"`
	Providers    hyperproviders.HostingProviders        `json:"providers,omitempty" yaml:"providers,omitempty"`
	Environments map[string]*hyperproviders.Environment `json:"environments,omitempty" yaml:"environments,omitempty"`

	Theme        httheme.Theme                `json:"-" yaml:"-"`
	TemplateData *hypertemplates.TemplateData `json:"-" yaml:"-"`
	GlobalData   hypertemplates.TemplateData  `json:"-" yaml:"-"`
	Pages        Pages                        `json:"-" yaml:"-"`
	Plugins      htvariable.Plugins           `json:"-" yaml:"-"`
	Assets       []string                     `json:"-" yaml:"-"` // paths relative to StaticDirName()
}

func (Website) JSONSchemaTitle() string {
	return "HyperTexting website"
}
func (Website) JSONSchemaName() string    { return "website" }
func (Website) JSONSchemaSummary() string { return "site.yaml website configuration." }

func (Website) JSONSchemaDescription() string {
	return "A website source configuration file. Use site.yaml, site.yml, or site.json."
}

type WebsiteFavicon struct {
	Href string `json:"href" yaml:"href" jsonschema:"format=uri-reference,description=A URI reference for the favicon. It may be absolute or relative."`
	Rel  string `json:"rel,omitempty" yaml:"rel,omitempty"`
	Size string `json:"size,omitempty" yaml:"size,omitempty"`
}

func (WebsiteFavicon) JSONSchemaTitle() string {
	return "Website favicon"
}
func (WebsiteFavicon) JSONSchemaName() string { return "website-favicon" }
func (WebsiteFavicon) JSONSchemaSummary() string {
	return "A favicon link included in the website HTML head."
}

func (WebsiteFavicon) JSONSchemaDescription() string {
	return "A favicon link included in the website HTML head."
}

type WebsiteFavicons []WebsiteFavicon

type WebsiteConfig struct {
	BuildsDirectory      string `json:"builds_dir,omitempty" yaml:"builds_dir,omitempty" jsonschema:"default=builds"`
	ContentDirectory     string `json:"content_dir,omitempty" yaml:"content_dir,omitempty" jsonschema:"default=content"`
	DataDirectory        string `json:"data_dir,omitempty" yaml:"data_dir,omitempty" jsonschema:"default=data"`
	DeploymentsDirectory string `json:"deployments_dir,omitempty" yaml:"deployments_dir,omitempty" jsonschema:"default=deployments"`
	PluginsDirectory     string `json:"plugins_dir,omitempty" yaml:"plugins_dir,omitempty" jsonschema:"default=plugins"`
	StaticDirectory      string `json:"static_dir,omitempty" yaml:"static_dir,omitempty" jsonschema:"default=static"`
	ThemesDirectory      string `json:"themes_dir,omitempty" yaml:"themes_dir,omitempty" jsonschema:"default=themes"`
	ThemePath            string `json:"theme,omitempty" yaml:"theme,omitempty" jsonschema:"default=theme.json"`

	TagLayout       string `json:"tag_layout,omitempty" yaml:"tag_layout,omitempty" jsonschema:"default=default"`
	TagPath         string `json:"tag_path,omitempty" yaml:"tag_path,omitempty" jsonschema:"default=/tags"` // path prefix for tag pages, e.g. "/tags/"
	RefreshInterval int    `json:"refresh_interval,omitempty" yaml:"refresh_interval,omitempty"`

	TidyMode bool                            `json:"tidy_mode,omitempty" yaml:"tidy_mode,omitempty"`
	Markdown *hypertemplates.HyperMarkConfig `json:"markdown,omitempty" yaml:"markdown,omitempty"`
}

func (WebsiteConfig) JSONSchemaTitle() string {
	return "Website configuration"
}
func (WebsiteConfig) JSONSchemaName() string { return "website-config" }
func (WebsiteConfig) JSONSchemaSummary() string {
	return "Optional website paths and rendering settings."
}

func (WebsiteConfig) JSONSchemaDescription() string {
	return "Optional website paths and rendering settings."
}

// METHODS =====================================================================

func (website *Website) Load(verbose bool, configOverrides *WebsiteConfig, w io.Writer) (err error) {
	if w == nil {
		w = os.Stdout
	}

	var fileExt string = filepath.Ext(website.URI.Path)
	var fileBytes []byte
	var fileTemplateData hypertemplates.TemplateData
	if verbose {
		fmt.Fprintf(w, "website \"%s\" load\n", website.URI.String())
	}
	if fileBytes, err = os.ReadFile(website.URI.Path); err != nil {
		return err
	}
	switch fileExt {
	case ".json":
		if err = json.Unmarshal(fileBytes, &website); err != nil {
			return err
		}
	case ".yaml", ".yml":
		if err = yaml.Unmarshal(fileBytes, &website); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported file type: %s", fileExt)
	}
	if fileTemplateData, err = hypertemplates.ParseTemplateDataBytes(fileBytes, fileExt, nil); err != nil {
		return err
	}
	website.TemplateData = &fileTemplateData
	website.ResolveConfig(configOverrides)
	if err = website.ValidateConfig(); err != nil {
		return err
	}
	website.ResolveEnvironments()
	website.Theme.URI = website.ThemeURI()
	if err = website.Theme.Parse(w); err != nil {
		return err
	}
	if err = website.DiscoverAssets(verbose, w); err != nil {
		return err
	}
	if err = website.DiscoverPlugins(verbose, w); err != nil {
		return err
	}
	var assetPublicPaths []string
	if assetPublicPaths, err = AssetPublicPaths(&website.Theme, website, nil); err != nil {
		return err
	}
	website.TemplateData.Set("assets", assetPublicPaths)
	if len(website.CreatedAt) == 0 {
		website.CreatedAt = LibReleaseDate.Format(time.RFC3339)
	}
	if len(website.UpdatedAt) == 0 {
		website.UpdatedAt = LibReleaseDate.Format(time.RFC3339)
	}
	return website.Validate()
}

func (website *Website) Validate() error {
	if website.URI == nil {
		return fmt.Errorf("website URI missing")
	}
	if len(website.BaseURL) == 0 {
		return fmt.Errorf("site.base_url is required")
	}
	baseURL, err := url.Parse(website.BaseURL)
	if err != nil || baseURL.Scheme == "" || baseURL.Host == "" {
		return fmt.Errorf("site.base_url must be an absolute URL with a scheme and host")
	}
	if len(website.Title) == 0 {
		return fmt.Errorf("site.title is required")
	}
	if len(website.Description) == 0 {
		return fmt.Errorf("site.description is required")
	}
	if !website.Byline.Ready() {
		return fmt.Errorf("site.byline is not valid")
	}
	if err := website.ValidateConfig(); err != nil {
		return err
	}
	if website.Sync != nil {
		if err := website.Sync.Validate(); err != nil {
			return err
		}
	}
	if err := website.Theme.Validate(); err != nil {
		return err
	}
	return nil
}

func (website *Website) ValidateConfig() error {
	if err := website.Config.Validate(); err != nil {
		return err
	}
	if err := website.Theme.Config.Validate(); err != nil {
		return err
	}
	return nil
}

func (website *Website) URL() *url.URL {
	u, _ := href.Parse(website.BaseURL) // ignore errors since we've pre-validated the URL in Validate()
	return &u.URL
}

func (website *Website) ResolveEnvironments() {
	if len(website.Environments) == 0 {
		defaultEnvironment := hyperproviders.Environment{"default"}
		website.Environments = map[string]*hyperproviders.Environment{
			"default": &defaultEnvironment,
		}
	}
}

func (website *Website) DeploymentEnvironment(name string) (*hyperproviders.Environment, error) {
	if name == "" {
		return nil, fmt.Errorf("deployment environment name is required")
	}
	env, ok := website.Environments[name]
	if !ok {
		return nil, fmt.Errorf("deployment environment %q not found", name)
	}
	if env == nil {
		return nil, fmt.Errorf("deployment environment %q is empty", name)
	}
	if err := website.validateProviderNames([]string(*env)); err != nil {
		return nil, fmt.Errorf("deployment environment %q: %w", name, err)
	}
	return env, nil
}

func (website *Website) DeploymentProviders(names []string) (hyperproviders.Environment, error) {
	env := hyperproviders.Environment(names)
	if err := website.validateProviderNames(names); err != nil {
		return nil, err
	}
	return env, nil
}

func (website *Website) validateProviderNames(names []string) error {
	if len(names) == 0 {
		return fmt.Errorf("no providers selected")
	}
	for _, name := range names {
		if name == "" {
			return fmt.Errorf("provider name is empty")
		}
		provider, ok := website.Providers[name]
		if !ok {
			return fmt.Errorf("provider %q not found", name)
		}
		if provider == nil {
			return fmt.Errorf("provider %q is empty", name)
		}
	}
	return nil
}

func (website *Website) RootDir() string {
	return filepath.Dir(website.URI.Path)
}

func (website *Website) StaticDirName() string {
	return htfiles.ResourceDirectoryName(website.Config.StaticDirectory, "static")
}

func (website *Website) StaticDirPath() string {
	return filepath.Join(website.RootDir(), website.StaticDirName())
}

func (website *Website) DataDirName() string {
	return htfiles.ResourceDirectoryName(website.Config.DataDirectory, "data")
}

func (website *Website) DataDirPath() string {
	return filepath.Join(website.RootDir(), website.DataDirName())
}

func (website *Website) ContentDirName() string {
	return htfiles.ResourceDirectoryName(website.Config.ContentDirectory, "content")
}

func (website *Website) ContentDirPath() string {
	return filepath.Join(website.RootDir(), website.ContentDirName())
}

func (website *Website) ThemesDirName() string {
	return htfiles.ResourceDirectoryName(website.Config.ThemesDirectory, "themes")
}

func (website *Website) ThemesDirPath() string {
	return filepath.Join(website.RootDir(), website.ThemesDirName())
}

func (website *Website) ThemeURI() (uri *url.URL) {
	var err error
	themePath := website.Config.ThemePath
	if themePath == "" {
		themePath = "theme.json"
	} else if themePath, err = htfiles.CleanRelativePath(themePath); err != nil {
		return nil
	}
	path := filepath.Join(website.RootDir(), themePath)
	if uri, err = url.Parse(path); err != nil {
		return nil
	}
	return uri
}

func (website *Website) BuildsDirName() string {
	return htfiles.ResourceDirectoryName(website.Config.BuildsDirectory, "builds")
}

func (website *Website) BuildsDirPath() string {
	return filepath.Join(website.RootDir(), website.BuildsDirName())
}

func (website *Website) DeploymentsDirName() string {
	return htfiles.ResourceDirectoryName(website.Config.DeploymentsDirectory, "deployments")
}

func (website *Website) DeploymentsDirPath() string {
	return filepath.Join(website.RootDir(), website.DeploymentsDirName())
}

func (website *Website) PluginsDirName() string {
	return htfiles.ResourceDirectoryName(website.Config.PluginsDirectory, "plugins")
}

func (website *Website) PluginsDirPath() string {
	return filepath.Join(website.RootDir(), website.PluginsDirName())
}

// RelativePath returns a path relative to the website root directory.
// If the relative path cannot be determined, it returns the original path.
func (website *Website) RelativePath(path string) (relPath string) {
	var basePath string
	var err error
	if basePath, err = filepath.Abs(website.RootDir()); err != nil {
		return path
	}
	if relPath, err = filepath.Rel(basePath, path); err != nil {
		return path
	}
	return relPath
}

func (website *Website) ResourcePath(root string, relPath string) (string, error) {
	return htfiles.LocalPath(root, relPath)
}

func (website *Website) AssetPublicPath(relPath string) (string, error) {
	relPath, err := htfiles.CleanRelativePath(relPath)
	if err != nil {
		return "", err
	}
	return "/" + relPath, nil
}

func (website *Website) AssetBuildPath(build *Build, relPath string) (string, error) {
	return build.AbsolutePath(relPath)
}

func (website *Website) ResolveConfig(overrides *WebsiteConfig) {
	website.Config.Resolve(overrides)
	website.Theme.ResolveConfig()
}

func (website *Website) ResolveURI() (err error) {
	var files []os.DirEntry
	if website.URI == nil {
		return fmt.Errorf("website.uri is required")
	}
	if len(website.URI.Path) == 0 {
		return fmt.Errorf("website.uri must contain a path")
	}
	if websiteSettingsFilePattern.MatchString(filepath.Base(website.URI.Path)) {
		return nil // early return
	}
	if files, err = os.ReadDir(website.URI.Path); err != nil {
		return err
	}
	for _, file := range files {
		if !websiteSettingsFilePattern.MatchString(file.Name()) {
			continue // skip
		}
		website.URI.Path = filepath.Join(website.URI.Path, file.Name()) // update the URI
		return nil
	}
	return fmt.Errorf("no website settings file found at path: %s", website.URI.Path)
}

func (website *Website) ResolveHref(href string) (*url.URL, error) {
	u, err := url.Parse(href)
	if err != nil {
		return nil, err
	}
	return website.URL().ResolveReference(u), nil
}

// Website.CreatePackage()
//
// Persists new website to disk by creating the website directory and writing the config file.
func (website *Website) CreatePackage(w io.Writer) (err error) {
	if w == nil {
		w = os.Stdout
	}

	var embedded fs.FS = bootstrap.Website

	website.Theme.URI = website.ThemeURI()
	website.Config.ThemePath = "theme.json" // always configure theme_dir when creating new websites
	website.Config.TidyMode = true          // always enable tidy mode when creating new websites

	// ensure website directory
	dirpath := filepath.Dir(website.URI.Path)
	err = os.MkdirAll(dirpath, 0755)
	if err != nil {
		return err
	}

	// write config file & create package
	if err = website.WriteFile(); err != nil {
		return err
	}
	if err = fs.WalkDir(embedded, "website", func(srcPath string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		var srcFile fs.File
		var dstPath string = filepath.Join(website.RootDir(), strings.TrimPrefix(srcPath, "website/"))
		var dstFile *os.File
		if entry.IsDir() {
			return nil // skip directories
		}
		if err = os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
			return err // ensure destination directory exists
		}
		if srcFile, err = embedded.Open(srcPath); err != nil {
			return err
		}
		defer srcFile.Close()
		if dstFile, err = os.Create(dstPath); err != nil {
			return err
		}
		defer dstFile.Close()
		if _, err = io.Copy(dstFile, srcFile); err != nil {
			return err // copy file contents
		}
		fmt.Fprintf(w, "file %s created\n", dstPath)
		return nil
	}); err != nil {
		return err
	}
	fmt.Fprintf(w, "website \"%s\" created\n", website.URI.Path)

	return nil
}

func (website *Website) WriteFile() (err error) {
	var filebytes []byte
	var fileLength int
	var file *os.File
	switch path.Ext(website.BaseURL) {
	case ".json":
		filebytes, err = json.MarshalIndent(website, "", "\t")
	case ".yaml", ".yml":
		filebytes, err = yaml.Marshal(website)
		filebytes = append([]byte("---\n"), filebytes...)
	default:
		filebytes, err = yaml.Marshal(website)
		filebytes = append([]byte("---\n"), filebytes...)
	}
	if err != nil {
		return err // serialization error
	}
	file, err = os.OpenFile(website.URI.Path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err == nil {
		defer file.Close()
	} else {
		return err
	}
	if fileLength, err = file.Write(filebytes); err != nil {
		return err
	}
	if fileLength != len(filebytes) {
		return fmt.Errorf("expected to write %v bytes, but wrote %v", len(filebytes), fileLength)
	}
	return nil
}

func (website *Website) DiscoverAssets(verbose bool, w io.Writer) error {
	if w == nil {
		w = os.Stdout
	}

	if err := website.Theme.DiscoverAssets(); err != nil {
		return err
	}
	if website.StaticDirPath() != website.Theme.StaticDirPath() {
		assets, err := htfiles.List(website.StaticDirPath(), true)
		if err != nil {
			return err
		}
		website.Assets = assets
	} else {
		website.Assets = []string{}
	}
	if !verbose {
		return nil
	}
	for _, asset := range website.Theme.Assets {
		if srcPath, err := website.ResourcePath(website.Theme.StaticDirPath(), asset); err == nil {
			fmt.Fprintf(w, "asset \"%s\" discovered\n", website.RelativePath(srcPath))
		}
	}
	for _, asset := range website.Assets {
		if srcPath, err := website.ResourcePath(website.StaticDirPath(), asset); err == nil {
			fmt.Fprintf(w, "asset \"%s\" discovered\n", website.RelativePath(srcPath))
		}
	}
	return nil
}

func (website *Website) DiscoverPlugins(verbose bool, w io.Writer) error {
	if w == nil {
		w = os.Stdout
	}

	if err := website.Theme.DiscoverPlugins(); err != nil {
		return err
	}
	if website.PluginsDirPath() != website.Theme.PluginsDirPath() {
		pluginPaths, err := htfiles.List(website.PluginsDirPath(), true)
		if err != nil {
			return err
		}
		paths, err := htfiles.LocalPaths(website.PluginsDirPath(), pluginPaths)
		if err != nil {
			return err
		}
		website.Plugins, err = htvariable.LoadPlugins(paths)
		if err != nil {
			return err
		}
	} else {
		website.Plugins = htvariable.Plugins{}
	}
	if !verbose {
		return nil
	}
	for _, plugin := range website.Theme.Plugins {
		fmt.Fprintf(w, "plugin \"%s\" discovered\n", website.RelativePath(plugin.Path))
	}
	for _, plugin := range website.Plugins {
		fmt.Fprintf(w, "plugin \"%s\" discovered\n", website.RelativePath(plugin.Path))
	}
	return nil
}

func (website *Website) CopyAsset(srcPath string, dstPath string, w io.Writer) (err error) {
	if w == nil {
		w = os.Stdout
	}

	var srcFile *os.File
	var dstFile *os.File
	dstPath = filepath.Join(website.StaticDirPath(), dstPath)
	fmt.Fprintf(w, "copying %s to %s\n", srcPath, dstPath)
	if err = os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		return err
	}
	if srcFile, err = os.Open(srcPath); err != nil {
		return err
	}
	defer srcFile.Close()
	if dstFile, err = os.Create(dstPath); err != nil {
		return err
	}
	defer dstFile.Close()
	if _, err = io.Copy(dstFile, srcFile); err != nil {
		return err
	}
	return nil
}

func (website *Website) GetAssetURI(assetPath string, w io.Writer) *url.URL {
	if w == nil {
		w = os.Stdout
	}

	var uri *url.URL
	var uris []*url.URL = []*url.URL{}
	var err error
	fmt.Fprintf(w, "asset \"%s\" lookup\n", assetPath)
	for _, assetDir := range []string{website.ContentDirPath(), website.StaticDirPath(), website.Theme.StaticDirPath()} {
		var u *url.URL
		if u, err = url.Parse(filepath.Join(assetDir, assetPath)); err != nil {
			continue
		}
		uris = append(uris, u)
	}
	for _, u := range uris {
		if _, err = os.Stat(u.Path); err == nil {
			uri = u
			break // use the first file that exists
		}
	}
	if uri != nil {
		uri.Path = website.RelativePath(uri.Path)
		fmt.Fprintf(w, "asset \"%s\" candidate \"%s\" found\n", assetPath, uri.Path)
	}
	return uri
}

func (website *Website) ContentTypes() (types ContentTypes, err error) {
	types = ContentTypes{}
	contentTypePaths, err := htfiles.List(website.Theme.ContentTypesDirPath(), false)
	if err != nil {
		return nil, err
	}
	for _, path := range contentTypePaths {
		absPath, err := htfiles.LocalPath(website.Theme.ContentTypesDirPath(), path)
		if err != nil {
			return nil, err
		}
		var contenttype *ContentType = &ContentType{URI: &href.Href{URL: url.URL{Path: absPath}}}
		if err = contenttype.Load(); err != nil {
			return nil, err
		}
		types = append(types, contenttype)
	}
	return types, nil
}
func (website *Website) GetContentTypeByName(name string, verbose bool, w io.Writer) (contenttype *ContentType) {
	if w == nil {
		w = os.Stdout
	}

	contenttypes, err := website.ContentTypes()
	if err != nil {
		fmt.Fprintln(w, err)
		contenttypes = ContentTypes{}
	}
	for _, c := range contenttypes {
		if strings.EqualFold(c.Metadata.Name, name) {
			if verbose {
				fmt.Fprintf(w, "content-type \"%s\" loading\n", c.Metadata.Name)
			}
			return c
		}
	}
	return nil
}

func (website *Website) ListPages() (Pages, error) {
	var err error
	if website.Pages, err = GetPages(website, website.ContentDirPath()); err != nil {
		return nil, err
	}
	return website.Pages, nil
}

// Recursively walk website.ThemesDirPath() looking for theme.json config files;
// for each config, marshal the file into a Theme and return []Theme.
func (website *Website) ListThemes(w io.Writer) (themes []httheme.Theme, err error) {
	if w == nil {
		w = os.Stdout
	}

	var discovered []httheme.Theme
	// include the current theme, which could be a "local" theme (installed outside of the themes directory)
	if website.ThemeURI() != nil {
		discovered = append(discovered, httheme.Theme{
			URI:    &url.URL{Path: website.ThemeURI().Path},
			Config: website.Theme.Config,
		})
	}
	err = filepath.WalkDir(website.ThemesDirPath(), func(themePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if !strings.EqualFold(entry.Name(), "theme.json") {
			return nil
		}
		if website.ThemeURI() != nil && filepath.Clean(website.ThemeURI().Path) == filepath.Clean(themePath) {
			return nil // already included above
		}
		theme := httheme.Theme{URI: &url.URL{Path: themePath}}
		discovered = append(discovered, theme)
		return nil
	})
	if err != nil {
		return nil, err
	}
	for theme := range discovered {
		if err = discovered[theme].Parse(w); err != nil {
			fmt.Fprintf(w, "warning: %s\n", err)
			continue
		}
		discovered[theme].ResolveConfig()
		themes = append(themes, discovered[theme])
	}
	return themes, nil
}

// ============================================================================
// Helper types, methods, and functions
// ============================================================================

func (config *WebsiteConfig) Resolve(overrides *WebsiteConfig) {
	if overrides != nil {
		if len(overrides.BuildsDirectory) > 0 {
			config.BuildsDirectory = overrides.BuildsDirectory
		}
		if len(overrides.ContentDirectory) > 0 {
			config.ContentDirectory = overrides.ContentDirectory
		}
		if len(overrides.DataDirectory) > 0 {
			config.DataDirectory = overrides.DataDirectory
		}
		if len(overrides.DeploymentsDirectory) > 0 {
			config.DeploymentsDirectory = overrides.DeploymentsDirectory
		}
		if len(overrides.PluginsDirectory) > 0 {
			config.PluginsDirectory = overrides.PluginsDirectory
		}
		if len(overrides.StaticDirectory) > 0 {
			config.StaticDirectory = overrides.StaticDirectory
		}
		if len(overrides.ThemesDirectory) > 0 {
			config.ThemesDirectory = overrides.ThemesDirectory
		}
		if len(overrides.ThemePath) > 0 {
			config.ThemePath = overrides.ThemePath
		}
		if len(overrides.TagLayout) > 0 {
			config.TagLayout = overrides.TagLayout
		}
		if len(overrides.TagPath) > 0 {
			config.TagPath = overrides.TagPath
		}
	}
	if len(config.BuildsDirectory) == 0 {
		config.BuildsDirectory = "builds"
	}
	if len(config.ContentDirectory) == 0 {
		config.ContentDirectory = "content"
	}
	if len(config.DataDirectory) == 0 {
		config.DataDirectory = "data"
	}
	if len(config.DeploymentsDirectory) == 0 {
		config.DeploymentsDirectory = "deployments"
	}
	if len(config.PluginsDirectory) == 0 {
		config.PluginsDirectory = "plugins"
	}
	if len(config.StaticDirectory) == 0 {
		config.StaticDirectory = "static"
	}
	if len(config.ThemesDirectory) == 0 {
		config.ThemesDirectory = "themes"
	}
	if len(config.ThemePath) == 0 {
		config.ThemePath = "theme.json"
	}
	if len(config.TagLayout) == 0 {
		config.TagLayout = "default"
	}
	if len(config.TagPath) == 0 {
		config.TagPath = "/tags"
	}
	config.Markdown.Resolve()
}

func (config *WebsiteConfig) Validate() error {
	fields := []struct {
		name string
		path string
	}{
		{name: "site.config.builds_dir", path: config.BuildsDirectory},
		{name: "site.config.content_dir", path: config.ContentDirectory},
		{name: "site.config.data_dir", path: config.DataDirectory},
		{name: "site.config.deployments_dir", path: config.DeploymentsDirectory},
		{name: "site.config.plugins_dir", path: config.PluginsDirectory},
		{name: "site.config.static_dir", path: config.StaticDirectory},
		{name: "site.config.themes_dir", path: config.ThemesDirectory},
		{name: "site.config.theme", path: config.ThemePath},
	}
	for _, field := range fields {
		if err := htfiles.ValidateRelativePath(field.name, field.path); err != nil {
			return err
		}
	}
	return nil
}
