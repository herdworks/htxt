package cli

import (
	"testing"

	"github.com/herdworks/clic"
	"github.com/herdworks/htup"
)

func TestDeployCommandExcludesPreviewOption(t *testing.T) {
	command := DeployCommand()
	if _, ok := command.Spec.Options["preview"]; ok {
		t.Fatal("deploy command exposes a preview option")
	}
}

func TestDeployValidator(t *testing.T) {
	tests := []struct {
		name    string
		options clic.Options
		want    string
	}{
		{name: "no target"},
		{
			name: "environment and providers",
			options: clic.Options{
				"environment": {State: clic.OptionState{Value: "production"}},
				"providers":   {State: clic.OptionState{Value: []string{"pages"}}},
			},
			want: "providers and environment are mutually exclusive",
		},
		{
			name: "invalid incremental build",
			options: clic.Options{
				"incremental": {State: clic.OptionState{Value: true}},
			},
			want: "--page is required when --incremental is set",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := deployValidator(&clic.Execution{Options: test.options})
			if test.want == "" && err != nil {
				t.Fatalf("deployValidator() error = %v", err)
			}
			if test.want != "" && (err == nil || err.Error() != test.want) {
				t.Fatalf("deployValidator() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestDeploymentSecretsUsesEnvironmentThenWebsiteFiles(t *testing.T) {
	secrets := deploymentSecrets("")
	if len(secrets) != 2 {
		t.Fatalf("deploymentSecrets(nil) = %#v, want environment and files providers", secrets)
	}
	for index, name := range []string{"env", "files"} {
		if secrets[index].Name() != name {
			t.Errorf("secret provider %d = %q, want %q", index, secrets[index].Name(), name)
		}
	}
	var _ htup.SecretsProvider = secrets[0]
}
