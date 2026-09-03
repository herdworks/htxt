package cli

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/herdworks/clic"
	"github.com/herdworks/htxt"
	href "github.com/herdworks/htxt/href"
)

type assetList struct {
	assets  htxt.Assets
	website *htxt.Website
}

func (list assetList) ColumnValues(field string) (values []string) {
	if !strings.EqualFold(field, "source") {
		return list.assets.ColumnValues(field)
	}
	for _, asset := range list.assets {
		if asset.Source == htxt.BuiltinAsset {
			values = append(values, asset.SourcePath)
			continue
		}
		values = append(values, filepath.ToSlash(list.website.RelativePath(asset.SourcePath)))
	}
	return values
}

func AssetCommand() clic.Command {
	return clic.Command{
		Meta: clic.CommandMeta{
			Description: "Manage website assets",
			Usage: []string{
				"{{ .Command }} [command] [options]",
			},
			Order: 6,
		},
		Spec: clic.CommandSpec{
			Key:  "asset",
			Help: clic.DefaultHelp,
		},
		Commands: clic.MustCommandsFrom(
			AssetAddCommand,
			AssetListCommand,
			AssetLookupCommand,
		),
	}
}
func AssetAddCommand() clic.Command {
	var config clic.Option = WebsiteConfigOption()
	return clic.Command{
		Meta: clic.CommandMeta{
			Description: "Add a static asset to a website.",
			Usage: []string{
				"{{ .Command }} [options] <asset> <path>",
			},
		},
		Spec: clic.CommandSpec{
			Key:  "add",
			Help: clic.DefaultHelp,
			Options: clic.Options{
				config.Spec.Key: &config,
			},
			Validator: expectedPositionalArguments(2),
			Handler:   assetAddHandler,
		},
	}
}
func AssetListCommand() clic.Command {
	var config clic.Option = WebsiteConfigOption()
	return clic.Command{
		Meta: clic.CommandMeta{
			Description: "List all static assets.",
			Usage: []string{
				"{{ .Command }} [options]",
			},
		},
		Spec: clic.CommandSpec{
			Key:  "ls",
			Help: clic.DefaultHelp,
			Options: clic.Options{
				config.Spec.Key: &config,
			},
			Handler: assetListHandler,
		},
	}
}
func AssetLookupCommand() clic.Command {
	var config clic.Option = WebsiteConfigOption()
	return clic.Command{
		Meta: clic.CommandMeta{
			Description: "Lookup a static asset by its path.",
			Usage: []string{
				"{{ .Command }} [options] <path>",
			},
		},
		Spec: clic.CommandSpec{
			Key:  "lookup",
			Help: clic.DefaultHelp,
			Options: clic.Options{
				config.Spec.Key: &config,
			},
			Validator: expectedPositionalArguments(1),
			Handler:   assetLookupHandler,
		},
	}
}

func assetAddHandler(execution *clic.Execution) error {
	website, err := assetWebsite(execution)
	if err != nil {
		return err
	}
	arguments := execution.Command.State.Arguments
	return website.CopyAsset(arguments[0], arguments[1], nil)
}

func assetListHandler(execution *clic.Execution) error {
	website, err := assetWebsite(execution)
	if err != nil {
		return err
	}
	candidates, err := website.AssetCandidates(execution.Context)
	if err != nil {
		return err
	}
	assets := candidates.Resolved()
	return clic.WriteList(execution.Stdout, clic.ColumnarFormat, assetList{assets: assets, website: website},
		clic.Field{Key: "Source", Header: "Source"},
		clic.Field{Key: "Path", Header: "Path"},
	)
}

func assetLookupHandler(execution *clic.Execution) error {
	website, err := assetWebsite(execution)
	if err != nil {
		return err
	}
	query := execution.Command.State.Arguments[0]
	publicPath, err := website.AssetPublicPath(strings.TrimPrefix(query, "/"))
	if err != nil {
		return err
	}
	asset, candidates, err := website.LookupAsset(execution.Context, publicPath)
	if _, err = fmt.Fprintf(execution.Stdout, "asset %q lookup\n", query); err != nil {
		return err
	}
	if errors.Is(err, htxt.ErrAssetNotFound) {
		_, err = fmt.Fprintf(execution.Stdout, "asset %q not found\n", query)
		return err
	}
	if err != nil {
		return err
	}
	for _, candidate := range candidates {
		if _, err = fmt.Fprintf(execution.Stdout, "asset %q candidate %q\n", query, website.RelativePath(candidate.SourcePath)); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintf(execution.Stdout, "asset %q source %q\n", query, website.RelativePath(asset.SourcePath))
	return err
}

func assetWebsite(execution *clic.Execution) (*htxt.Website, error) {
	website := &htxt.Website{URI: &href.Href{URL: url.URL{Path: execution.Options["config"].StringValue()}}}
	return website, website.Load(false, nil, nil)
}
