package htxt

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"

	"github.com/herdworks/httt"
	"github.com/herdworks/htxt/href"
	htpatch "github.com/herdworks/htxt/patch"
	htprompt "github.com/herdworks/htxt/prompt"
)

// =============================================================================
// Types
// =============================================================================

type ContentType struct {
	URI      *href.Href               `json:"uri" yaml:"-"`
	Metadata ContentTypeMetadata      `json:"metadata" yaml:"metadata"`
	Spec     ContentTypeSpecification `json:"spec" yaml:"spec"`
	State    ContentTypeState         `json:"-" yaml:"-"`
}
type ContentTypes []*ContentType

type ContentTypeMetadata struct {
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	Icon        string `json:"icon,omitempty" yaml:"icon,omitempty"`
	Action      string `json:"action,omitempty" yaml:"action,omitempty"`
}

type ContentTypeSpecification struct {
	Layout       string                    `json:"layout" yaml:"layout"`
	Path         string                    `json:"path" yaml:"path"`
	PathResolver PublicURLPathResolverKind `json:"path_resolver" yaml:"path_resolver"`
	Format       string                    `json:"format" yaml:"format"`
	Requires     ContentTypeRequirements   `json:"requires" yaml:"requires"`
	Visibility   PageVisibilityKind        `json:"visibility" yaml:"visibility" jsonschema:"description=Visibility for pages of this content type."`
	TemplateData httt.TemplateData         `json:"template" yaml:"template"`
	Prompts      htprompt.Prompts          `json:"prompts,omitempty" yaml:"prompts,omitempty"`
	Patches      htpatch.Patches           `json:"patches,omitempty" yaml:"patches,omitempty"`
}

type ContentTypeState struct {
	UserData map[string]any `json:"user_data,omitempty" yaml:"user_data,omitempty"`
}

type ContentTypeRequirements struct {
	Title       bool           `json:"title,omitempty" yaml:"title,omitempty"`
	Description bool           `json:"description,omitempty" yaml:"description,omitempty"`
	Content     bool           `json:"content,omitempty" yaml:"content,omitempty"`
	Redirect    bool           `json:"redirect,omitempty" yaml:"redirect,omitempty"`
	Attachment  AttachmentKind `json:"attachment,omitempty" yaml:"attachment,omitempty"`
}

// =============================================================================
// Variables
// =============================================================================

var draftTimestampURISuffixPattern = regexp.MustCompile(`^d[0-9a-f]{11}$`)

// =============================================================================
// Methods
// =============================================================================

func (types ContentTypes) ColumnValues(field string) []string {
	values := []string{}
	for _, t := range types {
		switch field {
		case "Name", "name":
			values = append(values, t.Metadata.Name)
		case "Description", "description":
			values = append(values, t.Metadata.Description)
		}
	}
	return values
}

func (contenttype ContentType) Ready(page *Page) error {
	var err error
	if err = contenttype.ValidateDefinition(); err != nil {
		return err
	}
	if page == nil {
		return fmt.Errorf("content-type requires page")
	}

	// title validation
	err = fmt.Errorf("content-type requires page.title property")
	if contenttype.Spec.Requires.Title && len(page.Title) == 0 {
		return err
	}
	if contenttype.Spec.PathResolver == TitleKabobPathResolver && len(page.Title) == 0 {
		return err
	}
	if contenttype.Spec.PathResolver == TitleSnakePathResolver && len(page.Title) == 0 {
		return err
	}

	// description validation
	err = fmt.Errorf("content-type requires page.description property")
	if contenttype.Spec.Requires.Description && len(page.Description) == 0 {
		return err
	}

	err = fmt.Errorf("content-type requires page.slug property")
	if contenttype.Spec.PathResolver == ManualPathResolver && len(page.Slug) == 0 {
		return err
	}

	// content validation
	err = fmt.Errorf("content-type requires page.content property")
	if contenttype.Spec.Requires.Content && len(page.Content) == 0 {
		return err
	}

	// redirect validation
	err = fmt.Errorf("content-type requires page.redirect property")
	if contenttype.Spec.Requires.Redirect && len(page.Redirect) == 0 {
		return err
	}

	// attachments validation
	err = fmt.Errorf("content-type requires page.attachments (expected 1 or more, got %v)", len(page.Attachments))
	if len(contenttype.Spec.Requires.Attachment) > 0 && len(page.Attachments) == 0 {
		return err
	}
	if contenttype.Spec.Requires.Attachment == AttachmentKindLink && len(page.Attachments.FilteredBy(AttachmentKindLink)) == 0 {
		return fmt.Errorf("content-type requires link attachment")
	}
	if contenttype.Spec.Requires.Attachment == AttachmentKindFile && len(page.Attachments.FilteredBy(AttachmentKindFile)) == 0 {
		return fmt.Errorf("content-type requires file attachment")
	}
	if contenttype.Spec.Requires.Attachment == AttachmentKindImage && len(page.Attachments.FilteredBy(AttachmentKindImage)) == 0 {
		return fmt.Errorf("content-type requires image attachment")
	}
	if contenttype.Spec.Requires.Attachment == AttachmentKindVideo && len(page.Attachments.FilteredBy(AttachmentKindVideo)) == 0 {
		return fmt.Errorf("content-type requires video attachment")
	}
	if contenttype.Spec.Requires.Attachment == AttachmentKindAudio && len(page.Attachments.FilteredBy(AttachmentKindAudio)) == 0 {
		return fmt.Errorf("content-type requires audio attachment")
	}
	if contenttype.Spec.Requires.Attachment == AttachmentKindDocument && len(page.Attachments.FilteredBy(AttachmentKindDocument)) == 0 {
		return fmt.Errorf("content-type requires document attachment")
	}

	// success!
	return nil
}

