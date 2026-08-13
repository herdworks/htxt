package cli

import (
	"fmt"
	"net/url"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/herdworks/clic"
	"github.com/herdworks/htxt"
	href "github.com/herdworks/htxt/href"
)

// BuildCommand builds a HyperTexting website.
func BuildCommand() clic.Command {
	return clic.Command{
		Meta: clic.CommandMeta{
			Description: "Generate static website builds.",
			Usage: []string{
				"{{ .Command }} [options]",
			},
			Order: 2,
		},
		Spec: clic.CommandSpec{
			Key:  "build",
			Help: clic.DefaultHelp,
			Options: clic.MustOptionsFrom(
				WebsiteConfigOption,
				BuildIdentifierOption,
				BuildIncrementalOption,
				BuildPageOption,
				BuildContentDirOption,
				BuildDataDirOption,
				BuildStaticDirOption,
				BuildThemeOption,
				BuildDataFileOption,
				BuildLogFileOption,
			),
			Validator: buildValidator,
			Handler:   buildHandler,
		},
	}
}

func BuildIdentifierOption() clic.Option {
	return clic.Option{
		Meta: clic.OptionMeta{
			ShortName:   "i",
			Description: "Identifier to use for the build.",
			Default:     uuid.New().String(),
		},
		Spec: clic.OptionSpec{
			Key:      "id",
			Kind:     clic.StringOption,
			Required: false,
		},
	}
}
func BuildIncrementalOption() clic.Option {
	return clic.Option{
		Meta: clic.OptionMeta{
			Description: "Generate an incremental build.",
		},
		Spec: clic.OptionSpec{
			Key:      "incremental",
			Kind:     clic.BoolOption,
			Required: false,
		},
	}
}
func BuildPageOption() clic.Option {
	return clic.Option{
		Meta: clic.OptionMeta{
			ShortName:   "p",
			Description: "Path to page data file to use for the incremental build.",
		},
		Spec: clic.OptionSpec{
			Key:      "page",
			Kind:     clic.StringOption,
			Required: false,
		},
	}
}
func BuildContentDirOption() clic.Option {
	return clic.Option{
		Meta: clic.OptionMeta{
			EnvName:     "{{ .EnvPrefix }}CONTENT_DIR",
			Description: "Relative path to the content directory to use for the build (overwrites site.config.content_dir).",
		},
		Spec: clic.OptionSpec{
			Key:      "content-dir",
			Kind:     clic.StringOption,
			Required: false,
		},
	}
}
func BuildDataDirOption() clic.Option {
	return clic.Option{
		Meta: clic.OptionMeta{
			EnvName:     "{{ .EnvPrefix }}DATA_DIR",
			Description: "Relative path to the data directory to use for the build (overwrites site.config.data_dir).",
		},
		Spec: clic.OptionSpec{
			Key:      "data-dir",
			Kind:     clic.StringOption,
			Required: false,
		},
	}
}
func BuildStaticDirOption() clic.Option {
	return clic.Option{
		Meta: clic.OptionMeta{
			EnvName:     "{{ .EnvPrefix }}STATIC_DIR",
			Description: "Relative path to the static directory to use for the build (overwrites site.config.static_dir).",
		},
		Spec: clic.OptionSpec{
			Key:      "static-dir",
			Kind:     clic.StringOption,
			Required: false,
		},
	}
}
func BuildThemeOption() clic.Option {
	return clic.Option{
		Meta: clic.OptionMeta{
			EnvName:     "{{ .EnvPrefix }}THEME",
			Description: "Relative path to the theme config to use for the build (overwrites site.config.theme).",
		},
		Spec: clic.OptionSpec{
			Key:      "theme",
			Kind:     clic.StringOption,
			Required: false,
		},
	}
}
func BuildDataFileOption() clic.Option {
	return clic.Option{
		Meta: clic.OptionMeta{
			Description: "Relative path to write an optional build data JSON file (e.g. build.json).",
		},
		Spec: clic.OptionSpec{
			Key:      "data-file",
			Kind:     clic.StringOption,
			Required: false,
		},
	}
}
func BuildLogFileOption() clic.Option {
	return clic.Option{
		Meta: clic.OptionMeta{
			Description: "Relative path to write an optional build log file (e.g. build.log).",
		},
		Spec: clic.OptionSpec{
			Key:      "log-file",
			Kind:     clic.StringOption,
			Required: false,
		},
	}
}

func buildValidator(execution *clic.Execution) error {
	incremental := execution.Options["incremental"].BoolValue()
	page := execution.Options["page"].StringValue()
	if incremental && page == "" {
		return fmt.Errorf("--page is required when --incremental is set")
	}
	if !incremental && page != "" {
		return fmt.Errorf("--page can only be used when --incremental is set")
	}
	return nil
}

func buildHandler(execution *clic.Execution) error {
	options := execution.Options
	websiteURI, err := href.Parse(options["config"].StringValue())
	if err != nil {
		return err
	}
	config := htxt.BuildConfig{WebsiteConfig: htxt.WebsiteConfig{ContentDirectory: options["content-dir"].StringValue(), DataDirectory: options["data-dir"].StringValue(), StaticDirectory: options["static-dir"].StringValue(), ThemePath: options["theme"].StringValue()}, DataFile: options["data-file"].StringValue(), LogFile: options["log-file"].StringValue(), Verbose: true}
	website := &htxt.Website{URI: websiteURI}
	if err := website.Load(false, &config.WebsiteConfig, nil); err != nil {
		return err
	}
	var page *htxt.Page
	if options["incremental"].BoolValue() {
		pageURI, err := href.Parse(options["page"].StringValue())
		if err != nil {
			return err
		}
		page = &htxt.Page{URI: &pageURI.URL, Path: htxt.PagePath(website, &pageURI.URL)}
	}
	build := &htxt.Build{URI: &url.URL{Path: filepath.Join(website.BuildsDirPath(), options["id"].StringValue())}, ID: options["id"].StringValue(), Website: website, Config: config, Incremental: options["incremental"].BoolValue(), Page: page}
	return build.Execute(execution.Context)
}
