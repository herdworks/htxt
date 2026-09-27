package htxt

import (
	"time"
)

// ============================ Package Variables ============================
var (
	LibVersion        = "v0.50.5"
	LibReleaseDate, _ = time.Parse(time.RFC3339, "2026-09-27T12:00:00-07:00") // Usage: LibReleaseDate.Format(time.RFC3339)
)
