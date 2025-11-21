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

	"gopkg.in/yaml.v3"

	hypergraph "github.com/herdworks/hypergraph-go"
	hyperproviders "github.com/herdworks/hyperproviders-go"
	hypertemplates "github.com/herdworks/hypertemplates-go"
	bootstrap "github.com/herdworks/hypertemplates-go/bootstrap"
)

var (
	websiteSettingsFilePattern *regexp.Regexp = regexp.MustCompile(`^site\.(yaml|yml|json)$`)
)

type Website struct {
	URI *url.URL `json:"-" yaml:"-"` // path to website config file
	Loaded bool `json:"-" yaml:"-"` // indicates if the website has been loaded from disk

	CreatedAt string `json:"created_at,omitempty" yaml:"created_at,omitempty"` // ISO 8601 timestamp
	UpdatedAt string `json:"updated_at,omitempty" yaml:"updated_at,omitempty"` // ISO 8601 timestamp
	BaseURL string `json:"base_url" yaml:"base_url"` // TODO: rename to baseHref or change type to *url.URL
	Title string `json:"title" yaml:"title"`
	Description string `json:"description" yaml:"description"`
	Author *hypergraph.Entity `json:"author,omitempty" yaml:"author,omitempty"`
	Favicon WebsiteFavicon `json:"favicon,omitempty" yaml:"favicon,omitempty"`
	Links []HTMLLink`json:"links,omitempty" yaml:"links,omitempty"`
	Metadata []HTMLMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Config WebsiteConfig `json:"config" yaml:"config"`
	Provider hyperproviders.HyperProviderConfig `json:"provider,omitempty" yaml:"provider,omitempty"`

	Theme hypertemplates.HyperTheme `json:"-" yaml:"-"`
	TemplateData *hypertemplates.TemplateData `json:"-" yaml:"-"`
	GlobalData hypertemplates.TemplateData `json:"-" yaml:"-"`
	Pages Pages `json:"-" yaml:"-"`
	Drafts Pages `json:"-" yaml:"-"`
	Assets hypertemplates.HyperFiles `json:"-" yaml:"-"`
}

func (website *Website) Load(verbose bool, configOverrides *WebsiteConfig) (err error) {
	var fileExt          string = filepath.Ext(website.URI.Path)
	var fileBytes        []byte
	var fileTemplateData hypertemplates.TemplateData
	if website.Loaded { return nil } // already loaded
	if verbose { fmt.Printf("website \"%s\" load\n", website.URI.String()) }
	if fileBytes, err = os.ReadFile(website.URI.Path); err != nil { return err }
	switch fileExt {
		case ".json": if err = json.Unmarshal(fileBytes, &website); err != nil { return err }
		case ".yaml", ".yml": if err = yaml.Unmarshal(fileBytes, &website); err != nil { return err }
		default: return fmt.Errorf("unsupported file type: %s", fileExt)
	}
	if fileTemplateData,err = hypertemplates.ParseTemplateDataBytes(fileBytes, fileExt, nil); err != nil { return err }
	website.TemplateData = &fileTemplateData
	website.ResolveConfig(configOverrides)
	website.Theme.URI = website.ThemeURI()
	if err = website.Theme.Parse(); err != nil { return err }
	website.DiscoverAssets(verbose)
	if len(website.CreatedAt) == 0 { website.CreatedAt = ReleaseDate() }
	if len(website.UpdatedAt) == 0 { website.UpdatedAt = ReleaseDate() }
	website.Loaded = true // mark website as loaded
	return website.Validate()
}

func (website *Website) Validate() (error) {
	if website.URI == nil { return fmt.Errorf("website URI missing") }
	if len(website.BaseURL) == 0 { return fmt.Errorf("site.base_url is required") }
	if _,err := url.Parse(website.BaseURL); err != nil { return fmt.Errorf("site.base_url is not a valid URL") }
	if len(website.Title) == 0 { return fmt.Errorf("site.title is required") }
	if len(website.Description) == 0 { return fmt.Errorf("site.description is required") }
	if website.Author == nil { return fmt.Errorf("site.author is required") }
	if err := website.Theme.Validate(); err != nil { return err }
	if !website.Author.Ready() { return fmt.Errorf("site.author is not valid") }
	return nil
}

func (website *Website) URL() (*url.URL) {
	u,_ := url.Parse(website.BaseURL) // ignore errors since we've pre-validated the URL in Validate()
	return u
}

