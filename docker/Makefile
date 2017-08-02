TAG_PREFIX :=

build:
	$(MAKE) -C .. CGROUP_BACKEND=none
	docker build -f Dockerfile --no-cache -t $(TAG_PREFIX)gate ..
	docker build -f containerd/Dockerfile --no-cache -t $(TAG_PREFIX)gate-containerd ..
	docker build -f server/Dockerfile --no-cache -t $(TAG_PREFIX)gate-server ..
	docker build -f server-unpriv/Dockerfile --no-cache -t $(TAG_PREFIX)gate-server-unpriv ..
	docker build -f server-priv/Dockerfile --no-cache -t $(TAG_PREFIX)gate-server-priv ..

.PHONY: build
