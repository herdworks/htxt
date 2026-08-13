package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"

	"github.com/herdworks/clic"
	"github.com/herdworks/httt/scripting"
	"github.com/herdworks/htxt"
	href "github.com/herdworks/htxt/href"
)

func DataCommand() clic.Command {
	return clic.Command{
		Meta: clic.CommandMeta{
			Description: "Manage website template data.",
			Usage: []string{
				"{{ .Command }} [command] [options]",
			},
			Order: 5,
		},
		Spec: clic.CommandSpec{
			Key:  "data",
			Help: clic.DefaultHelp,
		},
		Commands: clic.MustCommandsFrom(
			DataListCommand,
			DataInspectCommand,
		),
	}
}

func DataListCommand() clic.Command {
	var config clic.Option = WebsiteConfigOption()
	var verbose clic.Option = GlobalVerboseOption()
	verbose.Meta.Description = "Route computed-data console.log output to stderr (default: silenced)."
	return clic.Command{
		Meta: clic.CommandMeta{
			Description: "List build context namespaces (ht, build, env, site, data, theme).",
			Usage: []string{
				"{{ .Command }} [options]",
			},
		},
		Spec: clic.CommandSpec{
			Key:  "ls",
			Help: clic.DefaultHelp,
			Options: clic.Options{
				config.Spec.Key:  &config,
				verbose.Spec.Key: &verbose,
			},
			Handler: dataListHandler,
		},
	}
}
func DataInspectCommand() clic.Command {
	var config clic.Option = WebsiteConfigOption()
	var verbose clic.Option = GlobalVerboseOption()
	verbose.Meta.Description = "Route computed-data console.log output to stderr (default: silenced)."
	return clic.Command{
		Meta: clic.CommandMeta{
			Name:        "inspect",
			Description: "Inspect a value in the build context by keypath (e.g. site.pages, data.social.twitter).",
			Usage: []string{
				"{{ .Command }} <keypath> [options]",
			},
		},
		Spec: clic.CommandSpec{
			Key: "inspect", Help: clic.DefaultHelp,
			Options: clic.Options{
				config.Spec.Key:  &config,
				verbose.Spec.Key: &verbose,
			},
			Validator: func(execution *clic.Execution) error {
				if len(execution.Command.State.Arguments) != 1 {
					return positionalArgumentsError(execution, 1)
				}
				return nil
			},
			Handler: dataInspectHandler,
		},
	}
}
func loadBuildContext(execution *clic.Execution) (*htxt.Build, error) {
	uri, err := href.Parse(execution.Options["config"].StringValue())
	if err != nil {
		return nil, err
	}
	build := &htxt.Build{URI: &url.URL{}, ID: "inspect", Website: &htxt.Website{URI: uri}}
	writer := io.Writer(io.Discard)
	if execution.Options["verbose"].BoolValue() {
		writer = execution.Stderr
		scripting.SetConsoleOutput(execution.Stderr)
	} else {
		scripting.SetConsoleOutput(io.Discard)
	}
	if err = build.LoadContext(execution.Context, writer); err != nil {
		return nil, err
	}
	return build, nil
}
func dataListHandler(execution *clic.Execution) error {
	build, err := loadBuildContext(execution)
	if err != nil {
		return err
	}
	context := build.Context()
	if len(context) == 0 {
		_, err = fmt.Fprintln(execution.Stdout, "No build context configured.")
		return err
	}
	return clic.WriteList(execution.Stdout, clic.ColumnarFormat, htxt.NamespacedData(context),
		clic.Field{Key: "namespace", Header: "Namespace"},
		clic.Field{Key: "size", Header: "Size"},
	)
}
func dataInspectHandler(execution *clic.Execution) error {
	build, err := loadBuildContext(execution)
	if err != nil {
		return err
	}
	key := execution.Command.State.Arguments[0]
	value := build.Context().Get(key)
	if value == nil {
		return fmt.Errorf("no value at keypath %q", key)
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(execution.Stdout, string(data))
	return err
}
