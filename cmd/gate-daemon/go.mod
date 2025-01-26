module gate.computer/cmd/gate-daemon

go 1.23

replace (
	gate.computer => ../../
	gate.computer/grpc => ../../grpc/
	gate.computer/localhost => ../../localhost/
	gate.computer/otel => ../../otel/
)

require (
	gate.computer v0.0.0-00010101000000-000000000000
	gate.computer/grpc v0.0.0-00010101000000-000000000000
	gate.computer/localhost v0.0.0-00010101000000-000000000000
	gate.computer/otel v0.0.0-00010101000000-000000000000
	gate.computer/wag v0.36.1-0.20240923211841-04ccc6280731
	github.com/coreos/go-systemd/v22 v22.5.0
	github.com/godbus/dbus/v5 v5.1.0
	go.opentelemetry.io/otel v1.34.0
	go.opentelemetry.io/otel/trace v1.34.0
	google.golang.org/protobuf v1.36.4
	import.name/confi v1.6.0
	import.name/pan v0.2.0
	import.name/type v1.0.0
	kernel.org/pub/linux/libs/security/libcap/cap v1.2.66
	modernc.org/sqlite v1.34.5
)

require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/google/flatbuffers v25.1.24+incompatible // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gorilla/websocket v1.5.0 // indirect
	github.com/knightsc/gapstone v0.0.0-20211014144438-5e0e64002a6e // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/naoina/go-stringutil v0.1.0 // indirect
	github.com/naoina/toml v0.1.1 // indirect
	github.com/ncruces/go-strftime v0.1.9 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.opentelemetry.io/otel/metric v1.34.0 // indirect
	golang.org/x/net v0.33.0 // indirect
	golang.org/x/sys v0.28.0 // indirect
	golang.org/x/text v0.21.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20241223144023-3abc09e42ca8 // indirect
	google.golang.org/grpc v1.70.0 // indirect
	import.name/flux v1.0.0 // indirect
	import.name/lock v1.1.0 // indirect
	import.name/sjournal v1.0.0 // indirect
	kernel.org/pub/linux/libs/security/libcap/psx v1.2.66 // indirect
	modernc.org/libc v1.55.3 // indirect
	modernc.org/mathutil v1.6.0 // indirect
	modernc.org/memory v1.8.0 // indirect
)