func (contenttype ContentType) TemplateData() (httt.TemplateData, error) {
	if err := contenttype.ValidateDefinition(); err != nil {
		return nil, err
	}
	if err := contenttype.State.Validate(contenttype.Spec.Prompts); err != nil {
		return nil, err
	}
	resolved := httt.TemplateData{}
	if contenttype.Spec.TemplateData != nil {
		resolved = contenttype.Spec.TemplateData.Clone()
	}
	if len(contenttype.Spec.Patches) == 0 {
		return resolved, nil
	}
	if err := htpatch.Apply(resolved, contenttype.Spec.Patches, htpatch.Values(contenttype.State.UserData)); err != nil {
		return nil, err
	}
	return resolved, nil
}

func (contenttype ContentType) OverlayPage(page *Page) (*Page, error) {
	if page == nil {
		return nil, fmt.Errorf("content-type requires page")
	}
	resolved, err := contenttype.TemplateData()
	if err != nil {
		return nil, err
	}
	candidate := page.Clone()
	if err := applyPageContentType(candidate, &contenttype); err != nil {
		return nil, err
	}
	candidate.ResolveTemplateData(nil)
	pageData := candidate.TemplateData
	if pageData == nil {
		pageData = httt.TemplateData{}
	}
	candidate.TemplateData = pageData.Merge(resolved)
	if err := candidate.Parse(candidate.TemplateData); err != nil {
		return nil, err
	}
	return candidate, nil
}

// PreviewPageData returns serialized page data after applying the content type
// overlay without mutating page or writing to disk.
func (contenttype ContentType) PreviewPageData(website *Website, page *Page) ([]byte, error) {
	if err := contenttype.ValidateDefinition(); err != nil {
		return nil, err
	}
	candidate, err := contenttype.OverlayPage(page)
	if err != nil {
		return nil, err
	}
	// TODO: Apply website-aware page timestamp normalization to candidate so
	// timestamp-derived paths work for a newly decoded page.
	// Preview serialization uses the content type's required visibility.
	candidate.Visibility = contenttype.Spec.Visibility
	return candidate.RenderData(website, contenttype.Spec.Format)
}

func (contenttype ContentType) CandidateReady(page *Page) error {
	candidate, err := contenttype.OverlayPage(page)
	if err != nil {
		return err
	}
	return contenttype.Ready(candidate)
}

func (contenttype *ContentType) Create(website *Website, format string, w io.Writer) (err error) {
	if w == nil {
		w = os.Stdout
	}

	var fileDirectory string = website.Theme.ContentTypesDirPath()
	var fileName string
	var fileExtension string
	switch format {
	case "json":
		fileExtension = ".json"
	case "yaml", "yml":
		fileExtension = ".yaml"
	default:
		return fmt.Errorf("unsupported content type format: %s", format)
	}
	fileName = strings.ToLower(contenttype.Metadata.Name)
	fileName = strings.ReplaceAll(fileName, " ", "-")
	contenttype.URI = &href.Href{URL: url.URL{Path: filepath.Join(fileDirectory, fileName+fileExtension)}}
	return contenttype.Save(website, w)
}

