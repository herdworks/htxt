package hypertexting

import (
	"embed"

	httheme "github.com/herdworks/hypertemplates-go/theme"
)

//go:embed embedded
var embeddedFS embed.FS

func init() {
	httheme.BuiltinsFS = httheme.MustSubFS(embeddedFS, "embedded")
}
