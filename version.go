package htxt

import (
	"time"
)

// ============================ Package Variables ============================
var (
	LibVersion        = "v0.48.6"
	LibReleaseDate, _ = time.Parse(time.RFC3339, "2026-09-10T15:00:00-07:00") // Usage: LibReleaseDate.Format(time.RFC3339)
)
