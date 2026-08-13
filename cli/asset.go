package cli

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/herdworks/clic"
	"github.com/herdworks/htxt"
	href "github.com/herdworks/htxt/href"
)

func AssetCommand() clic.Command {
	return clic.Command{
		Meta: clic.CommandMeta{
			Description: "Manage website assets.",
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
	website, pages, err := assetPages(execution)
	if err != nil {
		return err
	}
	assets, err := htxt.AssetEntries(&website.Theme, website, pages)
	if err != nil {
		return err
	}
	return clic.WriteList(execution.Stdout, clic.ColumnarFormat, assets,
		clic.Field{Key: "Source", Header: "Source"},
		clic.Field{Key: "Path", Header: "Path"},
	)
}

func assetLookupHandler(execution *clic.Execution) error {
	website, pages, err := assetPages(execution)
	if err != nil {
		return err
	}
	query := execution.Command.State.Arguments[0]
	assets, err := htxt.AssetCandidates(&website.Theme, website, pages)
	if err != nil {
		return err
	}
	matches := htxt.Assets{}
	for _, asset := range assets {
		if asset.RelPath == strings.TrimPrefix(query, "/") {
			matches = append(matches, asset)
		}
	}
	if _, err = fmt.Fprintf(execution.Stdout, "asset %q lookup\n", query); err != nil {
		return err
	}
	if len(matches) == 0 {
		_, err = fmt.Fprintf(execution.Stdout, "asset %q not found\n", query)
		return err
	}
	for _, asset := range matches {
		if _, err = fmt.Fprintf(execution.Stdout, "asset %q candidate %q\n", query, website.RelativePath(asset.SourcePath)); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintf(execution.Stdout, "asset %q source %q\n", query, website.RelativePath(matches[len(matches)-1].SourcePath))
	return err
}

func assetWebsite(execution *clic.Execution) (*htxt.Website, error) {
	website := &htxt.Website{URI: &href.Href{URL: url.URL{Path: execution.Options["config"].StringValue()}}}
	return website, website.Load(false, nil, nil)
}
func assetPages(execution *clic.Execution) (*htxt.Website, htxt.Pages, error) {
	website, err := assetWebsite(execution)
	if err != nil {
		return nil, nil, err
	}
	pages, err := htxt.GetPages(website, website.ContentDirPath())
	if err != nil {
		return nil, nil, err
	}
	if err = htxt.LoadPages(execution.Context, website, pages, nil, false, nil); err != nil {
		return nil, nil, err
	}
	return website, pages, nil
}
