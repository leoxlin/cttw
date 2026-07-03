.PHONY: build test run

build:
	go build -o bin/cttw ./cmd/cttw

test:
	go test ./...

run:
	go run ./cmd/cttw
