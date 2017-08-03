## Requirements

make:

- Linux
- C compiler
- Go 1.8
- pkg-config
- libcap-dev and libcap2-bin
- libcapstone-dev
- libsystemd-dev unless CGROUP_BACKEND=none is specified

make all:

- Git submodules
- wag-toolchain as Docker image or built manually (set TOOLCHAINDIR)

make capabilities:

- libcap2-bin

