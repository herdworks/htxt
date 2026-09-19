# HyperTexting CMS

HyperTexting is the content management system for HyperTemplates.

At a high level, HyperTemplates templating engine attempts to be unopinionated about the structure of HTML content and layouts. 
The core specification is primarily concerned with HTML templates (layouts) and template data (content) as inputs, which are used to generate individual web pages (HTML documents) as outputs. 
When it comes to building web sites (collections of HTML documents), a more opinionated approach to content management is required.

## Concepts

Where HyperTemplates is concerned with the lower-level building blocks of templates and data, the HyperTexting CMS implements the higher-level building blocks requird to manage websites.

* Websites
* Pages
* Assets
* Attachments
* Tags
* Feeds 2.0: declarative `site.feeds` definitions, page-local `page.feeds`
  views, Atom output, and automatic tag feeds.
* Status pages: feed-only `page.visibility: status` content (including
  `feed`, `story`, `chat`, `message`, `response`, and `reply` aliases) renders
  a redirect landing page configured by `site.config.status_layout` and
  `site.config.status_redirect`.
* Namespaces
* Content Types
* Builds
* Providers

## hsync initialization

Sites opt into hsync with the canonical public URL of its mutable index. hsync
always stores the local manifest at `.hsync/manifest.json`:

```yaml
sync:
  remote: https://example.com/.hsync/latest.json
```

HyperTexting declares this configuration; `hsync.Deployment` owns adoption,
reconciliation, provider coordination, and finalization.

## Website contact vCard

An optional top-level `contact` block in `site.yaml` (or `site.json`) publishes
`/.well-known/hypertexting/contact.vcf` whenever `name` contains non-whitespace
text. A name alone is sufficient. Other contact details are optional and use
exactly four collections: `email`, `phone`, `social`, and `links`.

```yaml
contact:
  name: Caleb Hailey
  favicon: static/favicon.jpeg
  email:
    - label: default
      address: me@calebhailey.com
      preferred: true
    - label: comments
      address: hello@calebhailey.com
  phone:
    - label: iPhone
      address: "+15032094070"
      types: [cell, voice]
  social:
    - service: Instagram
      username: calebhailey
      href: https://instagram.com/calebhailey
    - service: Signal
      username: calebhailey.01
  links:
    - label: iMessage
      href: sms://me@calebhailey.com
    - label: FaceTime
      href: facetime://me@calebhailey.com
```

Email and phone entries accept `label`, `address`, optional `types` (such as
`home`, `work`, `cell`, or `voice`), and `preferred`. Labels do not imply types
or preference; use `preferred: true` rather than a `pref` type. Quote telephone
numbers to preserve their leading plus sign. Numbers may contain digits,
spaces, plus signs, parentheses, periods, and hyphens.

Social profiles accept `service`, optional `label`, `username`, `href`, and
`user_identifier`. Provide a username or an absolute URI, or both. These map
to Apple's `X-SOCIALPROFILE` properties; username-only entries use an opaque
`x-apple:` URI rather than an inferred public URL. Custom services are allowed.
Social parameter values cannot contain double quotes. Apple's macOS Contacts
parser drops service names containing spaces even in its own exports; use
service names without spaces for reliable Apple import. Generic `links` accept
`label`, `href`, optional `types`, and `preferred`, including custom messaging
and calling URI schemes. Types and preference share a comma-separated `TYPE`
parameter so clients preserve all values.

The output is vCard 3.0. `name` becomes `FN`; `N` contains empty structured-name
components. The website base URL supplies `NICKNAME` as `@hostname`, lowercased
and without a leading `www.`, port, path, query, or fragment. Custom labels use
Apple `X-ABLabel` properties. Text is UTF-8 with CRLF endings and folded lines.
Contact values cannot contain control characters. Contact metadata is validated
once during website loading, including HTTP(S) favicon URL syntax, without
filesystem or network I/O. VCF generation
trusts that metadata and loads the optional image separately. Direct callers
constructing or modifying `Website.Contact` must validate it before generation.
YAML and JSON preserve the configuration; importing existing VCF files is not
supported.

`favicon` accepts a source path relative to the website root or an HTTP/HTTPS
URL, such as `https://example.com/favicon.png`. Its JPEG or PNG content is
embedded as base64 `PHOTO` data. Both sources use HyperFeed's shared reader,
with its default 10-second timeout and a 3 MiB image byte limit. Images must be
no wider or taller than 4096 pixels; dimensions are checked before full decoding.
Oversized images are rejected, not resized. HTTP redirects may
only target HTTP/HTTPS URLs. Build cancellation cancels an in-flight fetch.
Missing, invalid, oversized, or unsupported images and HTTP errors fail the
build. Remote images are fetched on each build; there is no fallback to
`site.favicon`.

The configuration is available to templates as `site.contact`. No discovery
link is automatically added to the HTML head; users can advertise the file
through their own links or templates.

Full and incremental builds generate the file after ordinary assets, winning
same-path collisions. Removing the contact or clearing its name disables
generation by omitting the contact card from generated assets. This does not
delete or reconcile a previously generated card in a reused output directory.
To clear stale output, delete the build and regenerate it, or use a new build ID.
