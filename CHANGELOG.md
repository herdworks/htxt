# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog], and this project adheres to [Semantic Versioning].

## Unreleased

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
