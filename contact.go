package htxt

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"net/mail"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/herdworks/hyperfeed/reader"
)

// Contact is the website's public contact identity and communication details.
type Contact struct {
	Name    string                 `json:"name" yaml:"name" jsonschema:"description=A nonblank display name enables contact.vcf generation."`
	Favicon string                 `json:"favicon,omitempty" yaml:"favicon,omitempty" jsonschema:"description=Website-root-relative path or HTTP(S) URL to a JPEG or PNG embedded in the vCard."`
	Email   []ContactEmail         `json:"email,omitempty" yaml:"email,omitempty" jsonschema:"description=Email addresses."`
	Phone   []ContactPhone         `json:"phone,omitempty" yaml:"phone,omitempty" jsonschema:"description=Telephone numbers."`
	Social  []ContactSocialProfile `json:"social,omitempty" yaml:"social,omitempty" jsonschema:"description=Social profiles with independent service and username or URI."`
	Links   []ContactLink          `json:"links,omitempty" yaml:"links,omitempty" jsonschema:"description=Generic contact URIs including messaging and calling links."`
}

// ContactEmail is a labeled email address.
type ContactEmail struct {
	Label     string   `json:"label,omitempty" yaml:"label,omitempty" jsonschema:"description=Custom display label independent of types and preference."`
	Address   string   `json:"address" yaml:"address" jsonschema:"required,format=email"`
	Types     []string `json:"types,omitempty" yaml:"types,omitempty" jsonschema:"description=vCard classifications such as home or work."`
	Preferred bool     `json:"preferred,omitempty" yaml:"preferred,omitempty" jsonschema:"description=Whether this address is preferred."`
}

// ContactPhone is a labeled telephone number.
type ContactPhone struct {
	Label     string   `json:"label,omitempty" yaml:"label,omitempty" jsonschema:"description=Custom display label independent of types and preference."`
	Address   string   `json:"address" yaml:"address" jsonschema:"required,description=Telephone number stored as a string to preserve its leading plus sign."`
	Types     []string `json:"types,omitempty" yaml:"types,omitempty" jsonschema:"description=vCard classifications such as cell or voice."`
	Preferred bool     `json:"preferred,omitempty" yaml:"preferred,omitempty" jsonschema:"description=Whether this number is preferred."`
}

// ContactSocialProfile preserves the distinct parts of an Apple social profile.
type ContactSocialProfile struct {
	Service        string `json:"service" yaml:"service" jsonschema:"required,description=Social service name including custom services."`
	Label          string `json:"label,omitempty" yaml:"label,omitempty" jsonschema:"description=Optional custom display label."`
	Username       string `json:"username,omitempty" yaml:"username,omitempty" jsonschema:"description=Service username; provide a username or href."`
	Href           string `json:"href,omitempty" yaml:"href,omitempty" jsonschema:"format=uri,description=Explicit profile URI; no public URL is inferred from a username."`
	UserIdentifier string `json:"user_identifier,omitempty" yaml:"user_identifier,omitempty" jsonschema:"description=Optional service-specific user identifier."`
}

// ContactLink is a generic labeled contact URI.
type ContactLink struct {
	Label     string   `json:"label,omitempty" yaml:"label,omitempty" jsonschema:"description=Custom display label."`
	Href      string   `json:"href" yaml:"href" jsonschema:"required,format=uri,description=Contact URI including custom messaging or calling schemes."`
	Types     []string `json:"types,omitempty" yaml:"types,omitempty" jsonschema:"description=vCard link classifications."`
	Preferred bool     `json:"preferred,omitempty" yaml:"preferred,omitempty" jsonschema:"description=Whether this link is preferred."`
}

const (
	contactAssetPath         = ".well-known/contact.vcf"
	contactPhotoMaxDimension = 4096
)

func (Contact) JSONSchemaName() string    { return "contact" }
func (Contact) JSONSchemaTitle() string   { return "Website contact" }
func (Contact) JSONSchemaSummary() string { return "Public website contact information." }
func (Contact) JSONSchemaDescription() string {
	return "A named website contact published as a vCard with email, phone, social, and links collections."
}

