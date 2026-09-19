package htxt

import (
	"github.com/herdworks/hsync"
	"github.com/herdworks/httt/hypermark"
	"github.com/herdworks/httt/markdown"
	"github.com/herdworks/httt/theme"
	"github.com/herdworks/htup"
	"github.com/herdworks/htxt/jsonschema"
)

// SchemaRegistry contains the stable CMS and SSG JSON Schema providers.
var SchemaRegistry = jsonschema.Registry{
	Attachment{},
	Byline{},
	Contact{},
	ContactEmail{},
	ContactPhone{},
	ContactSocialProfile{},
	ContactLink{},
	FeedConfig{},
	FeedData{},
	FeedSelector{},
	HTMLLink{},
	HTMLMeta{},
	markdown.HyperMarkConfig{},
	hypermark.MentionConfig{},
	Page{},
	htup.HostingProvider{},
	htup.FilterRules{},
	htup.Secret{},
	hsync.SyncConfig{},
	hsync.Rules{},
	theme.Theme{},
	Website{},
	WebsiteConfig{},
	WebsiteFavicon{},
}
