# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog], and this project adheres to [Semantic Versioning].

## Unreleased

* Added non-writing page data preview support with `Page.RenderData` and `ContentType.PreviewPageData`, allowing composer clients to inspect content type overlays and prompt patches without autosaving or publishing.

## v0.36.0 (2026-07-03)

* **BREAKING:** Reworked content type composer configuration around declarative `spec.prompts` (`prompt.Prompts`) and `spec.patches` (`patch.Patches`), replacing top-level content type prompt/patch fields and generic map aliases with typed prompt and patch packages. Individual patches are now `patch.Patch` values with `patch.Operation` operations, validated against non-protected `page.*` template data paths.
* **BREAKING:** Added explicit composer runtime state with non-serialized `ContentType.State.UserData`; prompt patch values are supplied from that state instead of persisted content type definitions.
* **BREAKING:** Changed content type application to an overlay contract: `spec.template` and prompt patches produce a non-mutating candidate overlay for composer previews, while only `FinalizePage` materializes that overlay into persisted page data.
* **BREAKING:** Changed page URI resolution from package function `ResolvePageURI(website, page, contenttype, sourceURI)` to `ContentType.ResolvePageURI(website, page, sourceURI)`.
* **BREAKING:** Removed unused content type helpers `ContentType.Valid`, `ContentTypeRequirements.Validate`, `CustomContentType`, `CustomContentTypeMetadata`, `CustomContentTypeSpecification`, `DefaultContentTypeMetadata`, and `DefaultContentTypeSpecification`.
* Added content type overlay APIs with `ContentType.TemplateData()`, `ContentType.OverlayPage(page)`, and `ContentType.CandidateReady(page)`, applying prompt patches from non-serialized `ContentType.State.UserData` to cloned `spec.template` data for non-mutating composer previews.
* Added `ContentTypeState`, `ContentTypeState.SeedDefaults`, `prompt.Prompts.Defaults`, `prompt.Prompts.ValidateUserData`, and `prompt.Prompts.RefSet` helpers for composer session state initialization and validation.
* Added `FinalizePage` as the publish/finalize path for all composer pages, with an optional content type argument that materializes content type overlays once while freeform pages finalize without overlays.
* Changed `CreatePage` and `UpdatePage` so content type templates and prompt patches are not materialized during autosave/update writes.
* Added the `patch` package for applying `add`, `replace`, and array `append` operations to `page.*` template data paths.
* Added clone helpers for pages, page caches, bylines, attachments, and feeds to support non-mutating overlay candidates.
* Changed drop-down prompt defaults, user data, and options to allow only JSON-like scalar values.
* Fixed `Page.ApplyComputedProperties(nil)` to skip redirect refresh metadata instead of dereferencing a nil website.
* Updated `github.com/herdworks/hyperfeed-go` to `v0.26.5`.

## v0.35.0 (2026-06-06)

* Updated github.com/herdworks/hyperfeed-go to v0.22.0
* Updated github.com/herdworks/hypertemplates-go to v0.31.0

## v0.34.1 (2026-06-04)

* Changed attachment JSON decoding to preserve object-shaped `data` metadata through direct `Attachment` unmarshalling instead of accepting legacy stringified JSON attachment data.

## v0.34.0 (2026-06-03)

* **BREAKING:** Changed `Attachment.Href` from `string` to `*href.Href`; Go callers constructing or inspecting attachments must use `href.Parse`, `*href.Href`, and `Href.String()`. JSON/YAML attachment `href` values still encode as strings, but invalid URL values now fail decoding instead of being stored as opaque strings.
* **BREAKING:** Removed attachment link-scraping helpers `Attachments.Process`, `Attachment.Process`, and `Attachment.ProcessLink`, and dropped the direct `hypergraph-go` dependency. Callers should scrape link metadata with `hypergraph-go` before populating `Attachment.Data`.
* **BREAKING:** Removed unused `Page.AtomEntryLink()` and changed `Page.AtomEntryLinks(website)` to emit the canonical page `alternate` link plus attachment `related`/`enclosure` links instead of redirect, `via`, and page-canonical `related` links.
* Added `AttachmentKindDocument`, document attachment content-type requirements, attachment title helpers, and Atom enclosure MIME type/length metadata for file, image, audio, video, and document attachments.
* Changed link-post Atom summaries to use `hypergraph.description` when present before falling back to the timestamp summary.
* Updated `github.com/herdworks/hyperfeed-go` to `v0.20.0`.