// Validate checks enabled contact metadata without filesystem or network I/O.
// Website loading calls it once; VCF generation trusts the validated metadata.
// An absent or blank name disables generation.
func (contact *Contact) Validate() error {
	if !contact.enabled() {
		return nil
	}
	for _, value := range []string{contact.Name, contact.Favicon} {
		if err := validateContactText(value); err != nil {
			return fmt.Errorf("site.contact name or favicon: %w", err)
		}
	}
	if isContactPhotoURL(contact.Favicon) {
		if err := validateContactURI(contact.Favicon); err != nil {
			return fmt.Errorf("site.contact.favicon: %w", err)
		}
	}
	for i, entry := range contact.Email {
		field := fmt.Sprintf("site.contact.email[%d]", i)
		if err := validateContactAddress(entry.Label, entry.Address, entry.Types); err != nil {
			return fmt.Errorf("%s: %w", field, err)
		}
		address, err := mail.ParseAddress(entry.Address)
		if err != nil || address.Address != entry.Address || address.Name != "" {
			return fmt.Errorf("%s.address must be an email address without a display name", field)
		}
	}
	for i, entry := range contact.Phone {
		field := fmt.Sprintf("site.contact.phone[%d]", i)
		if err := validateContactAddress(entry.Label, entry.Address, entry.Types); err != nil {
			return fmt.Errorf("%s: %w", field, err)
		}
		digits := false
		for _, r := range entry.Address {
			if r >= '0' && r <= '9' {
				digits = true
				continue
			}
			if !strings.ContainsRune("+ -().", r) {
				return fmt.Errorf("%s.address contains an invalid telephone character", field)
			}
		}
		if !digits {
			return fmt.Errorf("%s.address must contain a telephone number", field)
		}
	}
	for i, entry := range contact.Social {
		field := fmt.Sprintf("site.contact.social[%d]", i)
		for _, value := range []string{entry.Service, entry.Label, entry.Username, entry.Href, entry.UserIdentifier} {
			if err := validateContactText(value); err != nil {
				return fmt.Errorf("%s: %w", field, err)
			}
		}
		for _, value := range []string{entry.Service, entry.Username, entry.UserIdentifier} {
			if strings.ContainsRune(value, '"') {
				return fmt.Errorf("%s parameters must not contain double quotes", field)
			}
		}
		if strings.TrimSpace(entry.Service) == "" || (strings.TrimSpace(entry.Username) == "" && entry.Href == "") {
			return fmt.Errorf("%s requires a service and username or href", field)
		}
		if entry.Href != "" {
			if err := validateContactURI(entry.Href); err != nil {
				return fmt.Errorf("%s.href: %w", field, err)
			}
		}
	}
	for i, entry := range contact.Links {
		if err := validateContactAddress(entry.Label, entry.Href, entry.Types); err != nil {
			return fmt.Errorf("site.contact.links[%d]: %w", i, err)
		}
		if err := validateContactURI(entry.Href); err != nil {
			return fmt.Errorf("site.contact.links[%d].href: %w", i, err)
		}
	}
	return nil
}

func (contact *Contact) enabled() bool {
	return contact != nil && strings.TrimSpace(contact.Name) != ""
}

