package hypertexting

import (
	"testing"

	hyperproviders "github.com/herdworks/hyperproviders-go"
)

func TestWebsite_ResolveEnvironmentsBackfillsName(t *testing.T) {
	website := &Website{
		Environments: map[string]*hyperproviders.Environment{
			"production": {},
			"staging":    {Name: "preview"},
		},
	}

	website.ResolveEnvironments()

	if got := website.Environments["production"].Name; got != "production" {
		t.Fatalf("production name = %q, want production", got)
	}
	if got := website.Environments["staging"].Name; got != "preview" {
		t.Fatalf("staging name = %q, want preview", got)
	}
}

func TestWebsite_DeploymentEnvironment(t *testing.T) {
	website := &Website{
		Environments: map[string]*hyperproviders.Environment{
			"production": {},
		},
	}

	env, err := website.DeploymentEnvironment("production")
	if err != nil {
		t.Fatalf("DeploymentEnvironment: %v", err)
	}
	if env.Name != "production" {
		t.Fatalf("Name = %q, want production", env.Name)
	}

	if _, err := website.DeploymentEnvironment(""); err == nil {
		t.Fatal("DeploymentEnvironment empty name error = nil, want error")
	}
	if _, err := website.DeploymentEnvironment("missing"); err == nil {
		t.Fatal("DeploymentEnvironment missing name error = nil, want error")
	}

	website.Environments["empty"] = nil
	if _, err := website.DeploymentEnvironment("empty"); err == nil {
		t.Fatal("DeploymentEnvironment empty error = nil, want error")
	}
}
