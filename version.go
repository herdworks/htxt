package hypertexting

import (
	"time"
)

// ============================ Package Variables ============================
var (
	LibVersion        = "v0.22.0"
	LibReleaseDate, _ = time.Parse(time.RFC3339, "2026-04-28T12:00:00-07:00") // Usage: LibReleaseDate.Format(time.RFC3339)
)
