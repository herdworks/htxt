// Package cli composes htxt's importable command-line interface.
package cli

import (
	"fmt"
	"strings"

	"github.com/herdworks/clic"
	"github.com/herdworks/htxt"
)

// The htxt CLI
var CLI = clic.CLI{
	Command: Command(),
	Version: htxt.LibVersion,
}

// Command returns a fresh htxt command tree for standalone use or host composition.
func Command() clic.Command {
	return clic.Command{
		Meta: clic.CommandMeta{
			Description: "The HyperTexting CLI.",
			EnvPrefix:   "HTXT_",
			Usage: []string{
				"{{ .CLI }} [command] [options]",
			},
		},
		Spec: clic.CommandSpec{
			Key:  "htxt",
			Help: clic.DefaultHelp,
		},
		Commands: clic.MustCommandsFrom(
			InitCommand,
			BuildCommand,
			ServerCommand,
			DataCommand,
			PageCommand,
			AssetCommand,
			ContentTypeCommand,
		),
	}
}

// WebsiteConfigOption selects the website configuration file.
func WebsiteConfigOption() clic.Option {
	return clic.Option{
		Meta: clic.OptionMeta{
			ShortName:   "c",
			EnvName:     "{{ .EnvPrefix }}CONFIG",
			Description: "Path to the website configuration file.",
			Default:     "site.yaml",
		},
		Spec: clic.OptionSpec{
			Key:      "config",
			Kind:     clic.StringOption,
			Required: true,
		},
	}
}

func GlobalVerboseOption() clic.Option {
	return clic.Option{
		Meta: clic.OptionMeta{
			Name:        "verbose",
			ShortName:   "v",
			Description: "Enable verbose mode.",
		},
		Spec: clic.OptionSpec{
			Key:  "verbose",
			Kind: clic.BoolOption,
		},
	}
}

func expectedPositionalArguments(count int) func(*clic.Execution) error {
	return func(execution *clic.Execution) error {
		if execution.Command.PositionalArguments() == count {
			return nil
		}
		return positionalArgumentsError(execution, count)
	}
}

func positionalArgumentsError(execution *clic.Execution, expected int) error {
	return fmt.Errorf(
		"%s expected %v argument(s), got %v (%s)",
		execution.Command.Meta.Name,
		expected,
		execution.Command.PositionalArguments(),
		strings.Join(execution.Command.State.Arguments, ", "),
	)
}
