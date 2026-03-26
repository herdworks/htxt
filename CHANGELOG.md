# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog], and this project adheres to [Semantic Versioning].

## v0.18.1 (2026-03-25)

* Added `Byline.Username` field for backwards compatibility with early-stage builds of `hyperctl`
* Fixed a bug in `Page.LastModified()` helper that caused a race condition in `hyperctl dev server`
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
