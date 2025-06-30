module gate.computer/grpc

go 1.24

tool google.golang.org/grpc/cmd/protoc-gen-go-grpc

require (
	gate.computer v0.0.0-20250131054915-e694ff662681
	github.com/google/uuid v1.6.0
	google.golang.org/grpc v1.70.0
	google.golang.org/protobuf v1.36.4
	import.name/lock v1.1.0
	import.name/type v1.0.0
)

require (
	gate.computer/wag v0.36.1-0.20240923211841-04ccc6280731 // indirect
	github.com/coreos/go-systemd/v22 v22.5.0 // indirect
	github.com/godbus/dbus/v5 v5.1.0 // indirect
	golang.org/x/net v0.41.0 // indirect
	golang.org/x/sys v0.33.0 // indirect
	golang.org/x/text v0.26.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20241223144023-3abc09e42ca8 // indirect
	google.golang.org/grpc/cmd/protoc-gen-go-grpc v1.5.1 // indirect
)
