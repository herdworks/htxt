# AGENTS.md

## Project-specific Instructions

<!-- Project-specific instructions take priority over general instructions -->

* Run tests with `go test ./... -v`
* Always run `go fmt ./...` after modifying source files
* This library is consumed by:
  * `../hyperctl` (CLI / static site generator) 
  * `../libhypertexting` (C API)
    * `../HyperTextingKit` (Swift wrapper around C API, packaged as binary XCFramework)
* Downstream repos pull updates via `go get github.com/herdworks/hypertexting-go@<branch>`; 
  for local testing, downstream repos can also use `replace` directives in `go.mod`

<!-- See https://agents.md for more information -->
