package htxt

import (
	"context"
	"fmt"
	"io"

	"github.com/herdworks/hsync"
	"github.com/herdworks/htup"
)

// Deployment configures a build and its hsync-enabled deployment.
type Deployment struct {
	// Declarative configuration
	Config DeploymentConfig

	// Runtime
	Secrets   htup.SecretsProviders
	Writer    io.Writer
	Build     *Build
	operation *hsync.Deployment
}

type DeploymentConfig struct {
	Build       BuildConfig
	Environment string
	Providers   []string
}

// Execute builds the configured website, then deploys the fresh build output.
// It reports success only after both operations have completed.
func (d *Deployment) Execute(ctx context.Context) (err error) {
	if d == nil {
		return fmt.Errorf("deployment is required")
	}
	if d.Config.Build.DraftsDirectory != "" {
		return fmt.Errorf("deployment does not support build.config.drafts_dir")
	}
	d.Build = &Build{Config: d.Config.Build}
	if err := d.Build.Execute(ctx); err != nil {
		return err
	}
	if d.operation, err = d.configure(); err != nil {
		return err
	}
	if err = d.logDefaultEnvironment(); err != nil {
		return err
	}
	deploymentContext := htup.WithDeploymentRoot(ctx, d.Build.Config.URI.Path)
	deploymentContext, writer, finish := htup.BeginDeployment(deploymentContext, d.operation.Deployment.Environment, d.Writer)
	defer func() { finish(err) }()
	d.operation.Writer = writer
	d.operation.Secrets = d.Secrets
	return d.operation.Run(deploymentContext)
}

func (d *Deployment) configure() (*hsync.Deployment, error) {
	var kind htup.DeploymentKind = htup.CompleteDeployment // default
	var environment string = d.Config.Environment
	var providers htup.HostingProviders
	if err := d.validate(); err != nil {
		return nil, err
	}
	if len(d.Config.Providers) > 0 {
		selected, err := d.Build.Website.DeploymentProviders(d.Config.Providers)
		if err != nil {
			return nil, err
		}
		providers, err = selected.Select(d.Build.Website.Providers)
		if err != nil {
			return nil, err
		}
	} else {
		if environment == "" {
			environment = "default"
		}
		if _, err := d.Build.Website.DeploymentEnvironment(environment); err != nil {
			return nil, err
		}
	}

	if d.Build.Config.Incremental {
		kind = htup.IncrementalDeployment
	}
	config, err := (SyncConfigManager{}).Read("", d.Build.Website.URI.Path)
	if err != nil {
		return nil, err
	}
	return hsync.NewDeploymentWithConfig(&htup.Deployment{
		ConfigPath:  d.Build.Website.URI.Path,
		PayloadPath: d.Build.Config.URI.Path,
		Providers:   providers,
		Environment: environment,
		Kind:        kind,
	}, config), nil
}

func (d *Deployment) validate() error {
	if d.Config.Environment != "" && len(d.Config.Providers) > 0 {
		return fmt.Errorf("providers and environment are mutually exclusive")
	}
	return nil
}

func (d *Deployment) logDefaultEnvironment() error {
	if d.Writer == nil || d.Config.Environment != "" || len(d.Config.Providers) != 0 {
		return nil
	}
	_, err := fmt.Fprintf(d.Writer, "No deployment target specified; using default environment %q\n", "default")
	return err
}
