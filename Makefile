.PHONY: all check test lint semgrep docker-build

all: check

check: lint test semgrep osv-scanner

lint:
	docker run --rm \
		-v $(PWD):/app \
		-v $(shell go env GOCACHE):/root/.cache/go-build \
		-v $(shell go env GOMODCACHE):/go/pkg/mod \
		-v $(HOME)/.cache/golangci-lint:/root/.cache/golangci-lint \
		-w /app \
		-e GOFLAGS="-mod=vendor" \
		golangci/golangci-lint:v2.12.2 \
		golangci-lint run


test:
	go test -v -covermode=atomic -coverprofile=coverage.out -race ./...

semgrep:
	docker run --rm -v $(PWD):/src returntocorp/semgrep:1.106.0 semgrep scan --config=p/default

osv-scanner:
	docker run --rm -e GOTOOLCHAIN=auto -v $(PWD):/src -w /src ghcr.io/google/osv-scanner:latest -r .

docker-build:
	docker build -t degen .
