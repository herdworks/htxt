package cli

import (
	"fmt"

	"github.com/herdworks/clic"
	"github.com/herdworks/htxt"
)

func SchemaCommand() clic.Command {
	return clic.Command{
		Meta: clic.CommandMeta{
			Name:        "schema",
			Description: "HyperTexting data contracts as JSON Schema.",
			Usage: []string{
				"{{ .Command }} [command]",
			},
			Order: 8,
		},
		Spec: clic.CommandSpec{
			Key:  "schema",
			Help: clic.DefaultHelp,
		},
		Commands: clic.MustCommandsFrom(
			SchemaListCommand,
			SchemaDescribeCommand,
		),
	}
}

func SchemaListCommand() clic.Command {
	return clic.Command{
		Meta: clic.CommandMeta{
			Name:        "ls",
			Description: "List stable schema kinds.",
			Usage: []string{
				"{{ .Command }}",
			},
		},
		Spec: clic.CommandSpec{
			Key:  "ls",
			Help: clic.DefaultHelp,
			Handler: func(execution *clic.Execution) error {
				if err := htxt.SchemaRegistry.Validate(); err != nil {
					return err
				}
				return clic.WriteList(execution.Stdout, clic.ColumnarFormat, htxt.SchemaRegistry.Entries(),
					clic.Field{Key: "Name", Header: "Name"},
					clic.Field{Key: "Description", Header: "Description"},
				)
			},
		},
	}
}

func SchemaDescribeCommand() clic.Command {
	return clic.Command{
		Meta: clic.CommandMeta{
			Name:        "describe",
			Description: "Show one JSON Schema document.",
			Usage: []string{
				"{{ .Command }} <kind>",
			},
		},
		Spec: clic.CommandSpec{
			Key:       "describe",
			Help:      clic.DefaultHelp,
			Validator: expectedPositionalArguments(1),
			Handler: func(execution *clic.Execution) error {
				document, err := htxt.SchemaRegistry.Describe(execution.Command.State.Arguments[0])
				if err != nil {
					return err
				}
				_, err = fmt.Fprint(execution.Stdout, string(document))
				return err
			},
		},
	}
}
