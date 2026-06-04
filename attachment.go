package hypertexting

import (
	"errors"

	hypertemplates "github.com/herdworks/hypertemplates-go"
	"github.com/herdworks/hypertexting-go/href"
)

const (
	// Attachment errors
	AttachmentErrorMissingKind = "attachment.kind is required"
	AttachmentErrorMissingHref = "attachment.href is required"
)

type Attachment struct {
	Kind AttachmentKind              `json:"kind,omitempty" yaml:"kind,omitempty"`
	Href *href.Href                  `json:"href,omitempty" yaml:"href,omitempty"`
	Data hypertemplates.TemplateData `json:"data,omitempty" yaml:"data,omitempty"`
}
type Attachments []*Attachment

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
