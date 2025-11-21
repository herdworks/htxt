package hypertexting

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"

	hypertemplates "github.com/herdworks/hypertemplates-go"
)

var (
	DefaultContentTypeMetadata = ContentTypeMetadata{ Name: "default", Description: "Default content type." }
	DefaultContentTypeSpecification = ContentTypeSpecification{ Layout: "default", Format: "markdown", TemplateData: hypertemplates.TemplateData{} }
	DefaultContentType = ContentType{ URI: &url.URL{ Path: "/dev/null" }, Metadata: DefaultContentTypeMetadata, Spec: DefaultContentTypeSpecification }

	CustomContentTypeMetadata = ContentTypeMetadata{ Name: "custom", Description: "Custom content type." }
	CustomContentTypeSpecification = ContentTypeSpecification{ Layout: "default", Format: "markdown", TemplateData: hypertemplates.TemplateData{} }
	CustomContentType = ContentType{ URI: &url.URL{ Path: "/dev/null" }, Metadata: CustomContentTypeMetadata, Spec: CustomContentTypeSpecification }
)

type ContentType struct {
	URI *url.URL `json:"-" yaml:"-"`
	Metadata ContentTypeMetadata `json:"metadata" yaml:"metadata"`
	Spec ContentTypeSpecification `json:"spec" yaml:"spec"`
	Prompts ContentTypePrompts `json:"prompts" yaml:"prompts"`
	Patches ContentTypePatches `json:"patches" yaml:"patches"`
}
type ContentTypes []*ContentType

func (types ContentTypes) ColumnValues(field string) []string {
	values := []string{}
	for _,t := range types {
		switch field {
			case "Name", "name": values = append(values, t.Metadata.Name)
			case "Description", "description": values = append(values, t.Metadata.Description)
		}
	}
	return values
}

func (contenttype ContentType) Validate(page *Page) (error) {
	var err error

	// skip validation for draft pages
	if page.Draft { return nil }

	// title validation
	err = fmt.Errorf("content-type requires page.title property")
	if contenttype.Spec.Requires.Title && len(page.Title) == 0 { return err }
	if contenttype.Spec.AutoSlug == TitleKabobSlug && len(page.Title) == 0 { return err }
	if contenttype.Spec.AutoSlug == TitleSnakeSlug && len(page.Title) == 0 { return err }

	// content validation
	err = fmt.Errorf("content-type requires page.content property")
	if contenttype.Spec.Requires.Content && len(page.Content) == 0 { return err }

	// redirect validation
	err = fmt.Errorf("content-type requires page.redirect property")
	if contenttype.Spec.Requires.Redirect && len(page.Redirect) == 0 { return err }

	// attachments validation
	err = fmt.Errorf("content-type requires page.attachments (expected 1 or more, got %v)", len(page.Attachments))
	if len(contenttype.Spec.Requires.Attachment) > 0 && len(page.Attachments) == 0 { return err }
	if contenttype.Spec.Requires.Attachment == AttachmentKindLink && len(page.Attachments.FilteredBy(AttachmentKindLink)) == 0 { return fmt.Errorf("content-type requires link attachment") }
	if contenttype.Spec.Requires.Attachment == AttachmentKindFile && len(page.Attachments.FilteredBy(AttachmentKindFile)) == 0 { return fmt.Errorf("content-type requires file attachment") }
	if contenttype.Spec.Requires.Attachment == AttachmentKindImage && len(page.Attachments.FilteredBy(AttachmentKindImage)) == 0 { return fmt.Errorf("content-type requires image attachment") }
	if contenttype.Spec.Requires.Attachment == AttachmentKindVideo && len(page.Attachments.FilteredBy(AttachmentKindVideo)) == 0 { return fmt.Errorf("content-type requires video attachment") }
	if contenttype.Spec.Requires.Attachment == AttachmentKindAudio && len(page.Attachments.FilteredBy(AttachmentKindAudio)) == 0 { return fmt.Errorf("content-type requires audio attachment") }

	// success!
	return nil
}
func (contenttype ContentType) Valid(page *Page) (bool) {
	err := contenttype.Validate(page)
	if err != nil { fmt.Println(err) }
	return err == nil
}