## v0.33.1 (2026-05-29)

* Added `page_count` and `feed_count` to `BuildResult`, and changed build reporting to generate report once, then log it and optionally write a build data file.
* Changed build start/completion logs to use `build.ID`, changed output copy/write log destinations to use `<build id>/<build-relative path>` instead of website-relative paths, and made `.deploymentignore` writes log the same build-relative `writing N bytes to ...` line as other build outputs.
* Changed post-output sidecars such as `.deploymentignore` and build data to log warnings instead of failing an otherwise successful build.

## v0.33.0 (2026-05-28)

Redesigned Build API around declarative configuration, improved build logging. 

* **BREAKING:** Changed `HyperBuild` to `Build`, moving build options into `Build.Config`. `Build.Execute(ctx)` now consumes the build's configured base URL, website overrides, artifact files, and verbose setting; `Build.LoadContext(ctx, w)` now uses the same build-owned config instead of per-call base URL, config override, and verbose arguments.
* Added `BuildConfig.BaseURL`, `BuildConfig.DataFile`, `BuildConfig.LogFile`, and `BuildConfig.Verbose`. Builds can now write flat `BuildResult` JSON data artifacts, mirror build logs to configured build-root-relative files, and automatically add in-build artifact files to `.deploymentignore`.
* Changed build status counters to live under `Build.Status`, with `Counter.Map()`, JSON helpers for safe counter snapshots, and `duration_ms` tracking through `BuildResult`.
* **BREAKING:** Changed website, page, content type, data, feed, asset, and attachment logging APIs to accept explicit `io.Writer` arguments, removing the package-global build logging state while preserving stdout and configured build log-file behavior through `Build.Execute(ctx)`.
* Updated `github.com/herdworks/hyperproviders-go` to `v0.23.0` and `github.com/herdworks/hypertemplates-go` to `v0.30.0` for explicit writer plumbing in downstream build and deployment paths.

## v0.32.2 (2026-05-27)

* Updated `github.com/herdworks/hyperproviders-go` to version 0.21.0 for new files secret provider and git ssh known_hosts support

## v0.32.1 (2026-05-27)

* Changed draft URIs from UUID to shorter timestamp based hex codes 

## v0.32.0 (2026-05-27)

* Added `Page.CanonicalURL(website)` and changed `page.canonical_url`, Atom entry IDs, and Atom `related` links to use canonical page URLs, preserving directory-style trailing slashes.
* Changed `Website.ResolveHref` to use URL reference resolution semantics instead of filesystem path joining, preserving queries/fragments and supporting protocol-relative hrefs.

## v0.31.2 (2026-05-25)

* Changed content type URI resolution from `spec.auto_slug` to `spec.uri_resolver`, with named internal URI resolvers that own full `page.URI` resolution.
* Added strict `spec.path` templating with `${`/`}` delimiters and only zero-argument `year`, `month`, and `day` functions based on `page.created_at`.
* Changed `DefaultContentType` to use the `-` content path, timestamp URI resolver, and Markdown format as the nil-content-type fallback for new page creation.
* Changed `ResolvePageURI` and page URI resolvers to accept `sourceURI` directly instead of carrying a `PageURIResolution` wrapper struct.
* Changed generated temporary draft URI segments from `draft-<uuid>` to timestamp-based `d<11-hex>` Unix millisecond identifiers while preserving existing `draft-*` draft paths.
* Changed content type validation to require `spec.format` and `spec.uri_resolver`, reject unsupported formats, and validate content type paths as safe relative URI paths.
* Changed `UpdatePage` so nil or empty content types perform an in-place auto update using the source file format instead of falling back to `DefaultContentType` URI resolution.
* Fixed content type application so `spec.template` data is treated as defaults and page template data overrides matching values.

