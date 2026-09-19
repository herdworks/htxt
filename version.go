package htxt

import (
	"time"
)

// ============================ Package Variables ============================
var (
	LibVersion        = "v0.49.3"
	LibReleaseDate, _ = time.Parse(time.RFC3339, "2026-09-19T11:00:00-07:00") // Usage: LibReleaseDate.Format(time.RFC3339)
)
