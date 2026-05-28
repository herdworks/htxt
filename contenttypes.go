package hypertexting

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

	hypertemplates "github.com/herdworks/hypertemplates-go"
	href "github.com/herdworks/hypertexting-go/href"
)

var (
	DefaultContentTypeMetadata      = ContentTypeMetadata{Name: "default", Description: "Default content type."}
	DefaultContentTypeSpecification = ContentTypeSpecification{Layout: "default", Path: "-", URIResolver: TimestampURIResolver, Format: "markdown", TemplateData: hypertemplates.TemplateData{}}
	DefaultContentType              = ContentType{URI: &href.Href{URL: url.URL{Path: "/dev/null"}}, Metadata: DefaultContentTypeMetadata, Spec: DefaultContentTypeSpecification}

	CustomContentTypeMetadata      = ContentTypeMetadata{Name: "custom", Description: "Custom content type."}
	CustomContentTypeSpecification = ContentTypeSpecification{Layout: "default", URIResolver: TimestampURIResolver, Format: "markdown", TemplateData: hypertemplates.TemplateData{}}
	CustomContentType              = ContentType{URI: &href.Href{URL: url.URL{Path: "/dev/null"}}, Metadata: CustomContentTypeMetadata, Spec: CustomContentTypeSpecification}

	draftTimestampURISuffixPattern = regexp.MustCompile(`^d[0-9a-f]{11}$`)
)

type ContentType struct {
	URI      *href.Href               `json:"uri" yaml:"-"`
	Metadata ContentTypeMetadata      `json:"metadata" yaml:"metadata"`
	Spec     ContentTypeSpecification `json:"spec" yaml:"spec"`
	Prompts  ContentTypePrompts       `json:"prompts" yaml:"prompts"`
	Patches  ContentTypePatches       `json:"patches" yaml:"patches"`
}
type ContentTypes []*ContentType

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

func (contenttype ContentType) Validate(page *Page) error {
	// skip validation for draft pages
	if page.Draft {
		return nil
	}
	return contenttype.Ready(page)
}

