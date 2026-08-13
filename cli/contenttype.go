package cli

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/herdworks/clic"
	"github.com/herdworks/htxt"
	href "github.com/herdworks/htxt/href"
)

func ContentTypeCommand() clic.Command {
	return clic.Command{
		Meta: clic.CommandMeta{
			Description: "Manage content-types.",
			Usage: []string{
				"{{ .Command }} [command] [options]",
			},
		},
		Spec: clic.CommandSpec{
			Key:  "content-type",
			Help: clic.DefaultHelp,
		},
		Commands: clic.MustCommandsFrom(
			ContentTypeListCommand,
			ContentTypeInspectCommand,
		),
	}
}
func ContentTypeListCommand() clic.Command {
	var config clic.Option = WebsiteConfigOption()
	return clic.Command{
		Meta: clic.CommandMeta{
			Description: "List content types.",
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
			Handler: cmsContentTypeListHandler,
		},
	}
}
func ContentTypeInspectCommand() clic.Command {
	var config clic.Option = WebsiteConfigOption()
	return clic.Command{
		Meta: clic.CommandMeta{
			Description: "Inspect content types.",
			Usage: []string{
				"{{ .Command }} [options] <type>",
			},
		},
		Spec: clic.CommandSpec{
			Key:  "inspect",
			Help: clic.DefaultHelp,
			Options: clic.Options{
				config.Spec.Key: &config,
			},
			Validator: expectedPositionalArguments(1),
			Handler:   cmsContentTypeInspectHandler,
		},
	}
}
func cmsContentTypeWebsite(execution *clic.Execution) (*htxt.Website, error) {
	website := &htxt.Website{URI: &href.Href{URL: url.URL{Path: execution.Options["config"].StringValue()}}}
	return website, website.Load(false, nil, nil)
}
func cmsContentTypeListHandler(execution *clic.Execution) error {
	website, err := cmsContentTypeWebsite(execution)
	if err != nil {
		return err
	}
	types, err := website.ContentTypes()
	if err != nil {
		return err
	}
	if len(types) == 0 {
		_, err = fmt.Fprintln(execution.Stdout, "No content types configured.")
		return err
	}
	return clic.WriteList(execution.Stdout, clic.ColumnarFormat, types,
		clic.Field{Key: "name", Header: "Name"},
		clic.Field{Key: "description", Header: "Description"},
	)
}
func cmsContentTypeInspectHandler(execution *clic.Execution) error {
	website, err := cmsContentTypeWebsite(execution)
	if err != nil {
		return err
	}
	contentType := website.GetContentTypeByName(execution.Command.State.Arguments[0], false, nil)
	if contentType == nil {
		return fmt.Errorf("content-type %q not found", execution.Command.State.Arguments[0])
	}
	data, err := json.Marshal(contentType.Spec.TemplateData)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(execution.Stdout, string(data))
	return err
}
