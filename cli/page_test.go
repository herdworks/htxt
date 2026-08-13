package cli

import (
	"testing"

	"github.com/herdworks/clic"
	"github.com/herdworks/htxt"
)

func TestFilterPagesByVisibility(t *testing.T) {
	auto := &htxt.Page{Visibility: htxt.AutoVisibility}
	public := &htxt.Page{Visibility: htxt.PublicVisibility}
	unlisted := &htxt.Page{Visibility: htxt.UnlistedVisibility}
	status := &htxt.Page{Visibility: htxt.StoryVisibility}
	draft := &htxt.Page{Visibility: htxt.PreviewVisibility}
	none := &htxt.Page{Visibility: htxt.HiddenVisibility}
	pages := htxt.Pages{auto, public, unlisted, status, draft, none}

	tests := map[string]htxt.Pages{
		"all":      pages,
		"public":   {auto, public},
		"unlisted": {unlisted},
		"status":   {status},
		"draft":    {draft},
		"none":     {none},
	}
	for visibility, want := range tests {
		got := filterPages(pages, visibility)
		if len(got) != len(want) {
			t.Errorf("filterPages(%q) returned %d pages, want %d", visibility, len(got), len(want))
			continue
		}
		for index := range want {
			if got[index] != want[index] {
				t.Errorf("filterPages(%q)[%d] = %#v, want %#v", visibility, index, got[index], want[index])
			}
		}
	}
}

func TestPageListValidatorAcceptsVisibilityFilters(t *testing.T) {
	command := PageListCommand()
	for _, visibility := range []string{"all", "public", "unlisted", "status", "draft", "none"} {
		execution := &clic.Execution{
			Command: &command,
			Options: clic.Options{
				"visibility": {
					State: clic.OptionState{
						Value: visibility,
					},
				},
			},
		}
		if err := command.Spec.Validator(execution); err != nil {
			t.Errorf("visibility %q rejected: %v", visibility, err)
		}
	}
}
