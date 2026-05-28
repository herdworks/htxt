package hypertexting

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	hypertemplates "github.com/herdworks/hypertemplates-go"
	"github.com/herdworks/hypertemplates-go/data"
	"gopkg.in/yaml.v3"
)

// NamespacedData wraps a data.TemplateData with SSG-flavored helpers that
// implement CLI ColumnListable conventions for the namespace columns.
type NamespacedData data.TemplateData

// ColumnValues implements cli.ColumnListable by returning the wrapped data's
// top-level keys when field is "namespace" or "namespaces". All other field
// names return nil.
func (n NamespacedData) ColumnValues(field string) []string {
	switch strings.ToLower(field) {
	case "namespace", "namespaces":
		return data.TemplateData(n).Keys("")
	}
	return nil
}

// OrderedTemplateData wraps a TemplateData to control YAML marshal field
// ordering. Known Page-schema fields are emitted first, in declaration order,
// followed by remaining fields. Use it as a one-shot conversion at the
// marshal site:
//
//	yaml.Marshal(hypertexting.OrderedTemplateData(page.TemplateData))
type OrderedTemplateData data.TemplateData

// orderedFields is the canonical Page-schema field order. Fields not in this
// list are emitted after the ordered ones in map iteration order.
var orderedFields = []string{
	"created_at", "updated_at", "layout", "path", "slug",
	"byline", "contributors", "title", "description", "summary",
	"more", "tags", "unlisted", "redirect", "attachments",
}

// MarshalYAML implements yaml.Marshaler so that yaml.Marshal emits known
// Page-schema fields first in canonical order.
func (d OrderedTemplateData) MarshalYAML() (any, error) {
	output := &yaml.Node{Kind: yaml.MappingNode}
	for _, k := range orderedFields {
		v, ok := d[k]
		if !ok {
			continue
		}
		key := &yaml.Node{Kind: yaml.ScalarNode, Value: k}
		value := &yaml.Node{}
		if err := value.Encode(v); err != nil {
			return nil, err
		}
		output.Content = append(output.Content, key, value)
	}
	for k, v := range d {
		if slices.Contains(orderedFields, k) {
			continue
		}
		key := &yaml.Node{Kind: yaml.ScalarNode, Value: k}
		value := &yaml.Node{}
		if err := value.Encode(v); err != nil {
			return nil, err
		}
		output.Content = append(output.Content, key, value)
	}
	return output, nil
}

// namespaceSegmentRegexp constrains a single namespace path segment to
// letters, digits, and underscores. Segments are looked up as TemplateData
// map keys (not Go identifiers); the regex rejects whitespace, dots,
// hyphens, and any other character that would interact awkwardly with the
// keypath syntax.
var namespaceSegmentRegexp = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

// dataFileExts lists the extensions ParseTemplateDataFile understands. The
// loader walks the data directory recursively and rejects any *.<ext> file
// whose <ext> is not in this set (computed-data scripts use .js and are
// handled separately).
var dataFileExts = map[string]struct{}{
	".yaml": {},
	".yml":  {},
	".json": {},
	".opml": {},
}

// dataExtRegexp matches the file extensions ParseTemplateDataFile handles.
var dataExtRegexp = `.*\.(yaml|yml|json|opml)$`

// computedExtRegexp matches computed-data script files.
var computedExtRegexp = `.*\.js$`

// namespaceSegmentsFromRelPath turns a slash-separated relative path like
// "social/twitter.yaml" into the keypath segments ["social", "twitter"].
// Returns an error if any segment is empty, contains a dot, has whitespace,
// or otherwise fails namespaceSegmentRegexp.
func namespaceSegmentsFromRelPath(rel string) ([]string, error) {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return nil, fmt.Errorf("empty path")
	}
	parts := strings.Split(rel, "/")
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty path")
	}
	// Strip the extension from the final segment.
	last := parts[len(parts)-1]
	ext := filepath.Ext(last)
	if ext != "" {
		parts[len(parts)-1] = strings.TrimSuffix(last, ext)
	}
	for _, seg := range parts {
		if !namespaceSegmentRegexp.MatchString(seg) {
			return nil, fmt.Errorf("invalid namespace segment %q (must match %s)", seg, namespaceSegmentRegexp.String())
		}
	}
	return parts, nil
}

