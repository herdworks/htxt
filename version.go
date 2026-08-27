package htxt

import (
	"time"
)

// ============================ Package Variables ============================
var (
	LibVersion        = "v0.47.0"
	LibReleaseDate, _ = time.Parse(time.RFC3339, "2026-08-27T11:00:00-07:00") // Usage: LibReleaseDate.Format(time.RFC3339)
)
