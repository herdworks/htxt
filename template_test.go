package hypertexting

import (
	"strings"
	"testing"
	"text/template"
)

func TestRenderTemplateUsesKeywordErrors(t *testing.T) {
	funcs := template.FuncMap{
		"year": func() string { return "2026" },
	}
	if rendered, err := renderTemplate("posts/${year}", funcs); err != nil || rendered != "posts/2026" {
		t.Fatalf("renderTemplate() = %q, %v; want posts/2026, nil", rendered, err)
	}

	tests := []struct {
		name string
		src  string
		want string
	}{
		{name: "undefined keyword", src: "posts/${hour}", want: `undefined keyword "hour"`},
		{name: "builtin keyword", src: `posts/${printf "x"}`, want: `undefined keyword "printf"`},
		{name: "argument", src: `posts/${year "x"}`, want: `keyword "year" does not accept arguments`},
		{name: "chained expression", src: `posts/${year | printf "%s"}`, want: "keyword expressions cannot be chained"},
		{name: "accessor", src: "posts/${.Title}", want: "invalid keyword expression"},
		{name: "unterminated", src: "posts/${year", want: "invalid keyword expression"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := renderTemplate(tc.src, funcs)
			if err == nil {
				t.Fatalf("renderTemplate() error = nil, want %q", tc.want)
			}
			message := err.Error()
			if !strings.Contains(message, tc.want) {
				t.Fatalf("renderTemplate() error = %q, want substring %q", message, tc.want)
			}
			for _, disallowed := range []string{"template", "function", "funcmap", "pipeline"} {
				if strings.Contains(message, disallowed) {
					t.Fatalf("renderTemplate() error = %q, should not contain %q", message, disallowed)
				}
			}
		})
	}
}
