GOBIN ?= $(shell which go)
GOOS ?= "linux"
GOARCH ?= "amd64"

default:
	@echo "Targets: clean, build"

clean:
	rm -rf benchmark


ifeq ($(strip $(GOBIN)),)
build:
	@echo "No go binary in path! Please specify GOBIN to point to your go binary"
	exit 1
else
build:
	GOOS=$(GOOS) GOARCH=$(GOARCH) $(GOBIN) build -o benchmark ./cmd
endif