## v0.31.1 (2026-05-21)

* Added `ContentType.Ready(page)` for draft-agnostic content type requirement checks, while keeping `ContentType.Validate(page)` as the draft-skipping publish compatibility wrapper.

## v0.31.0 (2026-05-20)

* Changed page/content-type persistence so content types resolve destination `page.URI` without mutating `page.Slug`; draft pages with incomplete final slug inputs now use stable temporary `draft-*` URI segments derived from source URI state, and `page.Path` is recomputed from the resolved URI after create/update.
* Changed page serialization to persist real user-provided `page.slug` values while leaving generated draft path segments out of page data.

## v0.30.0 (2026-05-20)

* Added `UpdatePage`, an existing-page persistence API that requires `page.URI`, recomputes the destination from the submitted page and content type, rehomes non-recursive regular page-directory files when the destination changes, preserves nested page directories, removes stale source page files after format changes, and rejects destination/file collisions.
* Changed `CreatePage` to require an empty `page.URI` and fail if the resolved destination page file already exists; callers should use `UpdatePage` to modify existing pages.

## v0.29.4 (2026-05-19)

* Fixed content type and page saves so overwriting an existing source file truncates stale trailing bytes before writing shorter JSON, YAML, or Markdown content.

## v0.29.3 (2026-05-13)

* Changed `HyperBuild.LoadContext` page-selection semantics: non-incremental builds now treat nil or empty `build.Pages` as a request for all discovered website pages, explicit page selections are limited to incremental builds, and incremental builds require at least one page.

## v0.29.2 (2026-05-11)

* Fixed infinite recursion when two feed-bearing pages both match each other. `HyperFeed.TemplateData()` now strips the `feed` key from each entry in `feed.pages`, breaking the `page→feed→pages→page` reference cycle that previously caused template traversal to recurse without a base case. Added `Pages.TemplateDataWithout(keys ...string)` as the helper.

## v0.29.1 (2026-05-11)

* Updated `github.com/herdworks/hypertemplates-go` to version 0.29.0

## v0.29.0 (2026-05-11)

* Added `AssetCandidates(theme, website, pages)` in `asset.go` returning the full un-collapsed list of theme, website, and page asset entries in iteration order. `AssetEntries` now delegates to it and applies the public-path collapse so the two helpers stay consistent. Enables callers (e.g. `hyperctl cms asset lookup`) to enumerate every contributor for a given asset.

## v0.28.0 (2026-05-08)

* Added `github.com/evanw/esbuild` v0.28.0 as an indirect dependency through hypertemplates-go.
* Updated `github.com/herdworks/hypertemplates-go` to v0.28.0, which standardizes scripting plugins on the ESM default-export contract. Computed-data files under the website data directory and `htvalue` plugins under the website/theme `plugins` directory must now declare `export default <function>` instead of `function namespace()` / `function plugin(input)`. The `Entrypoint`-based authoring contract is gone.
* Updated test fixtures (`assets_test.go`, `data_test.go`) to use `export default function(...)`.
* Removed `TestSeededFunctionInstallsNamespace` and `TestSeededFunctionRejectsCollidingEntry` from `data_test.go`: they exercised `scripting.Plugin{Source, Entrypoint, Bindings}.Run(ctx)`, which was removed in hypertemplates-go v0.28.0. The `data.LoadPlugin(...).Run(ctx)` path used by `generateComputedData` continues to be covered by `TestGenerateComputedDataDiscoversAndExecutes`.

## 0.27.0 (2026-05-06)

Added HyperTexting builtins (!) and updated `github.com/herdworks/hypertemplates-go` to version 0.27.0 with redesigned plugin interface.  

