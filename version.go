package htxt

import (
	"time"
)

// ============================ Package Variables ============================
var (
	LibVersion        = "v0.48.1"
	LibReleaseDate, _ = time.Parse(time.RFC3339, "2026-09-07T17:00:00-07:00") // Usage: LibReleaseDate.Format(time.RFC3339)
)
