package cli

import (
	"fmt"
	"path/filepath"

	"github.com/herdworks/clic"
	"github.com/herdworks/htup"
	envsecrets "github.com/herdworks/htup/providers/env"
	filesecrets "github.com/herdworks/htup/providers/files"
	"github.com/herdworks/htxt"
)

// DeployCommand builds and deploys a HyperTexting website.
func DeployCommand() clic.Command {
	return clic.Command{
		Meta: clic.CommandMeta{
			Description: "Build and deploy a static website",
			Usage: []string{
				"{{ .Command }} [options]",
			},
			Order: 3,
		},
		Spec: clic.CommandSpec{
			Key:  "deploy",
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
				BuildLogFileOption,
				DeploymentEnvironmentOption,
				DeploymentProvidersOption,
			),
			Validator: deployValidator,
			Handler:   deployHandler,
		},
	}
}

// DeploymentEnvironmentOption selects a configured deployment environment.
func DeploymentEnvironmentOption() clic.Option {
	return clic.Option{
		Meta: clic.OptionMeta{
			ShortName:   "e",
			EnvName:     "{{ .EnvPrefix }}ENVIRONMENT",
			Description: "Deployment environment to use from site.environments.",
		},
		Spec: clic.OptionSpec{
			Key:      "environment",
			Kind:     clic.StringOption,
			Required: false,
		},
	}
}

// DeploymentProvidersOption selects configured deployment providers.
func DeploymentProvidersOption() clic.Option {
	return clic.Option{
		Meta: clic.OptionMeta{
			EnvName:     "{{ .EnvPrefix }}PROVIDERS",
			Description: "Comma-separated provider names to deploy from site.providers.",
		},
		Spec: clic.OptionSpec{
			Key:       "providers",
			Kind:      clic.StringsOption,
			Separator: ",",
			Required:  false,
		},
	}
}

func deployValidator(execution *clic.Execution) (err error) {
	if err = buildValidator(execution); err != nil {
		return err
	}
	environment := execution.Options.StringValue("environment")
	providers := execution.Options.StringValues("providers")
	if environment != "" && len(providers) > 0 {
		return fmt.Errorf("providers and environment are mutually exclusive")
	}
	return nil
}

func deployHandler(execution *clic.Execution) (err error) {
	deployment := &htxt.Deployment{
		Config: htxt.DeploymentConfig{
			Build:       buildConfig(execution),
			Environment: execution.Options.StringValue("environment"),
			Providers:   execution.Options.StringValues("providers"),
		},
		Secrets: deploymentSecrets(execution.Options.StringValue("config")),
		Writer:  execution.Stdout,
	}
	if err = deployment.Execute(execution.Context); err != nil {
		return err
	}
	_, err = fmt.Fprintf(execution.Stdout, "\nBuild %q deployed\n", deployment.Build.Config.ID)
	return err
}

func deploymentSecrets(path string) htup.SecretsProviders {
	files := filesecrets.New()
	if path != "" {
		files = filesecrets.NewWithBaseDirectory(filepath.Dir(path))
	}
	return htup.SecretsProviders{envsecrets.New(), files}
}