* Added an embedded `head_start.html` (redirect / generator / canonical / icon / links) shipped via a top-level `//go:embed embedded` directive. Auto-overrides `hypertemplates.BuiltinsFS` at package `init()` so any caller importing `hypertexting` gets the override transparently.
* Updated `github.com/herdworks/hypertemplates-go` to v0.27.0. The `plugins` package is gone; variable-substitution plugins now live in `value` and computed-namespace plugins in `data`.
* Changed `HyperBuild.Plugins` and `Website.Plugins` from `plugins.Plugins` to `htvalue.Plugins`. `Website.DiscoverPlugins` calls `htvalue.LoadPlugins` instead of `htplugins.LoadPlugins`.
* Changed `generateComputedData` to load and run computed-namespace files through `data.LoadPlugin(path, bindings).Run(ctx)` rather than `scripting.SeededFunction`. The hypertexting layer keeps the directory walk, segment routing, and namespace-collision checks; per-file JS loading and validation now live in the `data` package.

## v0.26.1 (2026-05-05)

Updated `github.com/herdworks/hypertemplates-go` to v0.26.1 w/ implicit ht-apply for ht-template elements.

## v0.26.0 (2026-05-05)

This release reforms how static template data is named and discovered, adds a `computed template data` capability, and exposes the build's data-loading and template-context APIs for reuse by inspection tools (e.g. `hyperctl cms data ls` and `hyperctl cms data inspect`).

BREAKING: removes `hypertexting.Namespace()` / `NamespaceData()` and the matching `NamespacedData` methods; data namespaces are now derived from file paths under the data directory rather than from data contents.

* Added `generateComputedData` build pass: `*.js` files alongside static data define `function namespace()` and install their return value at the keypath derived from the file's path. Scripts see `ht`, `build`, `env`, `site`, `data`, `theme` as ambient bindings (`data` is a read-only snapshot, `site.pages` is populated). The pass runs after `Pages.ResolveTemplateData` and before `WritePages`, in lexicographic filename order. Scripts cannot depend on each other's output — only on static data. The entry function is named `namespace` (not `data`) to avoid an unavoidable JS scoping conflict with the `data` binding.
* Added `(*HyperBuild).LoadContext(ctx, baseurl, configOverrides, verbose)` — populates the build's data context in place (website + environment, static + theme data, pages with feeds and tag pages, computed data) without writing assets, feeds, or pages. Inspection tools call this and then read `Context()`.
* Added `(*HyperBuild).Context()` — returns the unified `{ht, build, env, site, data, theme}` template context that page templates and computed scripts read.
* Changed `NamespacedTemplateData` and `ThemeTemplateData` to derive each namespace from the file's path under the data directory: `data/site.yaml` → `data.site.*`, `data/social/twitter.yaml` → `data.social.twitter.*` (subdirectories nest). Recursion is on by default; flat and nested layouts are both supported.
* Changed filename rules: each path segment must match `^[A-Za-z_][A-Za-z0-9_-]*$`. Spaces, intermediate dots (`foo.bar.yaml`), leading digits (`2024.yaml`), and leading hyphens (`-foo.yaml`) are errors.
* Changed collision handling: cross-format collisions (`foo.yaml` + `foo.json`) and directory-vs-file collisions (`foo.yaml` + `foo/`) are errors that cite both contributing paths. There are no "extend / merge" semantics — the user resolves the collision.
* Removed `hypertexting.Namespace(d)` and `hypertexting.NamespaceData(d)` package functions.
* Removed `NamespacedData.Namespace()` and `NamespacedData.NamespaceData()` methods. `NamespacedData.ColumnValues(field)` is retained, so `cli.ColumnListable` integrations are unaffected.

## v0.25.1 (2026-05-04)

* Add missing templatedata.go module

## v0.25.0 (2026-05-04)

* Migrated to the `hypertemplates-go` v0.24.0+ data API: `TemplateData.Namespace()` / `.NamespaceData()` calls switched to package-level `hypertexting.Namespace(d)` / `hypertexting.NamespaceData(d)` (`build.go`); `.Merge(other, true, false)` switched to `.MergeAppend(other)` (`contenttypes.go`); `.SetPathValue(path, v)` switched to `.Set(strings.Join(path, "."), v)` (`env.go`); `.Clean([]string{...})` switched to variadic `.Without(...)` and `hypertemplates.OrderedTemplateData` switched to a local wrapper (`page.go`).
* Compatible with `hypertemplates-go` v0.24.0 (no consumer code in this package was affected by the `value.Render` → `value.RenderString` rename or the rewriter hardening).

