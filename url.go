package htxt

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

// cleanRelativePath normalizes a slash-delimited relative path for safe use
// beneath a managed root. It rejects absolute paths and parent traversal that
// could escape that root.
func cleanRelativePath(value string) (string, error) {
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
