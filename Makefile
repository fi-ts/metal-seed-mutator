export CGO_ENABLED := 0

SHA := $(shell git rev-parse --short=8 HEAD)
GITVERSION := $(shell git describe --long --all)
# gnu date format iso-8601 is parsable with Go RFC3339
BUILDDATE := $(shell date --iso-8601=seconds)
VERSION := $(or ${VERSION},$(shell git describe --tags --exact-match 2> /dev/null || git symbolic-ref -q --short HEAD || git rev-parse --short HEAD))

MINI_LAB_KUBECONFIG := $(shell pwd)/../mini-lab/.kubeconfig

LINKMODE := -X 'github.com/metal-stack/v.Version=$(VERSION)' \
		    -X 'github.com/metal-stack/v.Revision=$(GITVERSION)' \
		    -X 'github.com/metal-stack/v.GitSHA1=$(SHA)' \
		    -X 'github.com/metal-stack/v.BuildDate=$(BUILDDATE)'


all: metal-seed-mutator

.PHONY: metal-seed-mutator
metal-seed-mutator:
	go build \
		-tags 'osusergo netgo' \
		-ldflags \
		"$(LINKMODE)" \
		-o metal-seed-mutator \
		./...
	strip metal-seed-mutator
