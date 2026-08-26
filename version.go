package htxt

import (
	"time"
)

// ============================ Package Variables ============================
var (
	LibVersion        = "v0.46.0"
	LibReleaseDate, _ = time.Parse(time.RFC3339, "2026-08-26T16:00:00-07:00") // Usage: LibReleaseDate.Format(time.RFC3339)
)
