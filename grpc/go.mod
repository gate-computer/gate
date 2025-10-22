module gate.computer/grpc

go 1.24.0

tool google.golang.org/grpc/cmd/protoc-gen-go-grpc

require (
	gate.computer v0.0.0-20251020065250-5bad91ccb5ed
	github.com/google/uuid v1.6.0
	google.golang.org/grpc v1.76.0
	google.golang.org/protobuf v1.36.10
	import.name/lock v1.1.0
	import.name/type v1.0.0
)

require (
	gate.computer/wag v0.36.1-0.20250311023511-04b7ed9260b4 // indirect
	github.com/coreos/go-systemd/v22 v22.6.0 // indirect
	github.com/godbus/dbus/v5 v5.1.0 // indirect
	golang.org/x/net v0.46.0 // indirect
	golang.org/x/sys v0.37.0 // indirect
	golang.org/x/text v0.30.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251022142026-3a174f9686a8 // indirect
	google.golang.org/grpc/cmd/protoc-gen-go-grpc v1.5.1 // indirect
)
