package hypertexting

import (
	"encoding/json"
	"errors"
	"fmt"

	hypergraph "github.com/herdworks/hypergraph-go"
	hypertemplates "github.com/herdworks/hypertemplates-go"
)

const (
	// Attachment errors
	AttachmentErrorMissingKind = "attachment.kind is required"
	AttachmentErrorMissingHref = "attachment.href is required"
)

type Attachment struct {
	Kind        AttachmentKind `json:"kind,omitempty" yaml:"kind,omitempty"`
	Href        string `json:"href,omitempty" yaml:"href,omitempty"`
	Data        hypertemplates.TemplateData `json:"data,omitempty" yaml:"data,omitempty"`

	document    *hypergraph.Document
}
type Attachments []*Attachment

func (attachments Attachments) Process(verbose bool) (err error) {
	if len(attachments) == 0 { return nil } // guard
	for _, attachment := range attachments {
		if err := attachment.Process(verbose); err != nil { return err }
	}
	return nil
}
func (attachment *Attachment) Process(verbose bool) (err error) {
	switch attachment.Kind {
		case AttachmentKindLink: return attachment.ProcessLink(verbose)
		default:
			fmt.Printf("attachment kind \"%s\" is not supported\n", attachment.Kind)
			return nil // noop
	}
}
func (attachment *Attachment) ProcessLink(verbose bool) (err error) {
	attachment.document = &hypergraph.Document{ Href: attachment.Href }
	if err = attachment.document.Download(verbose); err != nil { return err }
	if err = attachment.document.ParseHTML(); err != nil { return err }
	attachment.Data["hypergraph"] = attachment.document.HyperGraphMetaData(verbose)
	attachment.Data["opengraph"] = attachment.document.OpenGraphMetaData(verbose)
	attachment.Data["oembed"] = attachment.document.OEmbedMetadata(verbose)
	attachment.Data["twitter"] = attachment.document.TwitterCardMetaData(verbose)
	return nil
}

func (attachments Attachments) TemplateData() (hypertemplates.TemplateDataSet) {
	data := []hypertemplates.TemplateData{}
	for _,attachment := range attachments {
		data = append(data, attachment.TemplateData())
	}
	return data
}
func (attachment *Attachment) TemplateData() (data hypertemplates.TemplateData) {
	return hypertemplates.TemplateData{
		"kind": attachment.Kind,
		"href": attachment.Href,
		"data": attachment.Data,
	}
}

func (attachment *Attachment) Validate() (err error) {
	if attachment.Kind == "" { return errors.New(AttachmentErrorMissingKind) }
	if attachment.Href == "" { return errors.New(AttachmentErrorMissingHref) }
	return nil
}

func (attachments Attachments) FilteredBy(kind AttachmentKind) (Attachments) {
	filtered := Attachments{}
	for _,attachment := range attachments {
		if attachment.Kind == kind { filtered = append(filtered, attachment)}
	}
	return filtered
}

func (attachment *Attachment) UnmarshalJSON(data []byte) (err error) {
	var rawData map[string]any
	if err = json.Unmarshal(data, &rawData); err != nil { return err }
	if kind, ok := rawData["kind"].(string); ok { attachment.Kind = AttachmentKind(kind) }
	if href, ok := rawData["href"].(string); ok { attachment.Href = href }
	if d, ok := rawData["data"].(string); ok {
		if err = json.Unmarshal([]byte(d), &attachment.Data); err != nil { return err }
	} else {
		attachment.Data = hypertemplates.TemplateData{}
	}
	return nil
}

// ============================================================================
// Helper types, methods, and functions
// ============================================================================
type AttachmentKind string
const (
	AttachmentKindLink AttachmentKind = "link"
	AttachmentKindFile AttachmentKind = "file"
	AttachmentKindImage AttachmentKind = "image"
	AttachmentKindVideo AttachmentKind = "video"
	AttachmentKindAudio AttachmentKind = "audio"
)