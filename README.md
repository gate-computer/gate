## Requirements

`make` (native components):

- Linux
- C compiler
- make
- pkg-config
- libcap-dev
- libsystemd-dev unless CGROUP_BACKEND=none is specified

`make bin` (Go programs):

- Go 1.8
- libcapstone-dev
- a number of Go packages which `go get` would get automatically

`make devlibs`:

- Git submodules
- wag-toolchain as Docker image or built manually (set TOOLCHAINDIR)

`make capabilities` (as root after making the native components):

- libcap2-bin

