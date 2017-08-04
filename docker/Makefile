TAG_PREFIX :=

build:
	$(MAKE) -C .. all CGROUP_BACKEND=none
	docker build -f Dockerfile --no-cache -t gate ..
	docker build -f containerd/Dockerfile --no-cache -t gate-containerd ..
	docker build -f server/Dockerfile --no-cache -t gate-server ..
	docker build -f server-unpriv/Dockerfile --no-cache -t gate-server-unpriv ..
	docker build -f server-priv/Dockerfile --no-cache -t gate-server-priv ..
	docker build -f toolchain/Dockerfile --no-cache -t gate-toolchain ..

tag:
	docker tag gate $(TAG_PREFIX)gate
	docker tag gate-containerd $(TAG_PREFIX)gate-containerd
	docker tag gate-server $(TAG_PREFIX)gate-server
	docker tag gate-server-unpriv $(TAG_PREFIX)gate-server-unpriv
	docker tag gate-server-priv $(TAG_PREFIX)gate-server-priv
	docker tag gate-toolchain $(TAG_PREFIX)gate-toolchain

.PHONY: build tag