// setNested walks segments into root, creating intermediate maps as needed,
// and sets the final segment to value. Callers are expected to have run
// scanNamespaceTree first; intermediates are always created by setNested
// itself, so a non-map value at an intermediate keypath would mean the
// scanner missed a file-vs-directory collision.
func setNested(root hypertemplates.TemplateData, segments []string, value any) error {
	if len(segments) == 0 {
		return fmt.Errorf("setNested: empty segments")
	}
	cur := root
	for _, seg := range segments[:len(segments)-1] {
		next, exists := cur[seg]
		if !exists {
			child := hypertemplates.TemplateData{}
			cur[seg] = child
			cur = child
			continue
		}
		cur = next.(hypertemplates.TemplateData)
	}
	leaf := segments[len(segments)-1]
	if _, exists := cur[leaf]; exists {
		return fmt.Errorf("namespace %q already set", strings.Join(segments, "."))
	}
	cur[leaf] = value
	return nil
}

// scanNamespaceTree walks dataDir to detect collisions between files and
// directories that would resolve to the same namespace path. Returns nil if
// the tree is consistent. Any collision (a file <name>.<ext> and a sibling
// directory <name>/) is reported with both source paths cited.
func scanNamespaceTree(dataDir string) error {
	dataAbs, err := filepath.Abs(dataDir)
	if err != nil {
		return fmt.Errorf("resolve data root %q: %w", dataDir, err)
	}
	info, err := os.Stat(dataAbs)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("stat data root %q: %w", dataAbs, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("data root %q is not a directory", dataAbs)
	}

	type claim struct {
		path string
		dir  bool
	}
	claims := map[string]claim{}

	walkErr := filepath.WalkDir(dataAbs, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == dataAbs {
			return nil
		}
		rel, relErr := filepath.Rel(dataAbs, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)

		if entry.IsDir() {
			segments, segErr := namespaceSegmentsFromDirRelPath(rel)
			if segErr != nil {
				return fmt.Errorf("data dir %q: %w", rel, segErr)
			}
			key := strings.Join(segments, ".")
			if prev, exists := claims[key]; exists {
				return fmt.Errorf("data: namespace %q claimed by both %q and %q", key, prev.path, rel)
			}
			claims[key] = claim{path: rel, dir: true}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(rel))
		_, isData := dataFileExts[ext]
		isComputed := ext == ".js"
		if !isData && !isComputed {
			return nil
		}
		segments, segErr := namespaceSegmentsFromRelPath(rel)
		if segErr != nil {
			return fmt.Errorf("data file %q: %w", rel, segErr)
		}
		key := strings.Join(segments, ".")
		if prev, exists := claims[key]; exists {
			return fmt.Errorf("data: namespace %q claimed by both %q and %q", key, prev.path, rel)
		}
		claims[key] = claim{path: rel}
		return nil
	})
	return walkErr
}

// namespaceSegmentsFromDirRelPath validates each segment of a directory's
// relative path. Like namespaceSegmentsFromRelPath but does not strip an
// extension (directories don't have one for our purposes).
func namespaceSegmentsFromDirRelPath(rel string) ([]string, error) {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return nil, fmt.Errorf("empty path")
	}
	parts := strings.Split(rel, "/")
	for _, seg := range parts {
		if !namespaceSegmentRegexp.MatchString(seg) {
			return nil, fmt.Errorf("invalid namespace segment %q (must match %s)", seg, namespaceSegmentRegexp.String())
		}
	}
	return parts, nil
}

