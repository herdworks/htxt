package htxt

import (
	"time"
)

// ============================ Package Variables ============================
var (
	LibVersion        = "v0.44.3"
	LibReleaseDate, _ = time.Parse(time.RFC3339, "2026-08-18T13:00:00-07:00") // Usage: LibReleaseDate.Format(time.RFC3339)
)