func (contenttype *ContentType) Create(website *Website, format string) (err error) {
	var fileDirectory string = website.Theme.ContentTypesDir()
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
	contenttype.URI = &url.URL{ Path: filepath.Join(fileDirectory, fileName + fileExtension) }
	return contenttype.Save(website)
}

func (contenttype *ContentType) Save(website *Website) (err error) {
	var file *os.File
	var fileExt string = filepath.Ext(contenttype.URI.Path)
	var fileBytes []byte
	var fileBytesWritten int
	switch fileExt {
		case ".json":
			bs, err := json.Marshal(contenttype)
			if err != nil { return err }
			fileBytes = bs
		case ".yaml", ".yml":
			bs, err := yaml.Marshal(contenttype)
			if err != nil { return err }
			fileBytes = bs
		default:
			return fmt.Errorf("unsupported content type format: %s", fileExt)
	}
	fmt.Printf("writing %d bytes to %s\n", len(fileBytes), website.RelativePath(contenttype.URI.Path))
	if err = os.MkdirAll(filepath.Dir(contenttype.URI.Path), 0755); err != nil { return err }
	if file,err = os.OpenFile(contenttype.URI.Path, os.O_CREATE|os.O_WRONLY, 0644); err != nil { return err }; defer file.Close()
	if fileBytesWritten,err = file.Write(fileBytes); err != nil { return err }
	if fileBytesWritten != len(fileBytes) { return fmt.Errorf("expected to write %v bytes, but wrote %v", len(fileBytes), fileBytesWritten) }
	return nil
}

func (contenttype *ContentType) Load() (err error) {
	var fileBytes []byte
	var fileExt string = filepath.Ext(contenttype.URI.Path)
	if contenttype.URI == nil { return fmt.Errorf("contenttype URI is required") }
	if fileBytes, err = os.ReadFile(contenttype.URI.Path); err != nil { return err }
	switch fileExt {
		case ".json":
			if err = json.Unmarshal(fileBytes, &contenttype); err != nil { return err }
		case ".yaml", ".yml":
			if err = yaml.Unmarshal(fileBytes, &contenttype); err != nil { return err }
		default:
			return fmt.Errorf("unsupported file type: %s", fileExt)
	}
	return nil
}

func (contenttype *ContentType) IsEmpty() bool {
	if contenttype == nil { return true }
	if contenttype.Metadata.Name == "" { return true }
	if contenttype.Spec.Layout == "" { return true }
	if contenttype.Spec.Format == "" { return true }
	return false
}

func (contenttype *ContentType) Apply(website *Website, page *Page) (err error) {
	op := *page // copy original page
	if page.URI == nil {
		// only generate Slug and set URI if not already set
		page.Slug = contenttype.Spec.AutoSlug.Format(page)
		page.URI = contenttype.PageURI(website, page)
		page.Path = PagePath(website, page.URI)
	} else {
		fmt.Println("page.URI is already set, skipping AutoSlug, URI, and Path generation")
	}
	if err = page.Parse(contenttype.Spec.TemplateData); err != nil { return err }
	page.TemplateData = page.TemplateData.Merge(contenttype.Spec.TemplateData, true, false)
	page.LayoutRef = contenttype.Spec.Layout
	if len(op.LayoutRef) > 0 { page.LayoutRef = op.LayoutRef }
	if len(op.Title) > 0 { page.Title = op.Title }
	if len(op.Description) > 0 { page.Description = op.Description }
	if len(op.Content) > 0 { page.Content = op.Content }
	return nil
}
func (contenttype *ContentType) PageURI(website *Website, page *Page) *url.URL {
	pageRoot := website.ContentDir()
	if page.Draft { pageRoot = website.DraftsDir() }
	return &url.URL{ Path: filepath.Join(pageRoot, contenttype.Path(), page.Slug, contenttype.IndexFilename()) }
}
func (contenttype *ContentType) Path() string {
	return contenttype.Spec.Path
}
func (contenttype ContentType) IndexFilename() string {
	switch contenttype.Spec.Format {
		case "json": return "index.json"
		case "yaml", "yml": return "index.yaml"
		case "markdown", "md": return "index.md"
		default:
			return fmt.Sprintf("index.%s", contenttype.Spec.Format)
	}
}