// vcard encodes validated contact data without performing filesystem access.
func (contact *Contact) vcard(photo []byte, photoType, nickname string) (data []byte, err error) {
	if !contact.enabled() {
		return nil, nil
	}
	var output strings.Builder
	write := func(line string) { writeContactLine(&output, line) }
	write("BEGIN:VCARD")
	write("VERSION:3.0")
	write("FN:" + contactText(strings.TrimSpace(contact.Name)))
	write("N:;;;;")
	if nickname != "" {
		if err := validateContactText(nickname); err != nil {
			return nil, fmt.Errorf("contact nickname: %w", err)
		}
		write("NICKNAME:" + contactText(nickname))
	}
	group := 0
	property := func(name, value, label string) {
		prefix := ""
		if label != "" {
			group++
			prefix = fmt.Sprintf("item%d.", group)
		}
		write(prefix + name + ":" + value)
		if label != "" {
			write(prefix + "X-ABLabel:" + contactText(label))
		}
	}
	for _, entry := range contact.Email {
		property("EMAIL"+contactTypes(entry.Types, entry.Preferred), contactText(entry.Address), entry.Label)
	}
	for _, entry := range contact.Phone {
		property("TEL"+contactTypes(entry.Types, entry.Preferred), contactText(entry.Address), entry.Label)
	}
	for _, entry := range contact.Social {
		name := "X-SOCIALPROFILE;TYPE=" + contactParameter(entry.Service)
		if entry.Username != "" {
			name += ";X-USER=" + contactParameter(entry.Username)
		}
		if entry.UserIdentifier != "" {
			name += ";X-USERID=" + contactParameter(entry.UserIdentifier)
		}
		uri := entry.Href
		if uri == "" {
			uri = "x-apple:" + url.PathEscape(entry.Username)
		}
		property(name, uri, entry.Label)
	}
	for _, entry := range contact.Links {
		property("URL"+contactTypes(entry.Types, entry.Preferred), entry.Href, entry.Label)
	}
	if len(photo) != 0 {
		write("PHOTO;ENCODING=b;TYPE=" + photoType + ":" + base64.StdEncoding.EncodeToString(photo))
	}
	write("END:VCARD")
	return []byte(output.String()), nil
}

func (ContactEmail) JSONSchemaName() string    { return "contact-email" }
func (ContactEmail) JSONSchemaTitle() string   { return "Contact email" }
func (ContactEmail) JSONSchemaSummary() string { return "A labeled email address." }
func (ContactEmail) JSONSchemaDescription() string {
	return "An email address with independent label, classifications, and preference."
}
func (ContactPhone) JSONSchemaName() string    { return "contact-phone" }
func (ContactPhone) JSONSchemaTitle() string   { return "Contact phone" }
func (ContactPhone) JSONSchemaSummary() string { return "A labeled telephone number." }
func (ContactPhone) JSONSchemaDescription() string {
	return "A telephone number with independent label, classifications, and preference."
}
func (ContactSocialProfile) JSONSchemaName() string    { return "contact-social-profile" }
func (ContactSocialProfile) JSONSchemaTitle() string   { return "Contact social profile" }
func (ContactSocialProfile) JSONSchemaSummary() string { return "An Apple-compatible social profile." }
func (ContactSocialProfile) JSONSchemaDescription() string {
	return "A service with a username or explicit URI and optional user identifier and label."
}
func (ContactLink) JSONSchemaName() string    { return "contact-link" }
func (ContactLink) JSONSchemaTitle() string   { return "Contact link" }
func (ContactLink) JSONSchemaSummary() string { return "A generic contact URI." }
func (ContactLink) JSONSchemaDescription() string {
	return "A labeled contact URI including messaging and calling schemes."
}

func (website *Website) contactVCard(ctx context.Context) ([]byte, error) {
	if website == nil || !website.Contact.enabled() {
		return nil, nil
	}
	var photo []byte
	var photoType string
	if website.Contact.Favicon != "" {
		var err error
		if photo, photoType, err = website.contactPhoto(ctx); err != nil {
			return nil, fmt.Errorf("site.contact.favicon: %w", err)
		}
	}
	var nickname string
	if baseURL, err := url.Parse(website.BaseURL); err == nil && baseURL.Hostname() != "" {
		nickname = "@" + strings.TrimPrefix(strings.ToLower(baseURL.Hostname()), "www.")
	}
	return website.Contact.vcard(photo, photoType, nickname)
}

