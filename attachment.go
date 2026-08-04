package hypertexting

import (
	"errors"
	"mime"
	"net/url"
	"path"

	hypertemplates "github.com/herdworks/hypertemplates-go"
	"github.com/herdworks/hypertexting-go/href"
)

type Attachment struct {
	Kind AttachmentKind              `json:"kind,omitempty" yaml:"kind,omitempty" jsonschema:"required"`
	Href *href.Href                  `json:"href,omitempty" yaml:"href,omitempty" jsonschema:"required,type=string,format=uri-reference,description=A URI reference. It may be absolute or relative."`
	Data hypertemplates.TemplateData `json:"data,omitempty" yaml:"data,omitempty"`
}
type Attachments []*Attachment

func (Attachment) JSONSchemaTitle() string {
	return "HyperTexting attachment"
}
func (Attachment) JSONSchemaName() string    { return "attachment" }
func (Attachment) JSONSchemaSummary() string { return "A linked or embedded page resource." }

func (Attachment) JSONSchemaDescription() string {
	return "A linked or embedded resource associated with a page."
}

const (
	// Attachment errors
	AttachmentErrorMissingKind = "attachment.kind is required"
	AttachmentErrorMissingHref = "attachment.href is required"
)

func (attachments Attachments) Clone() Attachments {
	if attachments == nil {
		return nil
	}
	clones := make(Attachments, len(attachments))
	for i, attachment := range attachments {
		clones[i] = attachment.Clone()
	}
	return clones
}

func (attachment *Attachment) Clone() *Attachment {
	if attachment == nil {
		return nil
	}
	clone := *attachment
	if attachment.Href != nil {
		hrefClone := *attachment.Href
		clone.Href = &hrefClone
	}
	if attachment.Data != nil {
		clone.Data = attachment.Data.Clone()
	}
	return &clone
}

func (attachments Attachments) TemplateData() hypertemplates.TemplateDataSet {
	data := []hypertemplates.TemplateData{}
	for _, attachment := range attachments {
		data = append(data, attachment.TemplateData())
	}
	return data
}
func (attachment *Attachment) TemplateData() (data hypertemplates.TemplateData) {
	href := ""
	if attachment.Href != nil {
		href = attachment.Href.String()
	}
	return hypertemplates.TemplateData{
		"kind": attachment.Kind,
		"href": href,
		"data": attachment.Data,
	}
}

func (attachment *Attachment) Title() string {
	if attachment == nil || attachment.Data == nil {
		return ""
	}
	for _, key := range []string{"hypergraph.title", "title", "opengraph.title", "twitter.title"} {
		if title, ok := attachment.Data.Get(key).(string); ok {
			return title
		}
	}
	return ""
}

func (attachment *Attachment) Validate() (err error) {
	if attachment.Kind == "" {
		return errors.New(AttachmentErrorMissingKind)
	}
	if attachment.Href == nil || attachment.Href.String() == "" {
		return errors.New(AttachmentErrorMissingHref)
	}
	return nil
}

func (attachments Attachments) FilteredBy(kind AttachmentKind) Attachments {
	filtered := Attachments{}
	for _, attachment := range attachments {
		if attachment.Kind == kind {
			filtered = append(filtered, attachment)
		}
	}
	return filtered
}

func (attachment *Attachment) AtomMIMEType(u *url.URL) string {
	if attachment != nil && attachment.Data != nil {
		for _, key := range []string{"mimetype", "mime_type", "type"} {
			if mimetype, ok := attachment.Data.Get(key).(string); ok && mimetype != "" {
				return mimetype
			}
		}
	}
	if u == nil || u.Path == "" {
		return ""
	}
	mimetype := mime.TypeByExtension(path.Ext(u.Path))
	if mimetype == "" {
		return ""
	}
	mediatype, _, err := mime.ParseMediaType(mimetype)
	if err != nil || mediatype == "" {
		return mimetype
	}
	return mediatype
}

// ============================================================================
// Helper types, methods, and functions
// ============================================================================
type AttachmentKind string

const (
	AttachmentKindLink     AttachmentKind = "link"
	AttachmentKindFile     AttachmentKind = "file"
	AttachmentKindImage    AttachmentKind = "image"
	AttachmentKindVideo    AttachmentKind = "video"
	AttachmentKindAudio    AttachmentKind = "audio"
	AttachmentKindDocument AttachmentKind = "document"
)

func (attachment *Attachment) IsLink() bool {
	return attachment != nil && attachment.Kind == AttachmentKindLink
}

func (attachment *Attachment) IsEnclosure() bool {
	if attachment == nil {
		return false
	}
	switch attachment.Kind {
	case AttachmentKindFile, AttachmentKindImage, AttachmentKindAudio, AttachmentKindVideo, AttachmentKindDocument:
		return true
	default:
		return false
	}
}