func (contenttype *ContentType) Save(website *Website, w io.Writer) (err error) {
	if w == nil {
		w = os.Stdout
	}

	var file *os.File
	var fileExt string = filepath.Ext(contenttype.URI.Path)
	var fileBytes []byte
	var fileBytesWritten int
	if err = contenttype.ValidateDefinition(); err != nil {
		return err
	}
	switch fileExt {
	case ".json":
		bs, err := json.Marshal(contenttype)
		if err != nil {
			return err
		}
		fileBytes = bs
	case ".yaml", ".yml":
		bs, err := yaml.Marshal(contenttype)
		if err != nil {
			return err
		}
		fileBytes = bs
	default:
		return fmt.Errorf("unsupported content type format: %s", fileExt)
	}
	fmt.Fprintf(w, "writing %d bytes to %s\n", len(fileBytes), website.RelativePath(contenttype.URI.Path))
	if err = os.MkdirAll(filepath.Dir(contenttype.URI.Path), 0755); err != nil {
		return err
	}
	if file, err = os.OpenFile(contenttype.URI.Path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644); err != nil {
		return err
	}
	defer file.Close()
	if fileBytesWritten, err = file.Write(fileBytes); err != nil {
		return err
	}
	if fileBytesWritten != len(fileBytes) {
		return fmt.Errorf("expected to write %v bytes, but wrote %v", len(fileBytes), fileBytesWritten)
	}
	return nil
}

func (contenttype *ContentType) Load() (err error) {
	var fileBytes []byte
	var fileExt string = filepath.Ext(contenttype.URI.Path)
	if contenttype.URI == nil {
		return fmt.Errorf("contenttype URI is required")
	}
	if fileBytes, err = os.ReadFile(contenttype.URI.Path); err != nil {
		return err
	}
	var legacy struct {
		Spec map[string]any `yaml:"spec"`
	}
	if err = yaml.Unmarshal(fileBytes, &legacy); err != nil {
		return err
	}
	if _, exists := legacy.Spec["unlisted"]; exists {
		return fmt.Errorf("content-type spec.unlisted is no longer supported; use spec.visibility")
	}
	if _, exists := legacy.Spec["uri_resolver"]; exists {
		return fmt.Errorf("content-type spec.uri_resolver is no longer supported; use spec.path_resolver")
	}
	switch fileExt {
	case ".json":
		if err = json.Unmarshal(fileBytes, &contenttype); err != nil {
			return err
		}
	case ".yaml", ".yml":
		if err = yaml.Unmarshal(fileBytes, &contenttype); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported file type: %s", fileExt)
	}
	if err = contenttype.ValidateDefinition(); err != nil {
		return err
	}
	return nil
}

func (contenttype ContentType) ValidateDefinition() error {
	return contenttype.Spec.Validate()
}

func (state ContentTypeState) Validate(prompts htprompt.Prompts) error {
	return prompts.ValidateUserData(state.UserData)
}

func (state *ContentTypeState) SeedDefaults(prompts htprompt.Prompts) {
	if state == nil {
		return
	}
	state.UserData = prompts.Defaults()
}

// ValidatePage validates page against this content type without writing it.
func (contenttype *ContentType) ValidatePage(website *Website, page *Page) error {
	return validatePage(website, contenttype, page)
}

// FinalizePage validates and publishes page through website context.
func (contenttype *ContentType) FinalizePage(website *Website, page *Page, createdAt string, w io.Writer) error {
	return finalizePage(website, contenttype, page, createdAt, w)
}

func (spec ContentTypeSpecification) MIMEType() string {
	switch strings.ToLower(spec.Format) {
	case "", "markdown", "md":
		return "text/markdown; charset=utf-8"
	case "json":
		return "application/json"
	case "yaml", "yml":
		return "application/yaml"
	default:
		return "application/octet-stream"
	}
}

func (spec ContentTypeSpecification) Validate() error {
	if spec.Visibility == "" {
		return fmt.Errorf("content-type spec.visibility is required")
	}
	if err := spec.Visibility.Validate(); err != nil {
		return fmt.Errorf("content-type spec.visibility invalid: %w", err)
	}
	if spec.Format == "" {
		return fmt.Errorf("content-type spec.format is required")
	}
	if _, err := pageDataFilename(spec.Format); err != nil {
		return err
	}
	if spec.PathResolver == "" {
		return fmt.Errorf("content-type spec.path_resolver is required")
	}
	if _, err := PublicURLPathResolverFor(spec.PathResolver); err != nil {
		return err
	}
	renderedPath, err := renderContentTypePathTemplate(spec.Path, time.Date(2006, 1, 2, 0, 0, 0, 0, time.UTC))
	if err != nil {
		return fmt.Errorf("content-type spec.path invalid: %w", err)
	}
	if _, err := cleanRelativePath(renderedPath); err != nil {
		return fmt.Errorf("content-type spec.path invalid: %w", err)
	}
	if err := spec.Prompts.Validate(); err != nil {
		return err
	}
	if err := spec.Patches.Validate(spec.TemplateData); err != nil {
		return err
	}
	prompts, err := spec.Prompts.Map()
	if err != nil {
		return err
	}
	if err := spec.Patches.ValidateReferences(prompts); err != nil {
		return err
	}
	return nil
}

