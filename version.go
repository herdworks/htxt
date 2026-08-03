package hypertexting

import (
	"time"
)

// ============================ Package Variables ============================
var (
	LibVersion        = "v0.39.0"
	LibReleaseDate, _ = time.Parse(time.RFC3339, "2026-08-03T11:00:00-07:00") // Usage: LibReleaseDate.Format(time.RFC3339)
)
