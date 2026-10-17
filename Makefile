.PHONY: tools deps build test check integration

tools:
	python3 scripts/install-temporal.py

deps:
	go mod download
	pnpm install --frozen-lockfile

build:
	go build -o .local/bin/spike ./cmd/spike
	go build -o .local/bin/mock-provider ./cmd/mock-provider
	pnpm build

test:
	go test -race ./...
	pnpm build
	pnpm test

check:
	go vet ./...
	pnpm check
	git diff --check

integration: build
	python3 tests/integration/phase0.py
