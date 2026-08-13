package cli

import (
	"fmt"

	"github.com/herdworks/clic"
	"github.com/herdworks/htxt"
	href "github.com/herdworks/htxt/href"
	"github.com/herdworks/htxtd"
)

// ServerCommand builds and live-reloads a preview website.
func ServerCommand() clic.Command {
	return clic.Command{
		Meta: clic.CommandMeta{
			Description: "Local website preview server.",
			Usage: []string{
				"{{ .Command }} [options]",
			},
			Order: 3,
		},
		Spec: clic.CommandSpec{
			Key:  "server",
			Help: clic.DefaultHelp,
			Options: clic.MustOptionsFrom(
				WebsiteConfigOption,
				BuildIdentifierOption,
				BuildContentDirOption,
				BuildDataDirOption,
				BuildStaticDirOption,
				BuildThemeOption,
				BuildDataFileOption,
				BuildLogFileOption,
				ServerBindOption,
				ServerPortOption,
			),
			Validator: expectedPositionalArguments(0),
			Handler:   serverHandler,
		},
	}
}

func ServerBindOption() clic.Option {
	return clic.Option{
		Meta: clic.OptionMeta{
			ShortName:   "b",
			EnvName:     "{{ .EnvPrefix }}BIND",
			Description: "IP address or hostname to bind.",
			Default:     "localhost",
		},
		Spec: clic.OptionSpec{
			Key:  "bind",
			Kind: clic.StringOption,
		},
	}
}

func ServerPortOption() clic.Option {
	return clic.Option{
		Meta: clic.OptionMeta{
			ShortName:   "p",
			EnvName:     "{{ .EnvPrefix }}PORT",
			Description: "TCP port to listen on.",
			Default:     "8080",
		},
		Spec: clic.OptionSpec{
			Key:  "port",
			Kind: clic.StringOption,
		},
	}
}

func serverHandler(execution *clic.Execution) error {
	options := execution.Options
	websiteURI, err := href.Parse(options["config"].StringValue())
	if err != nil {
		return err
	}
	port, err := parseServerPort(options["port"].StringValue())
	if err != nil {
		return err
	}
	server := &htxt.Server{
		Server: htxtd.Server{
			Bind:      options["bind"].StringValue(),
			Port:      port,
			LogWriter: execution.Stdout,
		},
		WebsiteURI: websiteURI,
		BuildID:    options["id"].StringValue(),
		BuildConfig: htxt.BuildConfig{
			WebsiteConfig: htxt.WebsiteConfig{
				ContentDirectory: options["content-dir"].StringValue(),
				DataDirectory:    options["data-dir"].StringValue(),
				StaticDirectory:  options["static-dir"].StringValue(),
				ThemePath:        options["theme"].StringValue(),
			},
			DataFile: options["data-file"].StringValue(),
			LogFile:  options["log-file"].StringValue(),
		},
	}
	if err = server.Validate(); err != nil {
		return err
	}
	return server.Run(execution.Context)
}

func parseServerPort(value string) (int, error) {
	var port int
	if _, err := fmt.Sscan(value, &port); err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("invalid port: %s", value)
	}
	return port, nil
}
