package hypertexting

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

func cloneURL(u *url.URL) *url.URL {
	if u == nil {
		return nil
	}
	clone := *u
	return &clone
}

func cleanRelativeURIPath(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	cleaned := filepath.Clean(filepath.FromSlash(strings.Trim(value, "/")))
	if cleaned == "." {
		return "", nil
	}
	if filepath.IsAbs(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe relative path %q", value)
	}
	return cleaned, nil
}
