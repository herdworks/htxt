package htxt

import (
	"time"
)

// ============================ Package Variables ============================
var (
	LibVersion        = "v0.48.4"
	LibReleaseDate, _ = time.Parse(time.RFC3339, "2026-09-07T23:00:00-07:00") // Usage: LibReleaseDate.Format(time.RFC3339)
)
