module github.com/modelrelay/modelrelay/cmd/mrl

go 1.25.4

require (
	github.com/google/uuid v1.6.0
	github.com/modelrelay/modelrelay/sdk/go v0.0.0
	github.com/modelrelay/modelrelay/providers v0.0.0-00010101000000-000000000000
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/evanphx/json-patch/v5 v5.9.11 // indirect
	github.com/modelrelay/modelrelay/platform v0.0.0-00010101000000-000000000000 // indirect
	github.com/oapi-codegen/runtime v1.1.2 // indirect
	github.com/santhosh-tekuri/jsonschema/v5 v5.3.1 // indirect
	go.opentelemetry.io/otel v1.39.0 // indirect
	go.opentelemetry.io/otel/trace v1.39.0 // indirect
)

replace github.com/modelrelay/modelrelay/sdk/go => ../../sdk/go

replace github.com/modelrelay/modelrelay/platform => ../../platform

replace github.com/modelrelay/modelrelay/providers => ../../providers

replace github.com/modelrelay/modelrelay/billing => ../../billing
