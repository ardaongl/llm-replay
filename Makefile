BINARY := bin/llm-replay
GO ?= go
RACE_FLAG := -race

ifeq ($(OS),Windows_NT)
BINARY := bin/llm-replay.exe
RACE_FLAG :=
endif

.PHONY: build arena test lint fmt examples smoke snapshot clean

build:
	$(GO) build -trimpath -ldflags "-s -w" -o $(BINARY) ./cmd/llm-replay

ARENA_ARGS ?=
arena: build
	$(BINARY) arena $(ARENA_ARGS)

test:
	$(GO) test -v $(RACE_FLAG) ./...

lint:
	golangci-lint run

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './vendor/*')

examples:
	$(GO) run ./scripts/generate-example-dataset

smoke:
	$(GO) run ./scripts/smoke-test

snapshot:
	goreleaser release --snapshot --clean

clean:
	$(GO) clean
	rm -rf ./bin
