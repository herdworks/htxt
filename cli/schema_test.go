package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/herdworks/clic"
	"github.com/herdworks/htxt"
)

func TestSchemaListUsesAlignedColumns(t *testing.T) {
	entries := htxt.SchemaRegistry.Entries()
	command := SchemaListCommand()
	var output strings.Builder
	if err := command.Spec.Handler(&clic.Execution{Stdout: &output}); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(output.String(), "\n"), "\n")
	if len(lines) != len(entries)+2 {
		t.Fatalf("schema list has %d lines, want %d", len(lines), len(entries)+2)
	}
	descriptionColumn := strings.Index(lines[0], "Description")
	if descriptionColumn <= len("Name") || !strings.HasPrefix(lines[1], "----") {
		t.Fatalf("schema list header is not tabular: %q / %q", lines[0], lines[1])
	}
	for index, entry := range entries {
		if got := strings.Index(lines[index+2], entry.Summary); got != descriptionColumn {
			t.Fatalf("%s description starts in column %d, want %d", entry.Name, got, descriptionColumn)
		}
	}
}

func TestSchemaCommandDescribesSchema(t *testing.T) {
	var output bytes.Buffer
	result := CLI.Run(clic.Execution{
		Context: context.Background(),
		Stdout:  &output,
	}, clic.Arguments{"schema", "describe", "website"})
	if result.Error != nil {
		t.Fatalf("htxt schema describe website = %#v", result)
	}
	var document map[string]any
	if err := json.Unmarshal(output.Bytes(), &document); err != nil {
		t.Fatalf("schema output is not JSON: %v", err)
	}
	if document["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
		t.Fatalf("schema = %#v", document["$schema"])
	}
	result = CLI.Run(clic.Execution{Context: context.Background()}, clic.Arguments{"schema", "describe", "unknown"})
	if result.Error == nil {
		t.Fatal("htxt schema describe unknown succeeded")
	}
}