func (website *Website) RootDir() (string) {
	return filepath.Dir(website.URI.Path)
}

func (website *Website) BuildsDir() (string) {
	if website.Config.BuildsDirectory == "" { website.Config.BuildsDirectory = "builds" }
	return filepath.Join(website.RootDir(), website.Config.BuildsDirectory)
}

func (website *Website) ContentDir() (string) {
	if website.Config.ContentDirectory == "" { website.Config.ContentDirectory = "content" }
	return filepath.Join(website.RootDir(), website.Config.ContentDirectory)
}

func (website *Website) DeploymentsDir() (string) {
	if website.Config.DeploymentsDirectory == "" { website.Config.DeploymentsDirectory = "deployments" }
	return filepath.Join(website.RootDir(), website.Config.DeploymentsDirectory)
}

func (website *Website) DataDir() (string) {
	if website.Config.DataDirectory == "" { website.Config.DataDirectory = "data" }
	return filepath.Join(website.RootDir(), website.Config.DataDirectory)
}

func (website *Website) DraftsDir() (string) {
	if website.Config.DraftsDirectory == "" { website.Config.DraftsDirectory = "drafts" }
	return filepath.Join(website.RootDir(), website.Config.DraftsDirectory)
}

func (website *Website) StaticDir() (string) {
	if website.Config.StaticDirectory == "" { website.Config.StaticDirectory = "static" }
	return filepath.Join(website.RootDir(), website.Config.StaticDirectory)
}

func (website *Website) ThemeURI() (uri *url.URL) {
	var err error
	if website.Config.ThemeSettings == "" { website.Config.ThemeSettings = "theme.json" }
	path := filepath.Join(website.RootDir(), website.Config.ThemeSettings)
	if uri,err = url.Parse(path); err != nil { return nil }
	return uri
}

// Website.RelativePath(path string)
//
// Website.RelativePath returns a relative path from the website root directory to the source file path.
// If the relative path cannot be determined, it returns the original path.
func (website *Website) RelativePath(path string) (relPath string) {
	var basePath string
	var err error
	if basePath,err = filepath.Abs(website.RootDir()); err != nil { return path }
	if relPath,err = filepath.Rel(basePath, path); err != nil { return path }
	return relPath
}

func (website *Website) ResolveConfig(overrides *WebsiteConfig) {
	website.Config.Resolve(overrides)
	website.Theme.ResolveConfig()
}

func (website *Website) ResolveURI() (err error) {
	var files []os.DirEntry
	if website.URI == nil { return fmt.Errorf("website.uri is required") }
	if len(website.URI.Path) == 0 { return fmt.Errorf("website.uri must contain a path") }
	if websiteSettingsFilePattern.MatchString(filepath.Base(website.URI.Path)) { return nil } // early return
	if files, err = os.ReadDir(website.URI.Path); err != nil { return err }
	for _, file := range files {
		if !websiteSettingsFilePattern.MatchString(file.Name()) { continue } // skip
		website.URI.Path = filepath.Join(website.URI.Path, file.Name()) // update the URI
		return nil
	}
	return fmt.Errorf("no website settings file found at path: %s", website.URI.Path)
}

func (website *Website) ResolveHref(href string) (*url.URL, error) {
	u,err := url.Parse(href)
	if err != nil { return nil, err }
	if u.IsAbs() { return u, nil }
	link := website.URL()
	link.Path = filepath.Join(link.Path, u.Path)
	return link, nil
}

// Website.CreatePackage()
//
// Persists new website to disk by creating the website directory and writing the config file.
func (website *Website) CreatePackage() (err error) {
	var embedded fs.FS = bootstrap.Website

	website.Theme.URI = website.ThemeURI()
	website.Config.ThemeSettings = "theme.json" // always configure theme_dir when creating new websites
	website.Config.TidyMode = true // always enable tidy mode when creating new websites

	// ensure website directory
	dirpath := filepath.Dir(website.URI.Path)
	err = os.MkdirAll(dirpath, 0755); if err != nil { return err }

	// write config file & create package
	if err = website.WriteFile(); err != nil { return err }
	if err = fs.WalkDir(embedded, "website", func(srcPath string, entry fs.DirEntry, err error) error {
	if err != nil { return err }
		var srcFile fs.File
		var dstPath string = filepath.Join(website.RootDir(), strings.TrimPrefix(srcPath, "website/"))
		var dstFile *os.File
		if entry.IsDir() { return nil } // skip directories
		if err = os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil { return err } // ensure destination directory exists
		if srcFile, err = embedded.Open(srcPath); err != nil { return err }; defer srcFile.Close()
		if dstFile, err = os.Create(dstPath); err != nil { return err }; defer dstFile.Close()
		if _, err = io.Copy(dstFile, srcFile); err != nil { return err } // copy file contents
		fmt.Printf("file %s created\n", dstPath)
		return nil
	}); err != nil { return err }
	fmt.Printf("website \"%s\" created\n", website.URI.Path)

	return nil
}

