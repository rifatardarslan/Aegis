.PHONY: build install dev debug clean tidy vet

build:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o aegis ./cmd/aegis

install: build
	cp aegis $(shell go env GOPATH)/bin/aegis
	chmod +x $(shell go env GOPATH)/bin/aegis

dev:
	go run ./cmd/aegis

debug:
	go run ./cmd/aegis --debug

clean:
	rm -f aegis

tidy:
	go mod tidy

vet:
	go vet ./...