// contactPhoto loads the original image bytes and validates their format.
// Local paths are resolved against the website root before reader normalization.
func (website *Website) contactPhoto(ctx context.Context) (photo []byte, photoType string, err error) {
	source := website.Contact.Favicon
	if !isContactPhotoURL(source) {
		if source, err = website.ResourcePath(website.RootDir(), source); err != nil {
			return nil, "", err
		}
	}
	uri, err := reader.Normalize(source)
	if err != nil {
		return nil, "", err
	}
	config := reader.DefaultConfig
	config.MaxContentLength = 3 << 20 // 3 MiB leaves room for base64 and vCard overhead.
	client := reader.Client(config)
	defer client.CloseIdleConnections()
	if photo, err = reader.Read(ctx, client, uri, config); err != nil {
		return nil, "", err
	}
	dimensions, format, err := image.DecodeConfig(bytes.NewReader(photo))
	if err != nil {
		return nil, "", fmt.Errorf("must be a valid JPEG or PNG: %w", err)
	}
	if dimensions.Width > contactPhotoMaxDimension || dimensions.Height > contactPhotoMaxDimension {
		return nil, "", fmt.Errorf("image dimensions %dx%d exceed the %dx%d pixel limit", dimensions.Width, dimensions.Height, contactPhotoMaxDimension, contactPhotoMaxDimension)
	}
	switch format {
	case "jpeg":
		photoType = "JPEG"
	case "png":
		photoType = "PNG"
	default:
		return nil, "", fmt.Errorf("must be JPEG or PNG, got %s", format)
	}
	// Header inspection bounds pixel allocation; full decoding still checks the
	// image body before embedding the original bytes unchanged.
	if _, _, err = image.Decode(bytes.NewReader(photo)); err != nil {
		return nil, "", fmt.Errorf("must be a valid JPEG or PNG: %w", err)
	}
	return photo, photoType, nil
}

func isContactPhotoURL(source string) bool {
	source = strings.ToLower(source)
	return strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://")
}

func validateContactText(value string) error {
	if !utf8.ValidString(value) {
		return fmt.Errorf("must be valid UTF-8")
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return fmt.Errorf("must not contain control characters")
		}
	}
	return nil
}

func validateContactAddress(label, address string, types []string) error {
	if strings.TrimSpace(address) == "" {
		return fmt.Errorf("address must not be blank")
	}
	for _, value := range []string{label, address} {
		if err := validateContactText(value); err != nil {
			return err
		}
	}
	for _, kind := range types {
		if kind == "" || strings.EqualFold(kind, "pref") {
			return fmt.Errorf("types must contain classification tokens; use preferred for preference")
		}
		for _, r := range kind {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-') {
				return fmt.Errorf("invalid type %q", kind)
			}
		}
	}
	return nil
}

func validateContactURI(value string) error {
	if err := validateContactText(value); err != nil {
		return err
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || strings.IndexFunc(value, unicode.IsSpace) >= 0 {
		return fmt.Errorf("must be an absolute URI without whitespace")
	}
	if (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Hostname() == "" {
		return fmt.Errorf("HTTP URLs must include a host")
	}
	return nil
}

func contactText(value string) string {
	return strings.NewReplacer("\\", "\\\\", ";", "\\;", ",", "\\,").Replace(value)
}

func contactParameter(value string) string {
	// vCard 3.0 parameters use quoting, not text-property backslash escaping.
	if strings.ContainsAny(value, ":;, ") {
		return "\"" + value + "\""
	}
	return value
}

func contactTypes(types []string, preferred bool) string {
	values := make([]string, 0, len(types)+1)
	for _, kind := range types {
		values = append(values, strings.ToUpper(kind))
	}
	if preferred {
		values = append(values, "PREF")
	}
	if len(values) == 0 {
		return ""
	}
	// A single TYPE parameter also preserves every value in clients that parse
	// parameters into a dictionary, including HyperTextingKit's contact parser.
	return ";TYPE=" + strings.Join(values, ",")
}

func writeContactLine(output *strings.Builder, line string) {
	limit := 75
	for len(line) > limit {
		end := limit
		for end > 0 && !utf8.RuneStart(line[end]) {
			end--
		}
		output.WriteString(line[:end])
		output.WriteString("\r\n ")
		line = line[end:]
		limit = 74
	}
	output.WriteString(line)
	output.WriteString("\r\n")
}
