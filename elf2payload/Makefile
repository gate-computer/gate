GO		?= go

build: elf2payload

elf2payload: elf2payload.go ../run/run.go
	$(GO) build -o $@ .

clean:
	rm -f elf2payload

.PHONY: build clean
