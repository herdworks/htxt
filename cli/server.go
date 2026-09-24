package cli

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/herdworks/clic"
	"github.com/herdworks/htxt"
	href "github.com/herdworks/htxt/href"
	"github.com/herdworks/htxtd"
)

// ServerCommand builds and live-reloads a preview website.
func ServerCommand() clic.Command {
	return clic.Command{
		Meta: clic.CommandMeta{
			Description: "Local website preview server",
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
				BuildDraftsDirOption,
				BuildDataDirOption,
				BuildStaticDirOption,
				BuildThemeOption,
				BuildDataFileOption,
				BuildLogFileOption,
				ServerBindOption,
				ServerHostOption,
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
			Description: "IP address or hostname to bind (overrides site.config.preview.bind).",
		},
		Spec: clic.OptionSpec{
			Key:  "bind",
			Kind: clic.StringOption,
		},
	}
}

func ServerHostOption() clic.Option {
	return clic.Option{
		Meta: clic.OptionMeta{
			EnvName:     "{{ .EnvPrefix }}HOST",
			Description: "Hostname or IP address used in generated preview URLs (independent of bind).",
			Default:     "localhost",
		},
		Spec: clic.OptionSpec{
			Key:  "host",
			Kind: clic.StringOption,
		},
	}
}

func ServerPortOption() clic.Option {
	return clic.Option{
		Meta: clic.OptionMeta{
			ShortName:   "p",
			EnvName:     "{{ .EnvPrefix }}PORT",
			Description: "TCP port to listen on (overrides site.config.preview.port).",
		},
		Spec: clic.OptionSpec{
			Key:  "port",
			Kind: clic.StringOption,
		},
	}
}

func serverHandler(execution *clic.Execution) error {
	server, err := serverForExecution(execution)
	if err != nil {
		return err
	}
	return server.Run(execution.Context)
}

func serverForExecution(execution *clic.Execution) (*htxt.Server, error) {
	options := execution.Options
	websiteURI, err := href.Parse(options["config"].StringValue())
	if err != nil {
		return nil, err
	}
	var port int
	if value := options["port"].StringValue(); value != "" {
		if port, err = parseServerPort(value); err != nil {
			return nil, err
		}
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
			DataFile:        options["data-file"].StringValue(),
			LogFile:         options["log-file"].StringValue(),
			DraftsDirectory: options["drafts-dir"].StringValue(),
		},
	}
	if err = server.ResolveConfig(); err != nil {
		return nil, err
	}
	if server.BaseURL, err = serverBaseURL(options["host"].StringValue(), server.Port); err != nil {
		return nil, err
	}
	if err = server.Validate(); err != nil {
		return nil, err
	}
	return server, nil
}

func serverBaseURL(host string, port int) (string, error) {
	// Accept bracketed IPv6 literals as well as bare addresses.
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = host[1 : len(host)-1]
		if net.ParseIP(host) == nil {
			return "", fmt.Errorf("invalid preview host: %q", host)
		}
	}
	if host == "" || strings.ContainsAny(host, "/?#@\\ \t\r\n") ||
		(strings.Contains(host, ":") && net.ParseIP(host) == nil) {
		return "", fmt.Errorf("host %q must be a hostname or IP address without a scheme, port, or path", host)
	}
	baseURL := "http://" + net.JoinHostPort(host, strconv.Itoa(port)) + "/"
	if _, err := url.Parse(baseURL); err != nil {
		return "", fmt.Errorf("invalid preview host %q: %w", host, err)
	}
	return baseURL, nil
}

func parseServerPort(value string) (int, error) {
	var port int
	if _, err := fmt.Sscan(value, &port); err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("invalid port: %s", value)
	}
	return port, nil
}
