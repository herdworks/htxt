package htxt

import (
	"embed"

	httheme "github.com/herdworks/httt/theme"
)

//go:embed embedded
var embeddedFS embed.FS

func init() {
	httheme.BuiltinsFS = httheme.MustSubFS(embeddedFS, "embedded")
}
