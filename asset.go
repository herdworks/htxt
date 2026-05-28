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

func (asset Asset) BuildPath(build *Build) (string, error) {
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

// AssetCandidates returns the full, un-collapsed list of theme, website, and
// page asset entries in iteration order (theme first, then website, then
// page-by-page). Entries that share a public path are preserved so callers can
// inspect every contributor; use AssetEntries for the collapsed view.
func AssetCandidates(theme *hypertemplates.Theme, website *Website, pages Pages) (Assets, error) {
	if website == nil {
		return nil, fmt.Errorf("website is required")
	}
	candidates := Assets{}
	if theme != nil {
		themeAssetSourcePath := func(relPath string) (string, error) {
			return hypertemplates.LocalFilePath(theme.StaticDirPath(), relPath)
		}
		for _, relPath := range theme.Assets {
			asset, err := buildAssetEntry(website, AssetSourceTheme, relPath, themeAssetSourcePath)
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
		asset, err := buildAssetEntry(website, AssetSourceWebsite, relPath, websiteAssetSourcePath)
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
			asset, err := buildAssetEntry(website, AssetSourcePage, relPath, pageAssetSourcePath)
			if err != nil {
				return nil, err
			}
			candidates = append(candidates, asset)
		}
	}
	return candidates, nil
}

// AssetEntries combines theme, website, and page assets into resolved asset entries.
// Later sources replace earlier entries with the same public path.
func AssetEntries(theme *hypertemplates.Theme, website *Website, pages Pages) (Assets, error) {
	candidates, err := AssetCandidates(theme, website, pages)
	if err != nil {
		return nil, err
	}
	assets := Assets{}
	indexes := map[string]int{}
	for _, asset := range candidates {
		if index, exists := indexes[asset.PublicPath]; exists {
			assets[index] = asset
			continue
		}
		indexes[asset.PublicPath] = len(assets)
		assets = append(assets, asset)
	}
	return assets, nil
}

func buildAssetEntry(website *Website, source AssetSource, relPath string, sourcePath func(string) (string, error)) (Asset, error) {
	relPath, err := hypertemplates.CleanRelativeFilePath(relPath)
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
