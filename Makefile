.PHONY: build test lint fmt

build:
	go build ./cmd/ip

test:
	go test ./...

lint:
	golangci-lint run

fmt:
	gofmt -w cmd internal
