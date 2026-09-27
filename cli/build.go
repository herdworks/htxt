package cli

import (
	"fmt"

	"github.com/herdworks/clic"
	"github.com/herdworks/htxt"
)

// BuildCommand builds a HyperTexting website.
func BuildCommand() clic.Command {
	return clic.Command{
		Meta: clic.CommandMeta{
			Description: "Generate static website builds",
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
				BuildDraftsDirOption,
				BuildDataDirOption,
				BuildStaticDirOption,
				BuildThemeOption,
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
			EnvName:     "{{ .EnvPrefix }}BUILD_ID",
			Description: "Identifier to use for the build.",
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
func BuildDraftsDirOption() clic.Option {
	return clic.Option{
		Meta: clic.OptionMeta{
			Description: "Relative path to the draft content directory.",
		},
		Spec: clic.OptionSpec{
			Key:  "drafts-dir",
			Kind: clic.StringOption,
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
	incremental := execution.Options.BoolValue("incremental")
	page := execution.Options.StringValue("page")
	if incremental && page == "" {
		return fmt.Errorf("--page is required when --incremental is set")
	}
	if !incremental && page != "" {
		return fmt.Errorf("--page can only be used when --incremental is set")
	}
	return nil
}

func buildHandler(execution *clic.Execution) error {
	build := &htxt.Build{Config: buildConfig(execution)}
	return build.Execute(execution.Context)
}

func buildConfig(execution *clic.Execution) htxt.BuildConfig {
	return htxt.BuildConfig{
		WebsiteConfig: htxt.WebsiteConfig{
			Path:             execution.Options.StringValue("config"),
			ContentDirectory: execution.Options.StringValue("content-dir"),
			DataDirectory:    execution.Options.StringValue("data-dir"),
			StaticDirectory:  execution.Options.StringValue("static-dir"),
			ThemePath:        execution.Options.StringValue("theme"),
		},
		ID:              execution.Options.StringValue("id"),
		Incremental:     execution.Options.BoolValue("incremental"),
		Page:            execution.Options.StringValue("page"),
		DraftsDirectory: execution.Options.StringValue("drafts-dir"),
		Preview:         execution.Options.StringValue("drafts-dir") != "",
		LogFile:         execution.Options.StringValue("log-file"),
		Verbose:         true,
	}
}