func (contenttype ContentType) Ready(page *Page) error {
	if err := contenttype.Spec.Validate(); err != nil {
		return err
	}
	if page == nil {
		return fmt.Errorf("content-type requires page")
	}
	var err error

	// title validation
	err = fmt.Errorf("content-type requires page.title property")
	if contenttype.Spec.Requires.Title && len(page.Title) == 0 {
		return err
	}
	if contenttype.Spec.URIResolver == TitleKabobURIResolver && len(page.Title) == 0 {
		return err
	}
	if contenttype.Spec.URIResolver == TitleSnakeURIResolver && len(page.Title) == 0 {
		return err
	}
	err = fmt.Errorf("content-type requires page.slug property")
	if contenttype.Spec.URIResolver == ManualURIResolver && len(page.Slug) == 0 {
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

	// success!
	return nil
}
func (contenttype ContentType) Valid(page *Page, w io.Writer) bool {
	if w == nil {
		w = os.Stdout
	}

	err := contenttype.Validate(page)
	if err != nil {
		fmt.Fprintln(w, err)
	}
	return err == nil
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
	if err = contenttype.Spec.Validate(); err != nil {
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
	if err = contenttype.Spec.Validate(); err != nil {
		return err
	}
	return nil
}

func (contenttype *ContentType) IsEmpty() bool {
	if contenttype == nil {
		return true
	}
	if contenttype.Metadata.Name == "" {
		return true
	}
	if contenttype.Spec.Layout == "" {
		return true
	}
	if contenttype.Spec.Format == "" {
		return true
	}
	return false
}

func (contenttype *ContentType) Apply(website *Website, page *Page) (err error) {
	op := *page // copy original page
	page.TemplateData = contenttype.Spec.TemplateData.MergeAppend(page.TemplateData)
	if err = page.Parse(page.TemplateData); err != nil {
		return err
	}
	page.LayoutRef = contenttype.Spec.Layout
	if len(op.LayoutRef) > 0 {
		page.LayoutRef = op.LayoutRef
	}
	if len(op.Title) > 0 {
		page.Title = op.Title
	}
	if len(op.Description) > 0 {
		page.Description = op.Description
	}
	if len(op.Content) > 0 {
		page.Content = op.Content
	}
	return nil
}
func ResolvePageURI(website *Website, page *Page, contenttype *ContentType, sourceURI *url.URL) (*url.URL, error) {
	if website == nil {
		return nil, fmt.Errorf("page.uri resolution requires website")
	}
	if page == nil {
		return nil, fmt.Errorf("page.uri resolution requires page")
	}
	contenttype = pageContentType(contenttype)
	resolver, err := PageURIResolverFor(contenttype.Spec.URIResolver)
	if err != nil {
		return nil, err
	}
	return resolver.ResolvePageURI(website, page, contenttype, sourceURI)
}

type ContentTypeMetadata struct {
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	Icon        string `json:"icon,omitempty" yaml:"icon,omitempty"`
	Action      string `json:"action,omitempty" yaml:"action,omitempty"`
}

type ContentTypeSpecification struct {
	Layout       string                      `json:"layout" yaml:"layout"`
	Path         string                      `json:"path" yaml:"path"`
	URIResolver  PageURIResolverName         `json:"uri_resolver" yaml:"uri_resolver"`
	Format       string                      `json:"format" yaml:"format"`
	Requires     ContentTypeRequirements     `json:"requires,omitempty" yaml:"requires,omitempty"`
	Unlisted     bool                        `json:"unlisted" yaml:"unlisted"`
	TemplateData hypertemplates.TemplateData `json:"template" yaml:"template"`
	Prompts      ContentTypePrompts          `json:"prompts,omitempty" yaml:"prompts,omitempty"`
	Patches      ContentTypePatches          `json:"patches,omitempty" yaml:"patches,omitempty"`
}

func (spec ContentTypeSpecification) Validate() error {
	if spec.Format == "" {
		return fmt.Errorf("content-type spec.format is required")
	}
	if _, err := contentTypeIndexFilename(spec.Format); err != nil {
		return err
	}
	if spec.URIResolver == "" {
		return fmt.Errorf("content-type spec.uri_resolver is required")
	}
	if _, err := PageURIResolverFor(spec.URIResolver); err != nil {
		return err
	}
	renderedPath, err := renderContentTypePathTemplate(spec.Path, time.Date(2006, 1, 2, 0, 0, 0, 0, time.UTC))
	if err != nil {
		return fmt.Errorf("content-type spec.path invalid: %w", err)
	}
	if _, err := cleanRelativeURIPath(renderedPath); err != nil {
		return fmt.Errorf("content-type spec.path invalid: %w", err)
	}
	return nil
}

type PageURIResolver interface {
	ResolvePageURI(website *Website, page *Page, contenttype *ContentType, sourceURI *url.URL) (*url.URL, error)
}

type PageURIResolverName string

const (
	ManualURIResolver     PageURIResolverName = "manual"
	TitleKabobURIResolver PageURIResolverName = "title-kabob"
	TitleSnakeURIResolver PageURIResolverName = "title_snake"
	TimestampURIResolver  PageURIResolverName = "timestamp"
	UUIDURIResolver       PageURIResolverName = "uuid"
	NoURIResolver         PageURIResolverName = "none"
)

func PageURIResolverFor(name PageURIResolverName) (PageURIResolver, error) {
	switch name {
	case ManualURIResolver:
		return manualURIResolver{}, nil
	case TitleKabobURIResolver:
		return titleKabobURIResolver{}, nil
	case TitleSnakeURIResolver:
		return titleSnakeURIResolver{}, nil
	case TimestampURIResolver:
		return timestampURIResolver{}, nil
	case UUIDURIResolver:
		return uuidURIResolver{}, nil
	case NoURIResolver:
		return noURIResolver{}, nil
	default:
		return nil, fmt.Errorf("content-type spec.uri_resolver %q is not supported", name)
	}
}

type manualURIResolver struct{}

func (manualURIResolver) ResolvePageURI(website *Website, page *Page, contenttype *ContentType, sourceURI *url.URL) (*url.URL, error) {
	if suffix, ok, err := explicitPageURISuffix(page); err != nil || ok {
		if err != nil {
			return nil, err
		}
		return contentTypePageURI(website, page, contenttype, suffix)
	}
	if page.Draft {
		return contentTypePageURI(website, page, contenttype, stableDraftURISuffix(page, sourceURI))
	}
	return nil, fmt.Errorf("content-type requires page.slug property")
}

type titleKabobURIResolver struct{}

func (titleKabobURIResolver) ResolvePageURI(website *Website, page *Page, contenttype *ContentType, sourceURI *url.URL) (*url.URL, error) {
	if suffix, ok, err := explicitPageURISuffix(page); err != nil || ok {
		if err != nil {
			return nil, err
		}
		return contentTypePageURI(website, page, contenttype, suffix)
	}
	if page.Title != "" {
		return contentTypePageURI(website, page, contenttype, kabobURISuffix(page.Title))
	}
	if page.Draft {
		return contentTypePageURI(website, page, contenttype, stableDraftURISuffix(page, sourceURI))
	}
	return nil, fmt.Errorf("content-type requires page.title property")
}

type titleSnakeURIResolver struct{}

func (titleSnakeURIResolver) ResolvePageURI(website *Website, page *Page, contenttype *ContentType, sourceURI *url.URL) (*url.URL, error) {
	if suffix, ok, err := explicitPageURISuffix(page); err != nil || ok {
		if err != nil {
			return nil, err
		}
		return contentTypePageURI(website, page, contenttype, suffix)
	}
	if page.Title != "" {
		return contentTypePageURI(website, page, contenttype, snakeURISuffix(page.Title))
	}
	if page.Draft {
		return contentTypePageURI(website, page, contenttype, stableDraftURISuffix(page, sourceURI))
	}
	return nil, fmt.Errorf("content-type requires page.title property")
}

type timestampURIResolver struct{}

func (timestampURIResolver) ResolvePageURI(website *Website, page *Page, contenttype *ContentType, sourceURI *url.URL) (*url.URL, error) {
	if suffix, ok, err := explicitPageURISuffix(page); err != nil || ok {
		if err != nil {
			return nil, err
		}
		return contentTypePageURI(website, page, contenttype, suffix)
	}
	if page.Draft {
		return contentTypePageURI(website, page, contenttype, stableDraftURISuffix(page, sourceURI))
	}
	return contentTypePageURI(website, page, contenttype, fmt.Sprintf("%v", page.LastModified().UnixMilli()))
}

type uuidURIResolver struct{}

func (uuidURIResolver) ResolvePageURI(website *Website, page *Page, contenttype *ContentType, sourceURI *url.URL) (*url.URL, error) {
	if suffix, ok, err := explicitPageURISuffix(page); err != nil || ok {
		if err != nil {
			return nil, err
		}
		return contentTypePageURI(website, page, contenttype, suffix)
	}
	if page.Draft {
		return contentTypePageURI(website, page, contenttype, stableDraftURISuffix(page, sourceURI))
	}
	return contentTypePageURI(website, page, contenttype, strings.ToLower(uuid.New().String()))
}

type noURIResolver struct{}

func (noURIResolver) ResolvePageURI(website *Website, page *Page, contenttype *ContentType, sourceURI *url.URL) (*url.URL, error) {
	if suffix, ok, err := explicitPageURISuffix(page); err != nil || ok {
		if err != nil {
			return nil, err
		}
		return contentTypePageURI(website, page, contenttype, suffix)
	}
	return contentTypePageURI(website, page, contenttype, "")
}

func explicitPageURISuffix(page *Page) (string, bool, error) {
	if page == nil || page.Slug == "" {
		return "", false, nil
	}
	if strings.Contains(page.Slug, "/") {
		return "", false, fmt.Errorf("page.slug cannot contain a slash (%s)", page.Slug)
	}
	return page.Slug, true, nil
}

func stableDraftURISuffix(page *Page, sourceURI *url.URL) string {
	uri := sourceURI
	if uri == nil && page != nil {
		uri = page.URI
	}
	if uri != nil && uri.Path != "" {
		dir := filepath.Base(filepath.Dir(uri.Path))
		if isStableDraftURISuffix(dir) {
			return dir
		}
	}
	return timestampDraftURISuffix(time.Now())
}

func isStableDraftURISuffix(suffix string) bool {
	return strings.HasPrefix(suffix, "draft-") || draftTimestampURISuffixPattern.MatchString(suffix)
}

func timestampDraftURISuffix(t time.Time) string {
	return fmt.Sprintf("d%011x", t.UnixMilli())
}

func contentTypePageURI(website *Website, page *Page, contenttype *ContentType, suffix string) (*url.URL, error) {
	if website == nil {
		return nil, fmt.Errorf("page.uri resolution requires website")
	}
	if page == nil {
		return nil, fmt.Errorf("page.uri resolution requires page")
	}
	if contenttype == nil {
		return nil, fmt.Errorf("page.uri resolution requires content type")
	}
	renderedPath, err := renderContentTypePath(contenttype.Spec.Path, page)
	if err != nil {
		return nil, fmt.Errorf("content-type spec.path invalid: %w", err)
	}
	prefix, err := cleanRelativeURIPath(renderedPath)
	if err != nil {
		return nil, fmt.Errorf("content-type spec.path invalid: %w", err)
	}
	cleanSuffix, err := cleanRelativeURIPath(suffix)
	if err != nil {
		return nil, fmt.Errorf("page uri suffix invalid: %w", err)
	}
	filename, err := contentTypeIndexFilename(contenttype.Spec.Format)
	if err != nil {
		return nil, err
	}
	parts := []string{website.ContentDirPath()}
	if prefix != "" {
		parts = append(parts, prefix)
	}
	if cleanSuffix != "" {
		parts = append(parts, cleanSuffix)
	}
	parts = append(parts, filename)
	return &url.URL{Path: filepath.Join(parts...)}, nil
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
	return renderTemplate(path, contentTypePathFuncMap(date))
}

func contentTypePathFuncMap(date time.Time) template.FuncMap {
	return template.FuncMap{
		"year":  func() string { return date.Format("2006") },
		"month": func() string { return date.Format("01") },
		"day":   func() string { return date.Format("02") },
	}
}

func contentTypePathDate(page *Page) (time.Time, error) {
	if page == nil {
		return time.Time{}, fmt.Errorf("content-type path template requires page")
	}
	if page.CreatedAt == "" {
		return time.Time{}, fmt.Errorf("content-type path template requires page.created_at")
	}
	date, err := time.Parse(time.RFC3339, page.CreatedAt)
	if err != nil {
		return time.Time{}, fmt.Errorf("content-type path template requires valid page.created_at: %w", err)
	}
	return date, nil
}
func contentTypeIndexFilename(format string) (string, error) {
	switch format {
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

type ContentTypeRequirements struct {
	Title      bool           `json:"title,omitempty" yaml:"title,omitempty"`
	Content    bool           `json:"content,omitempty" yaml:"content,omitempty"`
	Redirect   bool           `json:"redirect,omitempty" yaml:"redirect,omitempty"`
	Attachment AttachmentKind `json:"attachment,omitempty" yaml:"attachment,omitempty"`
}

func (requirements ContentTypeRequirements) Validate(page *Page) (err error) {
	// TODO: should we hang requirement validators off of the ContentTypeRequirements type?
	return nil
}

// ============================================================================
// Content Type Prompts and Patches
// ============================================================================
type ContentTypePrompt map[string]any
type ContentTypePrompts []ContentTypePrompt

func (prompts *ContentTypePrompts) JSON() (string, error) {
	bytes, err := json.Marshal(prompts)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}
func (prompt *ContentTypePrompt) JSON() (string, error) {
	bytes, err := json.Marshal(prompt)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

type ContentTypePatch map[string]any
type ContentTypePatches []ContentTypePatch

func (patch *ContentTypePatch) JSON() (string, error) {
	bytes, err := json.Marshal(patch)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}
func (patches *ContentTypePatches) JSON() (string, error) {
	bytes, err := json.Marshal(patches)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}
