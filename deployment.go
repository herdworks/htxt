package htxt

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/herdworks/hsync"
	"github.com/herdworks/htup"
	href "github.com/herdworks/htxt/href"
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
	if d.Config.Build.ID == "" {
		d.Config.Build.ID = uuid.NewString()
	}
	website := &Website{URI: &href.Href{URL: url.URL{Path: d.Config.Build.WebsiteConfig.Path}}}
	if err = website.ResolveURI(); err != nil {
		return err
	}
	d.Config.Build.WebsiteConfig.Path = website.URI.Path
	if err = website.LoadConfig(&d.Config.Build.WebsiteConfig); err != nil {
		return err
	}
	if d.Config.Build.URI == nil {
		d.Config.Build.URI = &url.URL{Path: filepath.Join(website.DeploymentsDirPath(), d.Config.Build.ID)}
	}
	d.Build = &Build{Config: d.Config.Build}
	d.Build.Website = website
	result := d.deploymentResult(htup.DeploymentRunning, nil, "")
	if err = htup.WriteDeploymentResult(d.Config.Build.URI.Path, result); err != nil {
		return fmt.Errorf("write initial deployment result: %w", err)
	}
	defer func() {
		completedAt := time.Now().UTC()
		status, message := htup.DeploymentSucceeded, ""
		if err != nil {
			status, message = htup.DeploymentFailed, err.Error()
		}
		result := d.deploymentResult(status, &completedAt, message)
		if result.StartedAt == nil {
			startedAt := completedAt
			result.StartedAt = &startedAt
		}
		if resultErr := htup.WriteDeploymentResult(d.Config.Build.URI.Path, result); resultErr != nil {
			err = errors.Join(err, fmt.Errorf("write final deployment result: %w", resultErr))
		}
	}()
	if err = d.Build.Execute(ctx); err != nil {
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

func (d *Deployment) deploymentResult(status htup.DeploymentStatus, completedAt *time.Time, message string) htup.DeploymentResult {
	startedAt := time.Now().UTC()
	if existing, err := htup.LoadDeploymentResult(d.Config.Build.URI.Path); err == nil && existing.StartedAt != nil {
		startedAt = *existing.StartedAt
	}
	kind := htup.CompleteDeployment
	if d.Config.Build.Incremental {
		kind = htup.IncrementalDeployment
	}
	result := htup.DeploymentResult{
		ID:          d.Config.Build.ID,
		Status:      status,
		StartedAt:   &startedAt,
		CompletedAt: completedAt,
		Error:       message,
		Environment: d.Config.Environment,
		Providers:   append([]string(nil), d.Config.Providers...),
		Kind:        kind,
	}
	if result.Environment == "" && len(result.Providers) == 0 {
		result.Environment = "default"
	}
	if existing, err := htup.LoadDeploymentResult(d.Config.Build.URI.Path); err == nil {
		result.DeploymentInstructions = existing.DeploymentInstructions
	}
	return result
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