func (website *Website) WriteFile() (err error) {
	var filebytes []byte
	var fileLength int
	var file *os.File
	switch path.Ext(website.BaseURL) {
		case ".json": 
			filebytes,err = json.MarshalIndent(website, "", "\t")
		case ".yaml", ".yml":
			filebytes,err = yaml.Marshal(website)
			filebytes = append([]byte("---\n"), filebytes...)
		default:
			filebytes,err = yaml.Marshal(website)
			filebytes = append([]byte("---\n"), filebytes...)
	}
	if err != nil { return err } // serialization error
	file, err = os.OpenFile(website.URI.Path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err == nil { defer file.Close() } else { return err }
	if fileLength, err = file.Write(filebytes); err != nil { return err }
	if fileLength != len(filebytes) { return fmt.Errorf("expected to write %v bytes, but wrote %v", len(filebytes), fileLength) }
	return nil
}

func (website *Website) DiscoverAssets(verbose bool) {
	website.Assets = website.Theme.Assets()
	if website.StaticDir() != website.Theme.StaticDir() {
		website.Assets = append(website.Assets, hypertemplates.GetFiles(website.StaticDir(), true)...)
	}
	if !verbose { return }
	for _,asset := range website.Assets {
		fmt.Printf("asset \"%s\" discovered\n", website.RelativePath(asset.URI.Path))
	}
}

func (website *Website) CopyAsset(srcPath string, dstPath string) (err error) {
	var srcFile *os.File
	var dstFile *os.File
	dstPath = filepath.Join(website.StaticDir(), dstPath)
	fmt.Printf("copying %s to %s\n", srcPath, dstPath)
	if err = os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil { return err }
	if srcFile,err = os.Open(srcPath); err != nil { return err }; defer srcFile.Close()
	if dstFile,err = os.Create(dstPath); err != nil { return err }; defer dstFile.Close()
	if _,err = io.Copy(dstFile, srcFile); err != nil { return err }
	return nil
}

func (website *Website) GetAssetURI(assetPath string) (*url.URL) {
	var uri *url.URL
	var uris []*url.URL = []*url.URL{}
	var err error
	fmt.Printf("asset \"%s\" lookup\n", assetPath)
	for _,assetDir := range []string{ website.ContentDir(), website.StaticDir(), website.Theme.StaticDir() } {
		var u *url.URL
		if u,err = url.Parse(filepath.Join(assetDir,assetPath)); err != nil { continue }
		uris = append(uris, u)
	}
	for _,u := range uris {
		if _,err = os.Stat(u.Path); err == nil { uri = u; break } // use the first file that exists
	}
	if uri != nil { 
		uri.Path = website.RelativePath(uri.Path)
		fmt.Printf("asset \"%s\" candidate \"%s\" found\n", assetPath, uri.Path)
	}
	return uri
}

func (website *Website) ContentTypes() (types ContentTypes, err error) {
	types = ContentTypes{}
	contentTypePaths := hypertemplates.GetPaths(website.Theme.ContentTypesDir(), false)
	for _,path := range contentTypePaths {
		var contenttype *ContentType = &ContentType{ URI: &url.URL{ Path: path } }
		if err = contenttype.Load(); err != nil { return nil, err }
		types = append(types, contenttype)
	}
	return types, nil
}
func (website *Website) GetContentTypeByName(name string, verbose bool) (contenttype *ContentType) {
	contenttypes, err := website.ContentTypes()
	if err != nil { 
		fmt.Println(err)
		contenttypes = ContentTypes{}
	}
	for _,c := range contenttypes {
		if strings.EqualFold(c.Metadata.Name, name) { 
			if verbose { fmt.Printf("content-type \"%s\" loading\n", c.Metadata.Name) }
			return c
		}
	}
	return nil
}

func (website *Website) ListPages(filter string) (pages Pages, err error) {
	if website.Pages,err = GetPages(website, website.ContentDir()); err != nil { return nil, err }
	if website.Drafts,err = GetPages(website, website.DraftsDir()); err != nil { return nil, err }
	switch filter {
		case "pages": pages = append(pages, website.Pages...)
		case "drafts": pages = append(pages, website.Drafts...)
		case "all": fallthrough
		default:
			pages = append(pages, website.Pages...)
			pages = append(pages, website.Drafts...)
	}
	return pages, nil
}

// ============================================================================
// Helper types, methods, and functions
// ============================================================================

type WebsiteFavicon struct {
	Href string `json:"href" yaml:"href"`
	Rel  string `json:"rel,omitempty" yaml:"rel,omitempty"`
	Size string `json:"size,omitempty" yaml:"size,omitempty"`
}
type WebsiteFavicons []WebsiteFavicon

type WebsiteConfig struct {
	BuildsDirectory string `json:"builds_dir,omitempty" yaml:"builds_dir,omitempty"`
	ContentDirectory string `json:"content_dir,omitempty" yaml:"content_dir,omitempty"`
	DataDirectory string `json:"data_dir,omitempty" yaml:"data_dir,omitempty"`
	DeploymentsDirectory string `json:"deployments_dir,omitempty" yaml:"deployments_dir,omitempty"`
	DraftsDirectory string `json:"drafts_dir,omitempty" yaml:"drafts_dir,omitempty"`
	StaticDirectory string `json:"static_dir,omitempty" yaml:"static_dir,omitempty"`
	ThemeSettings string `json:"theme,omitempty" yaml:"theme,omitempty"`

	TagLayout string `json:"tag_layout,omitempty" yaml:"tag_layout,omitempty"`
	TagPath string `json:"tag_path,omitempty" yaml:"tag_path,omitempty"` // path prefix for tag pages, e.g. "/tags/"
	RefreshInterval int `json:"refresh_interval,omitempty" yaml:"refresh_interval,omitempty"`

	TidyMode bool `json:"tidy_mode,omitempty" yaml:"tidy_mode,omitempty"`
	Markdown *hypertemplates.HyperMarkConfig `json:"markdown,omitempty" yaml:"markdown,omitempty"`
}

func (config *WebsiteConfig) Resolve(overrides *WebsiteConfig) {
	if overrides != nil {
		if len(overrides.BuildsDirectory) > 0 { config.BuildsDirectory = overrides.BuildsDirectory }
		if len(overrides.ContentDirectory) > 0 { config.ContentDirectory = overrides.ContentDirectory }
		if len(overrides.DataDirectory) > 0 { config.DataDirectory = overrides.DataDirectory }
		if len(overrides.DraftsDirectory) > 0 { config.DraftsDirectory = overrides.DraftsDirectory }
		if len(overrides.DeploymentsDirectory) > 0 { config.DeploymentsDirectory = overrides.DeploymentsDirectory }
		if len(overrides.StaticDirectory) > 0 { config.StaticDirectory = overrides.StaticDirectory }
		if len(overrides.ThemeSettings) > 0 { config.ThemeSettings = overrides.ThemeSettings }
		if len(overrides.TagLayout) > 0 { config.TagLayout = overrides.TagLayout }
		if len(overrides.TagPath) > 0 { config.TagPath = overrides.TagPath }
	}
	if len(config.BuildsDirectory) == 0 { config.BuildsDirectory = "builds" }
	if len(config.ContentDirectory) == 0 { config.ContentDirectory = "content" }
	if len(config.DataDirectory) == 0 { config.DataDirectory = "data" }
	if len(config.DraftsDirectory) == 0 { config.DraftsDirectory = "drafts" }
	if len(config.DeploymentsDirectory) == 0 { config.DeploymentsDirectory = "deployments" }
	if len(config.StaticDirectory) == 0 { config.StaticDirectory = "static" }
	if len(config.ThemeSettings) == 0 { config.ThemeSettings = "theme.json" }
	if len(config.TagLayout) == 0 { config.TagLayout = "default" }
	if len(config.TagPath) == 0 { config.TagPath = "/tags" }
	config.Markdown.Resolve()
}