type ContentTypeMetadata struct {
	Name string `json:"name" yaml:"name"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	Icon string `json:"icon,omitempty" yaml:"icon,omitempty"`
	Action string `json:"action,omitempty" yaml:"action,omitempty"`
}

type ContentTypeSpecification struct {
	Layout string `json:"layout" yaml:"layout"`
	Path string `json:"path" yaml:"path"`
	AutoSlug ContentTypeSlugFormat `json:"auto_slug" yaml:"auto_slug"`
	Format string `json:"format" yaml:"format"`
	Requires ContentTypeRequirements `json:"requires,omitempty" yaml:"requires,omitempty"`
	Unlisted bool `json:"unlisted" yaml:"unlisted"`
	TemplateData hypertemplates.TemplateData `json:"template" yaml:"template"`
	Prompts ContentTypePrompts `json:"prompts,omitempty" yaml:"prompts,omitempty"`
	Patches ContentTypePatches `json:"patches,omitempty" yaml:"patches,omitempty"`
}

type ContentTypeSlugFormat string
const (
	TimestampSlug ContentTypeSlugFormat = "timestamp" // default
	TitleKabobSlug ContentTypeSlugFormat = "title-kabob"
	TitleSnakeSlug ContentTypeSlugFormat = "title_camel"
	ManualSlug ContentTypeSlugFormat = "manual"
	UUIDSlug ContentTypeSlugFormat = "uuid"
	NoSlug ContentTypeSlugFormat = "none"
)
func (format ContentTypeSlugFormat) Format(page *Page) (string) {
	if len(page.Slug) > 0 { return page.Slug } // skip if page already has a slug
	switch format {
		case NoSlug: return "" // no-op
		case ManualSlug: return strings.ReplaceAll(format.NormalizeSlug(page.Slug), " ", "")
		case UUIDSlug: return strings.ToLower(uuid.New().String())
		case TitleKabobSlug: return format.KabobCase(page.Title)
		case TitleSnakeSlug: return format.SnakeCase(page.Title)
		case TimestampSlug: return fmt.Sprintf("%v", page.LastModified().UnixMilli())
		default:
			fmt.Printf("slug format \"%s\" unknown, falling back to \"timestamp\" format\n", format)
			return fmt.Sprintf("%v", page.LastModified().UnixMilli())
	}

}
func (format ContentTypeSlugFormat) NormalizeSlug(s string) (slug string) {
	slug = strings.ToLower(s) // convert to lowercase
	slug = strings.TrimSpace(slug) // trim leading and trailing whitespace
	slug = regexp.MustCompile(`[^a-zA-Z0-9\s]`).ReplaceAllString(slug, " ") // replace non-alphanumeric characters with spaces
	slug = regexp.MustCompile(`\s+`).ReplaceAllString(slug, " ") // replace consecurive whitespace characters with a single space
	return slug
}
func (format ContentTypeSlugFormat) KabobCase(s string) (slug string) {
	return strings.ReplaceAll(format.NormalizeSlug(s), " ", "-")
}
func (format ContentTypeSlugFormat) SnakeCase(s string) string {
	return strings.ReplaceAll(format.NormalizeSlug(s), " ", "_")
}

type ContentTypeRequirements struct {
	Title bool `json:"title,omitempty" yaml:"title,omitempty"`
	Content bool `json:"content,omitempty" yaml:"content,omitempty"`
	Redirect bool `json:"redirect,omitempty" yaml:"redirect,omitempty"`
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
	if err != nil { return "", err }
	return string(bytes), nil
}
func (prompt *ContentTypePrompt) JSON() (string, error) {
	bytes,err := json.Marshal(prompt)
	if err != nil { return "", err }
	return string(bytes), nil
}

type ContentTypePatch map[string]any
type ContentTypePatches []ContentTypePatch
func (patch *ContentTypePatch) JSON() (string, error) {
	bytes,err := json.Marshal(patch)
	if err != nil { return "", err }
	return string(bytes), nil
}
func (patches *ContentTypePatches) JSON() (string, error) {
	bytes,err := json.Marshal(patches)
	if err != nil { return "", err }
	return string(bytes), nil
}
