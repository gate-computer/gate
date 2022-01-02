.PHONY: help
help:
	@ go run make.go --help
	@ false

%:
	@ go run make.go --help
	@ false