type PublicURLPathResolver interface {
	ResolvePublicURLPath(page Page, contenttype *ContentType) (string, error)
}

type PublicURLPathResolverKind string

const (
	ManualPathResolver     PublicURLPathResolverKind = "manual"
	TitleKabobPathResolver PublicURLPathResolverKind = "title-kabob"
	TitleSnakePathResolver PublicURLPathResolverKind = "title_snake"
	TimestampPathResolver  PublicURLPathResolverKind = "timestamp"
	UUIDPathResolver       PublicURLPathResolverKind = "uuid"
	NoPathResolver         PublicURLPathResolverKind = "none"
)

func PublicURLPathResolverFor(name PublicURLPathResolverKind) (PublicURLPathResolver, error) {
	switch name {
	case ManualPathResolver:
		return manualPathResolver{}, nil
	case TitleKabobPathResolver:
		return titleKabobPathResolver{}, nil
	case TitleSnakePathResolver:
		return titleSnakePathResolver{}, nil
	case TimestampPathResolver:
		return timestampPathResolver{}, nil
	case UUIDPathResolver:
		return uuidPathResolver{}, nil
	case NoPathResolver:
		return noPathResolver{}, nil
	default:
		return nil, fmt.Errorf("content-type spec.path_resolver %q is not supported", name)
	}
}

type manualPathResolver struct{}

func (manualPathResolver) ResolvePublicURLPath(page Page, contenttype *ContentType) (string, error) {
	if suffix, ok, err := explicitPageURISuffix(page); err != nil || ok {
		if err != nil {
			return "", err
		}
		return contentTypePublicURLPath(page, contenttype, suffix)
	}
	if page.Visibility.IsDraft() {
		return contentTypePublicURLPath(page, contenttype, stableDraftURISuffix(page))
	}
	return "", fmt.Errorf("content-type requires page.slug property")
}

type titleKabobPathResolver struct{}

func (titleKabobPathResolver) ResolvePublicURLPath(page Page, contenttype *ContentType) (string, error) {
	if suffix, ok, err := explicitPageURISuffix(page); err != nil || ok {
		if err != nil {
			return "", err
		}
		return contentTypePublicURLPath(page, contenttype, suffix)
	}
	if page.Title != "" {
		return contentTypePublicURLPath(page, contenttype, kabobURISuffix(page.Title))
	}
	if page.Visibility.IsDraft() {
		return contentTypePublicURLPath(page, contenttype, stableDraftURISuffix(page))
	}
	return "", fmt.Errorf("content-type requires page.title property")
}

type titleSnakePathResolver struct{}

func (titleSnakePathResolver) ResolvePublicURLPath(page Page, contenttype *ContentType) (string, error) {
	if suffix, ok, err := explicitPageURISuffix(page); err != nil || ok {
		if err != nil {
			return "", err
		}
		return contentTypePublicURLPath(page, contenttype, suffix)
	}
	if page.Title != "" {
		return contentTypePublicURLPath(page, contenttype, snakeURISuffix(page.Title))
	}
	if page.Visibility.IsDraft() {
		return contentTypePublicURLPath(page, contenttype, stableDraftURISuffix(page))
	}
	return "", fmt.Errorf("content-type requires page.title property")
}

type timestampPathResolver struct{}

func (timestampPathResolver) ResolvePublicURLPath(page Page, contenttype *ContentType) (string, error) {
	if suffix, ok, err := explicitPageURISuffix(page); err != nil || ok {
		if err != nil {
			return "", err
		}
		return contentTypePublicURLPath(page, contenttype, suffix)
	}
	if page.Visibility.IsDraft() {
		return contentTypePublicURLPath(page, contenttype, stableDraftURISuffix(page))
	}
	return contentTypePublicURLPath(page, contenttype, fmt.Sprintf("%v", page.LastModified().UnixMilli()))
}

type uuidPathResolver struct{}

func (uuidPathResolver) ResolvePublicURLPath(page Page, contenttype *ContentType) (string, error) {
	if suffix, ok, err := explicitPageURISuffix(page); err != nil || ok {
		if err != nil {
			return "", err
		}
		return contentTypePublicURLPath(page, contenttype, suffix)
	}
	if page.Visibility.IsDraft() {
		return contentTypePublicURLPath(page, contenttype, stableDraftURISuffix(page))
	}
	return contentTypePublicURLPath(page, contenttype, strings.ToLower(uuid.New().String()))
}

