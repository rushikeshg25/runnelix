.PHONY: tools deps build build-phase1 test check integration integration-phase1 integration-phase2 build-phase2 dev format catalog

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
	node --experimental-strip-types --test tests/web/*.test.ts

check:
	python3 scripts/build-catalog.py --check
	test -z "$$(gofmt -l cmd internal contracts/product tests/integration)"
	go vet ./...
	pnpm check
	pnpm check:web
	git diff --check

integration: build
	python3 tests/integration/phase0.py

catalog:
	python3 scripts/build-catalog.py

format:
	gofmt -w cmd internal contracts/product tests/integration/phase1probe

build-phase1:
	python3 scripts/build-catalog.py --check
	go build -o .local/bin/runnelix ./cmd/runnelix
	go build -o .local/bin/phase1-probe ./tests/integration/phase1probe
	pnpm build

integration-phase1: build-phase1
	python3 tests/integration/postgres.py go test -race ./internal/store
	python3 tests/integration/phase1.py

build-phase2: build-phase1
	pnpm build:web

integration-phase2: build-phase2
	python3 tests/integration/postgres.py go test -race ./internal/store
	python3 tests/integration/phase2.py

dev: build-phase2
	python3 scripts/dev.py

.PHONY: integration-phase3
integration-phase3: build-phase2
	python3 scripts/build-compat-v2.py
	python3 tests/integration/postgres.py go test -race ./internal/store
	python3 tests/integration/phase3.py

.PHONY: integration-phase4
integration-phase4: build-phase2
	python3 tests/integration/postgres.py go test -race ./internal/store
	python3 tests/integration/phase4.py

.PHONY: integration-phase5
integration-phase5:
	python3 tests/integration/postgres.py go test -race -p 1 ./internal/store ./internal/identity
