// hypertexting-go module
//
// reference: https://go.dev/doc/modules/gomod-ref#replace
module github.com/herdworks/hypertexting-go

go 1.25.0

// first-party dependencies
require (
	github.com/herdworks/hyperfeed-go v0.19.1
	github.com/herdworks/hypergraph-go v0.19.0
	github.com/herdworks/hyperproviders-go v0.19.0
	github.com/herdworks/hypertemplates-go v0.27.0
)

// direct dependencies
require (
	github.com/google/uuid v1.6.0
	golang.org/x/net v0.47.0
	golang.org/x/sync v0.20.0
	gopkg.in/yaml.v3 v3.0.1
)

// local development dependencies (replacements)
//
// uncomment to use local dependencies
// replace github.com/herdworks/hypergraph-go => ../hypergraph-go
// replace github.com/herdworks/hyperfeed-go => ../hyperfeed-go
// replace github.com/herdworks/hyperproviders-go => ../hyperproviders-go
// replace github.com/herdworks/hypertemplates-go => ../hypertemplates-go

// transitive dependencies
require (
	github.com/alecthomas/chroma/v2 v2.2.0 // indirect
	github.com/andybalholm/cascadia v1.3.2 // indirect
	github.com/dlclark/regexp2 v1.11.4 // indirect
	github.com/dop251/goja v0.0.0-20260311135729-065cd970411c // indirect
	github.com/go-sourcemap/sourcemap v2.1.3+incompatible // indirect
	github.com/google/pprof v0.0.0-20230207041349-798e818bf904 // indirect
	github.com/yuin/goldmark v1.7.8 // indirect
	github.com/yuin/goldmark-emoji v1.0.4 // indirect
	github.com/yuin/goldmark-highlighting/v2 v2.0.0-20230729083705-37449abec8cc // indirect
	github.com/yuin/goldmark-meta v1.1.0 // indirect
	golang.org/x/text v0.31.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
)
