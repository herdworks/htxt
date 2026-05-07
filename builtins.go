package hypertexting

import (
	"embed"

	hypertemplates "github.com/herdworks/hypertemplates-go"
)

//go:embed embedded
var embeddedFS embed.FS

func init() {
	hypertemplates.BuiltinsFS = hypertemplates.MustSubFS(embeddedFS, "embedded")
}