## v0.24.0 (2026-04-30)

* Added `page.ugly_url` / `Page.UglyURL` for generating explicit `.html` page URLs such as `/404.html` instead of directory-index URLs such as `/404/index.html`.
* Changed page build output paths to derive from `Page.Path`, keeping public page paths, canonical URLs, feeds, and generated HTML destinations aligned.

## v0.23.0 (2026-04-30)

* Added `Website.Providers` and changed `Website.Environments` entries to provider-name selections, defaulting missing environments to `default: [default]`.
* Changed deployment provider config to use provider map keys for names and root-level `include` / `exclude` filters.
* Updated `github.com/herdworks/hyperproviders-go` to version 0.19.0.

## v0.22.1 (2026-04-30)

* Changed page render template data so `ht.version`, `ht.release_date`, `build.id`, and `build.date` are assembled for each page instead of mutating website global data during build setup.
* Updated `github.com/herdworks/hyperproviders-go` to v0.18.2.
* Updated `github.com/herdworks/hypertemplates-go` to v0.23.0.

## v0.22.0 (2026-04-28)

Major redesign of HyperTexting resource management internals, including guards to prevent malicious themes from escaping the website root. This release also removes support for config.drafts_dir – drafts are now just pages with page.draft=true. 

* Added `Asset`, `Assets`, and asset-entry helpers for combining theme, website, and page assets while preserving override precedence by public path.
* Added a build option for draft preview modes while excluding draft pages from normal builds by default.
* Added validation for unsafe configured website directory and theme paths.
* Changed website, page, data, content type, and theme file discovery to use the root-scoped path helpers from `hypertemplates-go`.
* Changed website directory accessors to use side-effect-free `XDirName()` and `XDirPath()` getters instead of mutating config defaults while resolving paths.
* Changed plugin discovery to store compiled theme and website plugins separately, with website plugins taking precedence during builds.
* Removed `config.drafts_dir` and `site.drafts`; draft handling now uses only the page-level `draft` property.
* Replaced `HyperFile` asset tracking with slash-separated source paths and website-owned helpers for resolving asset source, public, and build output paths.

## v0.21.0 (2026-04-27)

* Added website plugin discovery from theme plugins and a new `config.plugins_dir` website directory, defaulting to `plugins`.
* Added build-time JavaScript plugin loading with `github.com/herdworks/hypertemplates-go/scripting`.
* Added plugin registration on rendered templates through `Template.Plugins`, enabling plugins in `ht-apply` value pipelines.
* Added plugin validation for duplicate plugin names and reserved plugin names (`httd`, `default`).
* Added `Website.Environments`, `Website.ResolveEnvironments()`, and `Website.DeploymentEnvironment()` for named deployment environments.
* Changed website deployment config from `provider` to `environments`, with empty environment names backfilled from their map keys.
* Updated `github.com/herdworks/hypertemplates-go` to v0.21.1.
* Updated `github.com/herdworks/hyperproviders-go` to v0.18.1.
* Updated `golang.org/x/net` to v0.47.0 and `golang.org/x/sync` to v0.20.0.
* Removed transitive AWS SDK dependencies no longer pulled in by `github.com/herdworks/hyperproviders-go`.
* Changed minimum Go toolchain to `1.25.0`.

## v0.20.0 (2026-04-21)

* Changed `Build.RenderPage()` to use `page.TemplateData` as `ht-block` user data 🤌🏽
  making `page.*` template data attributes accessible from `ht-block` elements
* Fixed `site.drafts` to load drafts from the configure drafts directory
* Updated `github.com/herdworks/hypertemplates-go` to v0.20.0

## v0.19.0 (2026-04-18)

Added `site.pages`, `site.drafts`, and `site.assets` template data.

* Added the internal `site.pages` slice to template data
* Added the internal `site.drafts` slice to template data
* Added the internal `site.assets` slice to template data (a `[]string` of asset paths)
* Updated `github.com/herdworks/hypertemplates-go` to v0.19.0 for the new `HyperFiles.Paths()` method

