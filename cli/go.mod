module github.com/quillit/cli

go 1.26.3

require (
	github.com/quillit/contentengine v0.0.0
	github.com/spf13/cobra v1.10.2
	gopkg.in/yaml.v3 v3.0.1
)

// Resolved locally via the repo-root go.work workspace, not published —
// see that file's comment for the version string this line needs.
replace github.com/quillit/contentengine => ../pkg/contentengine

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	github.com/yuin/goldmark v1.8.5 // indirect
)
