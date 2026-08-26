package htxt

import (
	"embed"
	"io/fs"

	httheme "github.com/herdworks/httt/theme"
)

const builtinStaticDir = "builtins/static"

//go:embed embedded
var embeddedFS embed.FS

var builtinsFS fs.FS = httheme.MustSubFS(embeddedFS, "embedded")

func hydrateThemeBuiltins(theme *httheme.Theme) {
	if theme != nil && theme.BuiltinsFS == nil {
		theme.BuiltinsFS = builtinsFS
	}
}
