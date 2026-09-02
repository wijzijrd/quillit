module github.com/quillit/messaging-svc

go 1.27.0

require (
	connectrpc.com/connect v1.20.0
	github.com/go-chi/chi/v5 v5.2.5
	github.com/quillit/gen v0.0.0-00010101000000-000000000000
	github.com/swaggo/http-swagger v1.3.4
	github.com/swaggo/swag v1.16.6
	golang.org/x/net v0.57.0
)

// Resolved locally via the repo-root go.work workspace (see that file's
// comment for the version string this line needs) — the connectrpc
// scaffolding module from Task 8, imported for real for the first time by
// this task's MessagingInternalService server.
replace github.com/quillit/gen => ../../gen

require (
	github.com/KyleBanks/depth v1.2.1 // indirect
	github.com/go-openapi/jsonpointer v0.19.5 // indirect
	github.com/go-openapi/jsonreference v0.20.0 // indirect
	github.com/go-openapi/spec v0.20.6 // indirect
	github.com/go-openapi/swag v0.19.15 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/mailru/easyjson v0.7.6 // indirect
	github.com/swaggo/files v0.0.0-20220610200504-28940afbdbfe // indirect
	golang.org/x/mod v0.37.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/text v0.40.0 // indirect
	golang.org/x/tools v0.47.0 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
)
