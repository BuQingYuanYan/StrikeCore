.PHONY: build run test lint fmt vet clean build-all

BINARY := strike-core
VERSION := $(shell git describe --tags --always 2>/dev/null || echo dev)
LDFLAGS := -ldflags "-s -w -X strike-core/internal/config.buildVersion=$(VERSION)"

build:
	go build $(LDFLAGS) -o $(BINARY).exe .

run:
	go run .

test:
	go test ./...

# Race detector requires a C toolchain (CGO). Used in CI.
test-race:
	CGO_ENABLED=1 go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

lint: vet
	@echo "checking formatting..."
	@test -z "$$(gofmt -l .)" || (echo "unformatted files:"; gofmt -l .; exit 1)

# Cross-compile a release binary for each supported platform into dist/.
build-all:
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o dist/$(BINARY)-windows-amd64.exe .
	GOOS=linux   GOARCH=amd64 go build $(LDFLAGS) -o dist/$(BINARY)-linux-amd64 .
	GOOS=darwin  GOARCH=arm64 go build $(LDFLAGS) -o dist/$(BINARY)-darwin-arm64 .

clean:
	rm -f $(BINARY).exe $(BINARY) test_build.exe
	rm -rf dist/
