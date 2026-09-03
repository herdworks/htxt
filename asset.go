package htxt

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	htfiles "github.com/herdworks/httt/files"
	httheme "github.com/herdworks/httt/theme"
)

// Asset describes one discovered asset and its resolved source and public paths.
type Asset struct {
	Source     AssetSource
	RelPath    string
	PublicPath string
	SourcePath string
}

// Assets is an ordered list of discovered asset entries.
type Assets []Asset

// AssetSource identifies the root that contributed an asset.
type AssetSource string

const (
	BuiltinAsset AssetSource = "builtin"
	ThemeAsset   AssetSource = "theme"
	WebsiteAsset AssetSource = "website"
	PageAsset    AssetSource = "page"
)

var ErrAssetNotFound = errors.New("asset not found")

func (assets Assets) PublicPaths() []string {
	paths := make([]string, 0, len(assets))
	for _, asset := range assets {
		paths = append(paths, asset.PublicPath)
	}
	return paths
}

// Resolved returns one asset per public path, with later candidates replacing
// earlier ones while retaining the original public-path order.
func (assets Assets) Resolved() Assets {
	resolved := Assets{}
	indexes := map[string]int{}
	for _, asset := range assets {
		if index, exists := indexes[asset.PublicPath]; exists {
			resolved[index] = asset
			continue
		}
		indexes[asset.PublicPath] = len(resolved)
		resolved = append(resolved, asset)
	}
	return resolved
}

// ColumnValues returns the source and published-path values for assets.
func (assets Assets) ColumnValues(field string) (values []string) {
	for _, asset := range assets {
		switch field {
		case "Source", "source":
			values = append(values, asset.SourcePath)
		case "Path", "path":
			values = append(values, asset.PublicPath)
		}
	}
	return values
}

func (assets Assets) FilterSource(source AssetSource) Assets {
	filtered := Assets{}
	for _, asset := range assets {
		if asset.Source == source {
			filtered = append(filtered, asset)
		}
	}
	return filtered
}

// AssetPublicPaths returns public URL paths for discovered builtin, theme, website, and page assets.
func AssetPublicPaths(theme *httheme.Theme, website *Website, pages Pages) ([]string, error) {
	candidates, err := AssetCandidates(theme, website, pages)
	if err != nil {
		return nil, err
	}
	return candidates.Resolved().PublicPaths(), nil
}

// AssetCandidates returns the full, un-collapsed list of builtin, theme,
// website, and page assets in precedence order. Entries that share a public
// path are preserved so callers can inspect every contributor; use Resolved
// for the collapsed view.
func AssetCandidates(theme *httheme.Theme, website *Website, pages Pages) (Assets, error) {
	if website == nil {
		return nil, fmt.Errorf("website is required")
	}
	hydrateThemeBuiltins(&website.Theme)
	candidates := Assets{}
	builtinAssets, err := builtinAssetEntries(&website.Theme, website)
	if err != nil {
		return nil, err
	}
	candidates = append(candidates, builtinAssets...)
	if theme != nil {
		themeAssetSourcePath := func(relPath string) (string, error) {
			return htfiles.LocalPath(theme.StaticDirPath(), relPath)
		}
		for _, relPath := range theme.Assets {
			asset, err := buildAssetEntry(website, ThemeAsset, relPath, themeAssetSourcePath)
			if err != nil {
				return nil, err
			}
			candidates = append(candidates, asset)
		}
	}
	websiteAssetSourcePath := func(relPath string) (string, error) {
		return website.ResourcePath(website.StaticDirPath(), relPath)
	}
	for _, relPath := range website.Assets {
		asset, err := buildAssetEntry(website, WebsiteAsset, relPath, websiteAssetSourcePath)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, asset)
	}
	pageAssetSourcePath := func(relPath string) (string, error) {
		return website.ResourcePath(website.ContentDirPath(), relPath)
	}
	for _, page := range pages {
		if page == nil {
			continue
		}
		for _, relPath := range page.Assets {
			asset, err := buildAssetEntry(website, PageAsset, relPath, pageAssetSourcePath)
			if err != nil {
				return nil, err
			}
			candidates = append(candidates, asset)
		}
	}
	return candidates, nil
}

func builtinAssetEntries(theme *httheme.Theme, website *Website) (assets Assets, err error) {
	hydrateThemeBuiltins(theme)
	err = fs.WalkDir(theme.BuiltinsFS, builtinStaticDir, func(sourcePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, fs.ErrNotExist) {
				return nil
			}
			return walkErr
		}
		if entry.IsDir() || !entry.Type().IsRegular() {
			return nil
		}
		relPath := strings.TrimPrefix(sourcePath, builtinStaticDir+"/")
		asset, err := buildAssetEntry(website, BuiltinAsset, relPath, func(string) (string, error) {
			return sourcePath, nil
		})
		if err != nil {
			return err
		}
		assets = append(assets, asset)
		return nil
	})
	return assets, err
}

func buildAssetEntry(website *Website, source AssetSource, relPath string, sourcePath func(string) (string, error)) (Asset, error) {
	relPath, err := htfiles.CleanRelativePath(relPath)
	if err != nil {
		return Asset{}, err
	}
	publicPath, err := website.AssetPublicPath(relPath)
	if err != nil {
		return Asset{}, err
	}
	srcPath, err := sourcePath(relPath)
	if err != nil {
		return Asset{}, err
	}
	return Asset{
		Source:     source,
		RelPath:    filepath.ToSlash(relPath),
		PublicPath: publicPath,
		SourcePath: srcPath,
	}, nil
}
