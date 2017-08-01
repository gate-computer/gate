TAG	:= gate-server

build:
	$(MAKE) -C .. CGROUP_BACKEND=none
	docker build -f Dockerfile -t $(TAG) ..

.PHONY: build
