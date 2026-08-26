package cli

import (
	"net/url"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/herdworks/clic"
	"github.com/herdworks/htxt"
	href "github.com/herdworks/htxt/href"
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

func TestAssetListUsesWebsiteRelativeDiskSources(t *testing.T) {
	root := t.TempDir()
	website := &htxt.Website{URI: &href.Href{URL: url.URL{Path: filepath.Join(root, "site.yaml")}}}
	list := assetList{
		website: website,
		assets: htxt.Assets{
			{Source: htxt.WebsiteAsset, SourcePath: filepath.Join(root, "static", "site.css"), PublicPath: "/site.css"},
			{Source: htxt.BuiltinAsset, SourcePath: "builtins/static/css/reset.css", PublicPath: "/css/reset.css"},
		},
	}
	if got, want := list.ColumnValues("source"), []string{"static/site.css", "builtins/static/css/reset.css"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ColumnValues(source) = %#v, want %#v", got, want)
	}
}
