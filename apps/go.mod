// The client apps are not Go code. This file makes apps/ a separate module so that
// `go vet ./...` and `go test ./...` in the repository root skip it (and node_modules).
module kinora-apps

go 1.24
