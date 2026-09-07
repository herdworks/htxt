package cli

import (
	"encoding/json"
	"fmt"

	"github.com/herdworks/clic"
	"github.com/herdworks/htxt"
	href "github.com/herdworks/htxt/href"
)

func PageCommand() clic.Command {
	return clic.Command{
		Meta: clic.CommandMeta{
			Description: "Manage website pages",
			Usage: []string{
				"{{ .Command }} [command] [options]",
			},
			Order: 4,
		},
		Spec: clic.CommandSpec{
			Key:  "page",
			Help: clic.DefaultHelp,
		},
		Commands: clic.MustCommandsFrom(
			PageListCommand,
			PageInspectCommand,
		),
	}
}

func PageListCommand() clic.Command {
	var config clic.Option = WebsiteConfigOption()
	var visibility clic.Option = PageVisibilityOption()
	return clic.Command{
		Meta: clic.CommandMeta{
			Description: "List website pages.",
			Usage: []string{
				"{{ .Command }} [options]",
			}},
		Spec: clic.CommandSpec{
			Key:  "ls",
			Help: clic.DefaultHelp,
			Options: clic.Options{
				config.Spec.Key:     &config,
				visibility.Spec.Key: &visibility,
			},
			Validator: pageListValidator,
			Handler:   pageListHandler,
		},
	}
}
func PageInspectCommand() clic.Command {
	var config clic.Option = WebsiteConfigOption()
	return clic.Command{
		Meta: clic.CommandMeta{
			Description: "Inspect website pages.",
			Usage: []string{
				"{{ .Command }} [options] <path>",
			},
		},
		Spec: clic.CommandSpec{
			Key:  "inspect",
			Help: clic.DefaultHelp,
			Options: clic.Options{
				config.Spec.Key: &config,
			},
			Validator: expectedPositionalArguments(1),
			Handler:   pageInspectHandler,
		},
	}
}

func PageVisibilityOption() clic.Option {
	return clic.Option{
		Meta: clic.OptionMeta{
			Description: "Page visibility: all, public, unlisted, status, draft, or none.",
			Default:     "all",
		},
		Spec: clic.OptionSpec{
			Key:  "visibility",
			Kind: clic.StringOption,
		},
	}
}

func pageListValidator(execution *clic.Execution) error {
	if err := expectedPositionalArguments(0)(execution); err != nil {
		return err
	}
	visibility := execution.Options["visibility"].StringValue()
	switch visibility {
	case "all", "public", "unlisted", "status", "draft", "none":
		return nil
	default:
		return fmt.Errorf("visibility %q unknown (expected one of: all, public, unlisted, status, draft, none)", visibility)
	}
}

func pageListHandler(execution *clic.Execution) error {
	uri, err := href.Parse(execution.Options["config"].StringValue())
	if err != nil {
		return err
	}
	website := &htxt.Website{URI: uri}
	if err = website.Load(false, nil, nil); err != nil {
		return err
	}
	pages, err := website.ListPages()
	if err != nil {
		return err
	}
	if err = htxt.LoadPages(execution.Context, website, pages, nil, false, nil); err != nil {
		return err
	}
	pages = filterPages(pages, execution.Options["visibility"].StringValue())
	pages.SortBy("slug", true)
	return clic.WriteList(execution.Stdout, clic.ColumnarFormat, pages,
		clic.Field{Key: "lastmod", Header: "Last Modified"},
		clic.Field{Key: "title", Header: "Title"},
		clic.Field{Key: "visibility", Header: "Visibility"},
		clic.Field{Key: "path", Header: "Path"},
	)
}
func filterPages(pages htxt.Pages, visibility string) htxt.Pages {
	result := make(htxt.Pages, 0, len(pages))
	for _, page := range pages {
		if page == nil || !matchesPageVisibility(page.Visibility, visibility) {
			continue
		}
		result = append(result, page)
	}
	return result
}

func matchesPageVisibility(visibility htxt.PageVisibilityKind, filter string) bool {
	switch filter {
	case "all":
		return true
	case "public":
		return visibility.IsPublic()
	case "unlisted":
		return visibility.IsUnlisted()
	case "status":
		return visibility.IsStatus()
	case "draft":
		return visibility.IsDraft()
	case "none":
		return visibility.IsNone()
	default:
		return false
	}
}
func pageInspectHandler(execution *clic.Execution) error {
	uri, err := href.Parse(execution.Options["config"].StringValue())
	if err != nil {
		return err
	}
	website := &htxt.Website{URI: uri}
	if err = website.Load(false, nil, nil); err != nil {
		return err
	}
	build := &htxt.Build{Website: website}
	if err = build.LoadContext(execution.Context, nil); err != nil {
		return err
	}
	page := build.Pages.ByPublicURLPath(execution.Command.State.Arguments[0])
	if page == nil {
		return fmt.Errorf("page %q not found", execution.Command.State.Arguments[0])
	}
	data, err := json.Marshal(page.TemplateData)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(execution.Stdout, string(data))
	return err
}
