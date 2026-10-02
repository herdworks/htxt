package cli

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/herdworks/clic"
	"github.com/herdworks/htxt"
	href "github.com/herdworks/htxt/href"
)

const defaultWebsiteDescription = "This website is powered by HyperTexting."

// InitCommand initializes a website in the current or selected directory.
func InitCommand() clic.Command {
	return clic.Command{
		Meta: clic.CommandMeta{
			Description: "Initialize a new website",
			Usage: []string{
				"{{ .Command }} [directory] [options]",
			},
			Order: 1,
		},
		Spec: clic.CommandSpec{
			Key:  "init",
			Help: clic.DefaultHelp,
			Options: clic.MustOptionsFrom(
				InitTitleOption,
				InitDescriptionOption,
				InitBaseURLOption,
			),
			Validator: initValidator,
			Handler:   initHandler,
		},
	}
}

func InitTitleOption() clic.Option {
	return clic.Option{
		Meta: clic.OptionMeta{
			ShortName:   "t",
			Description: "Website title (defaults to the directory name).",
		},
		Spec: clic.OptionSpec{
			Key:  "title",
			Kind: clic.StringOption,
		},
	}
}

func InitDescriptionOption() clic.Option {
	return clic.Option{
		Meta: clic.OptionMeta{
			ShortName:   "d",
			Description: "Website description.",
			Default:     defaultWebsiteDescription,
		},
		Spec: clic.OptionSpec{
			Key:  "description",
			Kind: clic.StringOption,
		},
	}
}

func InitBaseURLOption() clic.Option {
	return clic.Option{
		Meta: clic.OptionMeta{
			ShortName:   "b",
			Description: "Website base URL.",
			Default:     "http://localhost",
		},
		Spec: clic.OptionSpec{
			Key:  "baseurl",
			Kind: clic.StringOption,
		},
	}
}

func initValidator(execution *clic.Execution) error {
	if execution.Command.PositionalArguments() > 1 {
		return positionalArgumentsError(execution, 1)
	}
	baseURL, err := url.Parse(execution.Options["baseurl"].StringValue())
	if err != nil || (baseURL.Scheme != "http" && baseURL.Scheme != "https") || baseURL.Host == "" {
		return fmt.Errorf("baseurl %q must be an absolute HTTP(S) URL", execution.Options["baseurl"].StringValue())
	}
	return nil
}

func initHandler(execution *clic.Execution) error {
	directory, err := initDirectory(execution)
	if err != nil {
		return err
	}
	if err = websiteConfigAbsent(directory); err != nil {
		return err
	}
	title := execution.Options["title"].StringValue()
	if title == "" {
		title = filepath.Base(directory)
	}
	website := htxt.Website{
		URI:         &href.Href{URL: url.URL{Path: filepath.Join(directory, "site.yaml")}},
		BaseURL:     execution.Options["baseurl"].StringValue(),
		Title:       title,
		Description: execution.Options["description"].StringValue(),
		Favicon:     htxt.WebsiteFavicon{Href: "/favicon.png"},
		Byline:      htxt.Byline{Name: title, Favicon: "/favicon.png", Href: "/"},
	}
	website.ResolveConfig(nil)
	return website.CreatePackage(execution.Stdout)
}

func initDirectory(execution *clic.Execution) (string, error) {
	directory := execution.WorkingDirectory
	if execution.Command.PositionalArguments() == 1 {
		directory = execution.Command.State.Arguments[0]
	}
	if directory == "" {
		return "", fmt.Errorf("working directory is required")
	}
	if !filepath.IsAbs(directory) {
		directory = filepath.Join(execution.WorkingDirectory, directory)
	}
	directory, err := filepath.Abs(directory)
	if err != nil {
		return "", err
	}
	if info, err := os.Stat(directory); err == nil && !info.IsDir() {
		return "", fmt.Errorf("init path %q is not a directory", directory)
	} else if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	return directory, nil
}

func websiteConfigAbsent(directory string) error {
	for _, name := range []string{"site.yaml", "site.yml", "site.json"} {
		path := filepath.Join(directory, name)
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("website configuration already exists at %q", path)
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
