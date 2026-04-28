package hypertexting

import (
	"fmt"
	"path/filepath"

	hypertemplates "github.com/herdworks/hypertemplates-go"
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
	AssetSourceTheme   AssetSource = "theme"
	AssetSourceWebsite AssetSource = "website"
	AssetSourcePage    AssetSource = "page"
)

func (assets Assets) PublicPaths() []string {
	paths := make([]string, 0, len(assets))
	for _, asset := range assets {
		paths = append(paths, asset.PublicPath)
	}
	return paths
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

func (asset Asset) BuildPath(build *HyperBuild) (string, error) {
	return build.Website.AssetBuildPath(build, asset.RelPath)
}

// AssetPublicPaths returns public URL paths for discovered theme, website, and page assets.
func AssetPublicPaths(theme *hypertemplates.Theme, website *Website, pages Pages) ([]string, error) {
	assets, err := AssetEntries(theme, website, pages)
	if err != nil {
		return nil, err
	}
	return assets.PublicPaths(), nil
}

// AssetEntries combines theme, website, and page assets into resolved asset entries.
// Later sources replace earlier entries with the same public path.
func AssetEntries(theme *hypertemplates.Theme, website *Website, pages Pages) (Assets, error) {
	if website == nil {
		return nil, fmt.Errorf("website is required")
	}
	assets := Assets{}
	indexes := map[string]int{}
	var err error

	if theme != nil {
		themeAssetSourcePath := func(relPath string) (string, error) {
			return hypertemplates.LocalFilePath(theme.StaticDirPath(), relPath)
		}
		for _, relPath := range theme.Assets {
			if assets, err = appendAssetEntry(website, assets, indexes, AssetSourceTheme, relPath, themeAssetSourcePath); err != nil {
				return nil, err
			}
		}
	}
	websiteAssetSourcePath := func(relPath string) (string, error) {
		return website.ResourcePath(website.StaticDirPath(), relPath)
	}
	for _, relPath := range website.Assets {
		if assets, err = appendAssetEntry(website, assets, indexes, AssetSourceWebsite, relPath, websiteAssetSourcePath); err != nil {
			return nil, err
		}
	}
	pageAssetSourcePath := func(relPath string) (string, error) {
		return website.ResourcePath(website.ContentDirPath(), relPath)
	}
	for _, page := range pages {
		if page == nil {
			continue
		}
		for _, relPath := range page.Assets {
			if assets, err = appendAssetEntry(website, assets, indexes, AssetSourcePage, relPath, pageAssetSourcePath); err != nil {
				return nil, err
			}
		}
	}
	return assets, nil
}

func appendAssetEntry(website *Website, assets Assets, indexes map[string]int, source AssetSource, relPath string, sourcePath func(string) (string, error)) (Assets, error) {
	relPath, err := hypertemplates.CleanRelativeFilePath(relPath)
	if err != nil {
		return nil, err
	}
	publicPath, err := website.AssetPublicPath(relPath)
	if err != nil {
		return nil, err
	}
	srcPath, err := sourcePath(relPath)
	if err != nil {
		return nil, err
	}
	asset := Asset{
		Source:     source,
		RelPath:    filepath.ToSlash(relPath),
		PublicPath: publicPath,
		SourcePath: srcPath,
	}
	if index, exists := indexes[publicPath]; exists {
		assets[index] = asset
		return assets, nil
	}
	indexes[publicPath] = len(assets)
	return append(assets, asset), nil
}
