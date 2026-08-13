# AGENTS.md

## Project-specific Instructions

<!-- Project-specific instructions take priority over general instructions -->

* Run tests with `go test ./... -v`
* Always run `go fmt ./...` after modifying source files
* This library is consumed by:
  * `../hyperctl` (CLI / static site generator) 
  * `../libhypertexting` (C API)
    * `../HyperTextingKit` (Swift wrapper around C API, packaged as binary XCFramework)
* Downstream repos pull updates via `go get github.com/herdworks/htxt@<branch>`;
  for local testing, downstream repos can also use `replace` directives in `go.mod`
* Only bump the library version (`version.go`) from the `main` branch. On other branches, record breaking changes (and other notable changes) under the `Unreleased` section of `CHANGELOG.md`; the version bump happens at release time on `main`
* Format all `clic` command, option, metadata, specification, and options-map struct literals in fully expanded multi-line form. Never use inline or single-line composite literals for `clic.Command`, `clic.Option`, `clic.CommandMeta`, `clic.CommandSpec`, `clic.OptionMeta`, `clic.OptionSpec`, or `clic.Options`, even when the literal is short. Put one field per line and expand nested literals recursively.

### Release Instructions

* Release commits and tags must only be cut from the `main` branch
* Before creating a release commit, verify that `main` is cleanly formatted, passes tests, and builds by running `go fmt ./...`, `go test ./... -v`, and `go build ./...`
* Move the relevant `CHANGELOG.md` entries from `Unreleased` into `## vX.Y.Z (YYYY-MM-DD)`
* Update `version.go` with `LibVersion = "vX.Y.Z"`
* Set `LibReleaseDate` to the current local time rounded down to the most recent hour; for example, `2026-05-11T10:36:31-07:00` becomes `2026-05-11T10:00:00-07:00`
* After the version/changelog edits, run `go fmt ./...`, `go test ./... -v`, and `go build ./...` again
* Commit as `updated changelog and bumped version for X.Y.Z`
* Create an annotated tag `vX.Y.Z` with message `release version vX.Y.Z`
* Tags should not skip version numbers (e.g. don't jump from v0.1.0 to v0.3.0)

<!-- See https://agents.md for more information -->
