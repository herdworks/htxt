package htxt

import (
	"errors"
	"mime"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/herdworks/httt"
	"github.com/herdworks/htxt/href" // TODO: Use github.com/herdworks/hypermedia/href when it is available.
	"github.com/herdworks/hyperfeed"
	hyperhref "github.com/herdworks/hyperfeed/href" // TODO: Use github.com/herdworks/hypermedia/href when it is available.
)

type Attachment struct {
	Kind AttachmentKind    `json:"kind,omitempty" yaml:"kind,omitempty" jsonschema:"required"`
	Href *href.Href        `json:"href,omitempty" yaml:"href,omitempty" jsonschema:"required,type=string,format=uri-reference,description=A URI reference. It may be absolute or relative."`
	Data httt.TemplateData `json:"data,omitempty" yaml:"data,omitempty"`
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

func (attachments Attachments) TemplateData() httt.TemplateDataSet {
	data := []httt.TemplateData{}
	for _, attachment := range attachments {
		data = append(data, attachment.TemplateData())
	}
	return data
}
func (attachment *Attachment) TemplateData() (data httt.TemplateData) {
	href := ""
	if attachment.Href != nil {
		href = attachment.Href.String()
	}
	return httt.TemplateData{
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

// toHyperFeedLink converts link attachments into related feed metadata links.
func (attachment *Attachment) toHyperFeedLink(base *url.URL) (*hyperfeed.Link, error) {
	if !attachment.IsLink() || attachment.Href == nil || attachment.Href.String() == "" {
		return nil, nil
	}
	attachmentURL := base.ResolveReference(&attachment.Href.URL)
	attachmentHref, err := hyperhref.Parse(attachmentURL)
	if err != nil {
		return nil, err
	}
	return &hyperfeed.Link{Href: attachmentHref, Rel: "related", Mimetype: "text/html", Title: attachment.Title(), Origin: "metadata"}, nil
}

// toHyperFeedAttachment converts file-like attachments into feed enclosures.
func (attachment *Attachment) toHyperFeedAttachment(source PageSource, base *url.URL) (*hyperfeed.Attachment, error) {
	if !attachment.IsEnclosure() || attachment.Href == nil || attachment.Href.String() == "" {
		return nil, nil
	}
	attachmentURL := base.ResolveReference(&attachment.Href.URL)
	attachmentHref, err := hyperhref.Parse(attachmentURL)
	if err != nil {
		return nil, err
	}
	return &hyperfeed.Attachment{Href: attachmentHref, Mimetype: attachment.AtomMIMEType(attachmentURL), Title: attachment.Title(), Enclosure: true, Size: attachment.Size(source)}, nil
}

func (attachment *Attachment) Size(source PageSource) int64 {
	uri := source.URI()
	if uri == nil || attachment == nil || attachment.Href == nil {
		return 0
	}
	ref := &attachment.Href.URL
	if ref.IsAbs() || ref.Path == "" || strings.HasPrefix(ref.Path, "/") {
		return 0
	}
	pageDir := filepath.Dir(uri.Path)
	filePath := filepath.Clean(filepath.Join(pageDir, filepath.FromSlash(ref.Path)))
	relPath, err := filepath.Rel(pageDir, filePath)
	if err != nil || relPath == ".." || strings.HasPrefix(relPath, ".."+string(os.PathSeparator)) || filepath.IsAbs(relPath) {
		return 0
	}
	info, err := os.Stat(filePath)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 {
		return 0
	}
	return info.Size()
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