// loadStaticData walks dataDir for *.yaml, *.json, *.opml (etc.) files and
// installs each parsed TemplateData under the keypath derived from its
// filename. Subdirectories nest.
func loadStaticData(website *Website, dataDir string, config *hypertemplates.HyperMarkConfig, verbose bool, w io.Writer) (hypertemplates.TemplateData, error) {
	out := hypertemplates.TemplateData{}

	if err := scanNamespaceTree(dataDir); err != nil {
		return nil, err
	}

	paths, err := hypertemplates.ListFilesByPattern(dataDir, true, dataExtRegexp, "")
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)

	// Track which file first claimed each namespace key so collisions cite
	// both contributing paths.
	claimants := map[string]string{}

	for _, p := range paths {
		absPath, err := hypertemplates.LocalFilePath(dataDir, p)
		if err != nil {
			return nil, err
		}
		segments, err := namespaceSegmentsFromRelPath(p)
		if err != nil {
			return nil, fmt.Errorf("data %q: %w", p, err)
		}
		key := strings.Join(segments, ".")
		if prev, claimed := claimants[key]; claimed {
			return nil, fmt.Errorf("data: namespace %q defined by both %q and %q; remove one", key, prev, p)
		}
		claimants[key] = p

		if verbose && website != nil {
			fmt.Fprintf(w, "data \"%s\" loading\n", website.RelativePath(absPath))
		}
		parsed, err := hypertemplates.ParseTemplateDataFile(absPath, config)
		if err != nil {
			return nil, fmt.Errorf("data %q: %w", p, err)
		}
		if err := setNested(out, segments, parsed); err != nil {
			return nil, fmt.Errorf("data %q: %w", p, err)
		}
	}
	return out, nil
}

// generateComputedData discovers *.js files in the website's data directory and
// executes each as a "seeded" script. Each file must `export default` a
// function; its return value is installed under the keypath derived from
// the file's path. The script sees the static-data context
// (ht/build/env/site/data/theme) as ambient bindings.
//
// Computed data must run AFTER all static data is loaded and after pages
// have been discovered/loaded — site.pages, in particular, is what gives
// computed scripts their primary purpose. The build's Execute method
// invokes this between Pages.ResolveTemplateData and WritePages.
func generateComputedData(build *Build, verbose bool, w io.Writer) error {
	if build.Website == nil {
		return nil
	}
	dataDir := build.Website.DataDirPath()

	dataAbs, err := filepath.Abs(dataDir)
	if err != nil {
		return fmt.Errorf("resolve data root %q: %w", dataDir, err)
	}
	if info, statErr := os.Stat(dataAbs); os.IsNotExist(statErr) {
		return nil
	} else if statErr != nil {
		return fmt.Errorf("stat data root %q: %w", dataAbs, statErr)
	} else if !info.IsDir() {
		return fmt.Errorf("data root %q is not a directory", dataAbs)
	}

	paths, err := hypertemplates.ListFilesByPattern(dataDir, true, computedExtRegexp, "")
	if err != nil {
		return err
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		return nil
	}

	// Snapshot the bindings once. site.pages and feed/tag pages are populated
	// before this runs (see Build.Execute).
	bindings := map[string]any(build.Context())

	for _, p := range paths {
		absPath, err := hypertemplates.LocalFilePath(dataDir, p)
		if err != nil {
			return err
		}
		segments, err := namespaceSegmentsFromRelPath(p)
		if err != nil {
			return fmt.Errorf("computed data %q: %w", p, err)
		}
		if verbose {
			fmt.Fprintf(w, "computed data \"%s\" loading\n", build.Website.RelativePath(absPath))
		}
		plugin, err := data.LoadPlugin(absPath, bindings)
		if err != nil {
			return fmt.Errorf("computed data %q: %w", p, err)
		}
		result, err := plugin.Run(context.Background())
		if err != nil {
			return fmt.Errorf("computed data %q: %w", p, err)
		}
		// Install into website.GlobalData so per-page TemplateData picks it
		// up via the existing TemplateData() helper.
		if err := setNested(build.Website.GlobalData, segments, result); err != nil {
			return fmt.Errorf("computed data %q: %w", p, err)
		}
	}
	return nil
}
