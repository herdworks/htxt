package htxt

import (
	"fmt"
	"strings"
	"text/template"
	"text/template/parse"
)

func renderTemplate(source string, funcs template.FuncMap) (string, error) {
	tmpl, err := template.New("template").Delims("${", "}").Funcs(funcs).Parse(source)
	if err != nil {
		return "", normalizeTemplateParseError(err)
	}
	if err := validateTemplateNode(tmpl.Tree.Root, funcs); err != nil {
		return "", err
	}

	var rendered strings.Builder
	if err := tmpl.Execute(&rendered, nil); err != nil {
		return "", fmt.Errorf("keyword expansion failed")
	}
	return rendered.String(), nil
}

func normalizeTemplateParseError(err error) error {
	message := err.Error()
	start := strings.Index(message, "function \"")
	if start == -1 {
		return fmt.Errorf("invalid keyword expression")
	}
	start += len("function \"")
	end := strings.Index(message[start:], "\"")
	if end == -1 {
		return fmt.Errorf("invalid keyword expression")
	}
	keyword := message[start : start+end]
	if keyword == "" || !strings.Contains(message[start+end:], " not defined") {
		return fmt.Errorf("invalid keyword expression")
	}
	return fmt.Errorf("undefined keyword %q", keyword)
}

func validateTemplateNode(node parse.Node, funcs template.FuncMap) error {
	switch n := node.(type) {
	case nil:
		return nil
	case *parse.ListNode:
		for _, child := range n.Nodes {
			if err := validateTemplateNode(child, funcs); err != nil {
				return err
			}
		}
		return nil
	case *parse.TextNode:
		return nil
	case *parse.ActionNode:
		return validateTemplatePipe(n.Pipe, funcs)
	default:
		return fmt.Errorf("invalid keyword expression")
	}
}

func validateTemplatePipe(pipe *parse.PipeNode, funcs template.FuncMap) error {
	if pipe == nil || len(pipe.Decl) > 0 || len(pipe.Cmds) == 0 {
		return fmt.Errorf("invalid keyword expression")
	}
	if len(pipe.Cmds) != 1 {
		return fmt.Errorf("keyword expressions cannot be chained")
	}
	cmd := pipe.Cmds[0]
	if cmd == nil || len(cmd.Args) == 0 {
		return fmt.Errorf("invalid keyword expression")
	}
	identifier, ok := cmd.Args[0].(*parse.IdentifierNode)
	if !ok {
		return fmt.Errorf("invalid keyword expression")
	}
	if _, ok := funcs[identifier.Ident]; !ok {
		return fmt.Errorf("undefined keyword %q", identifier.Ident)
	}
	if len(cmd.Args) != 1 {
		return fmt.Errorf("keyword %q does not accept arguments", identifier.Ident)
	}
	return nil
}
