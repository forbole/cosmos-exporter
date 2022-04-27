COMMIT  := $(shell git log -1 --format='%H')

export GO111MODULE = on

###############################################################################
###                                   All                                   ###
###############################################################################

all: lint test-unit install

###############################################################################
###                                  Build                                  ###
###############################################################################

build: go.sum
	@echo "building cosmos_exporter binary..."
	@go build -mod=readonly -o build/cosmos_exporter ./cmd/cosmos_exporter
.PHONY: build

###############################################################################
###                                 Install                                 ###
###############################################################################

install: go.sum
	@echo "installing cosmos_exporter binary..."
	@go install -mod=readonly ./cmd/cosmos_exporter
.PHONY: install

###############################################################################
###                           Tests & Simulation                            ###
###############################################################################
lint:
	golangci-lint run --out-format=tab
.PHONY: lint

lint-fix:
	golangci-lint run --fix --out-format=tab --issues-exit-code=0
.PHONY: lint-fix

clean:
	rm -f tools-stamp ./build/**
.PHONY: clean