type noPathResolver struct{}

func (noPathResolver) ResolvePublicURLPath(page Page, contenttype *ContentType) (string, error) {
	if suffix, ok, err := explicitPageURISuffix(page); err != nil || ok {
		if err != nil {
			return "", err
		}
		return contentTypePublicURLPath(page, contenttype, suffix)
	}
	return contentTypePublicURLPath(page, contenttype, "")
}

// =============================================================================
// Functions
// =============================================================================

func explicitPageURISuffix(page Page) (string, bool, error) {
	if page.Slug == "" {
		return "", false, nil
	}
	if strings.Contains(page.Slug, "/") {
		return "", false, fmt.Errorf("page.slug cannot contain a slash (%s)", page.Slug)
	}
	return page.Slug, true, nil
}

func stableDraftURISuffix(page Page) string {
	if page.Source.Path != "" {
		dir := filepath.Base(page.Source.Path)
		if strings.HasPrefix(dir, "draft-") || draftTimestampURISuffixPattern.MatchString(dir) {
			return dir
		}
	}
	return timestampDraftURISuffix(time.Now())
}

func timestampDraftURISuffix(t time.Time) string {
	return fmt.Sprintf("d%011x", t.UnixMilli())
}

func contentTypePublicURLPath(page Page, contenttype *ContentType, suffix string) (string, error) {
	if contenttype == nil {
		return "", fmt.Errorf("page source resolution requires content type")
	}
	renderedPath, err := renderContentTypePath(contenttype.Spec.Path, &page)
	if err != nil {
		return "", fmt.Errorf("content-type spec.path invalid: %w", err)
	}
	prefix, err := cleanRelativePath(renderedPath)
	if err != nil {
		return "", fmt.Errorf("content-type spec.path invalid: %w", err)
	}
	cleanSuffix, err := cleanRelativePath(suffix)
	if err != nil {
		return "", fmt.Errorf("page uri suffix invalid: %w", err)
	}
	parts := []string{}
	if prefix != "" {
		parts = append(parts, prefix)
	}
	if cleanSuffix != "" {
		parts = append(parts, cleanSuffix)
	}
	sourcePath := filepath.Join(parts...)
	return (PageSource{Path: sourcePath}).PublicURLPath(page.UglyURL), nil
}

func renderContentTypePath(path string, page *Page) (string, error) {
	if !strings.Contains(path, "${") {
		return path, nil
	}
	date, err := contentTypePathDate(page)
	if err != nil {
		return "", err
	}
	return renderContentTypePathTemplate(path, date)
}

func renderContentTypePathTemplate(path string, date time.Time) (string, error) {
	return renderTemplate(path, template.FuncMap{
		"year":  func() string { return date.Format("2006") },
		"month": func() string { return date.Format("01") },
		"day":   func() string { return date.Format("02") },
	})
}

func contentTypePathDate(page *Page) (time.Time, error) {
	if page == nil {
		return time.Time{}, fmt.Errorf("content-type path template requires page")
	}
	timestamp := page.CreatedAt
	field := "created_at"
	if timestamp == "" {
		timestamp = page.UpdatedAt
		field = "updated_at"
	}
	if timestamp == "" {
		return time.Time{}, fmt.Errorf("content-type path template requires page.created_at or page.updated_at")
	}
	date, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return time.Time{}, fmt.Errorf("content-type path template requires valid page.%s: %w", field, err)
	}
	return date, nil
}
func pageDataFilename(format string) (string, error) {
	switch strings.ToLower(format) {
	case "json":
		return "index.json", nil
	case "yaml", "yml":
		return "index.yaml", nil
	case "markdown", "md":
		return "index.md", nil
	default:
		return "", fmt.Errorf("unsupported content type format: %s", format)
	}
}

func normalizeURISuffix(s string) string {
	s = strings.ToLower(s)
	s = strings.TrimSpace(s)
	s = regexp.MustCompile(`[^a-zA-Z0-9\s]`).ReplaceAllString(s, " ")
	return regexp.MustCompile(`\s+`).ReplaceAllString(s, " ")
}

func kabobURISuffix(s string) string {
	return strings.ReplaceAll(normalizeURISuffix(s), " ", "-")
}

func snakeURISuffix(s string) string {
	return strings.ReplaceAll(normalizeURISuffix(s), " ", "_")
}
