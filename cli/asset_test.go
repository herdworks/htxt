package cli

import (
	"testing"

	"github.com/herdworks/clic"
)

func TestAssetCommandRegistersAssetWorkflow(t *testing.T) {
	command := AssetCommand()
	for _, key := range []string{"add", "ls", "lookup"} {
		if _, ok := command.SubCommand(key); !ok {
			t.Errorf("asset command %q is not registered", key)
		}
	}
}

func TestAssetCommandPositionalArgumentValidation(t *testing.T) {
	tests := []struct {
		name      string
		command   clic.Command
		arguments clic.Arguments
		valid     bool
	}{
		{name: "add", command: AssetAddCommand(), arguments: clic.Arguments{"asset.css", "static/asset.css"}, valid: true},
		{name: "add missing path", command: AssetAddCommand(), arguments: clic.Arguments{"asset.css"}},
		{name: "lookup", command: AssetLookupCommand(), arguments: clic.Arguments{"asset.css"}, valid: true},
		{name: "lookup missing path", command: AssetLookupCommand()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			execution := &clic.Execution{
				Command: &test.command,
			}
			execution.Command.State.Arguments = test.arguments
			err := test.command.Spec.Validator(execution)
			if (err == nil) != test.valid {
				t.Fatalf("validator error = %v, valid = %t", err, test.valid)
			}
		})
	}
}