## v0.18.3 (2026-04-12)

* Fixed generated Atom feed `<link rel='self'>` hrefs which were concatenating "atom.xml" instead of joining "atom.xml" to the URL path.

## v0.18.2 (2026-03-30)

* Changed `Version()` to `LibVersion` package variable
* Changed `ReleaseDate() string` to `LibReleaseDate: time.Time` package variable 
* Fixed a bug in `Page.LastModified()` helper that caused a race condition in `hyperctl dev server`
* Updated `github.com/herdworks/hypergraph-go` to version 0.19.0
* Updated `github.com/herdworks/hyperfeed-go` to version 0.19.1
* Updated `github.com/herdworks/hypertemplates-go` to version 0.18.2

> **NOTE:** until now the `github.com/herdworks/hypertexting-go` package has attempted to stay in sync with the full suite of HyperTexting packages: 
> 
> * `hypergraph-go`
> * `hyperfeed-go`
> * `hypertemplates-go`
> * `hyperproviders-go`
> * `hypertexting-go`
> * `hyperctl`
> * `libhypertexting`
> * `HyperTextingKit`
> * `hypertexting-app`
> 
> Starting with this release we will decouple the library package versions from the user-facing app versions. 

## v0.18.1 (2026-03-25)

* Added `Byline.Username` field for backwards compatibility with early-stage builds of `hyperctl`
* Updated `hypergraph-go` and `hyperfeed-go` dependencies

## v0.18.0 (2026-02-26)

* Added an embedded bootstrap website scaffold (content, layouts, partials, static assets, and `theme.json`) for `Website.CreatePackage()`.
* Added support for theme template data files from the active theme (`theme/data/*.(yaml|json|opml)`).
* Added `Website.ListThemes()` and `config.themes_dir` support for discovering available themes.
* Added lightweight `Byline` and `Bylines` models for attribution and template serialization across website, page, and feed flows.
* Added support for `page.md`, `page.yaml`, and `page.json` page data files.
* Added support for static pages sourced from `index.html` or `page.html` files.
* Changed template data access for runtime and theme namespaces from `data.ht.*`, `data.env.*`, and `data.theme.*` to `ht.*`, `env.*`, and `theme.*`.
* Changed website and page attribution handling from hypergraph entity-backed `author` values to local `byline` models.
* Changed theme configuration handling to use a theme path (`config.theme`) plus a themes directory (`config.themes_dir`) for discovery.
* Changed theme integration types to match `hypertemplates-go` by using `hypertemplates.Theme` (instead of `hypertemplates.HyperTheme`).
* Changed page asset discovery to exclude page data files, site config files, and `theme.json` from copied page assets.
* Changed page discovery to exclude static pages in the configured builds directory.
* Changed attachment processing to accept an `io.Writer` for log/progress output and align with current `hypergraph-go` APIs.
* Changed tag page/feed generation to normalize tag URIs and sort generated feed pages.
* Updated first-party dependencies to `hyperfeed-go v0.18.0`, `hypergraph-go v0.18.0`, and `hypertemplates-go v0.18.0`.
* Fixed the dev server to honor `site.config.builds_dir` instead of always writing builds to `./builds/*`.

## v0.17.3

Initial release of the `hypertexting` package from the standalone `herdworks/hypertexting-go` repo.

* Completed initial migration from `herdworks/hypertexting-monorepo`
* Added versioned imports of standalone packages:
  * Added `github.com/herdworks/hypergraph-go v0.17.3` import to `go.mod`
  * Added `github.com/herdworks/hyperproviders-go v0.17.3` import to `go.mod`
  * Added `github.com/herdworks/hypertemplates-go v0.17.3` import to `go.mod`
* Adopted `v` prefixed tag naming convention beginning with the `v0.17.3` release

## 0.17.2

See `herdworks/hypertexting-monorepo` for version 0.17.2 and prior changes.

<!-- Links -->
[Keep a Changelog]: https://keepachangelog.com
[Semantic Versioning]: https://semver.org
