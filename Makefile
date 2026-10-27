NAME=braintrust
BINARY=terraform-provider-$(NAME)

default: install

build:
	go build -ldflags "-w -s" -o $(BINARY) .

build-terraforming:
	go build -ldflags "-w -s" -o braintrust-terraforming ./cmd/braintrust-terraforming

install: build
	mkdir -p $(HOME)/.terraform.d/plugins
	mv ./$(BINARY) $(HOME)/.terraform.d/plugins/$(BINARY)

doc:
	go generate ./...

test: deps
	go test -mod=readonly -count=1 ./...

test-acceptance: deps
	./scripts/test-acceptance.sh

test-acceptance-local: deps
	BRAINTRUST_API_URL=http://localhost:8000 ./scripts/test-acceptance.sh

check-docs: doc
	@test -z "$$(git status --porcelain -- docs)" || (git status --short -- docs; echo "docs/ is out of date; commit the regenerated files"; exit 1)

check-versions:
	./scripts/check-tool-versions.sh

check-fmt:
	@test -z "$$(gofmt -l .)" || (gofmt -l .; echo "Go files are not formatted; run make fmt"; exit 1)
	terraform fmt -check -diff -recursive

check-tidy:
	go mod tidy -diff

vet:
	go vet -mod=readonly ./...

# Everything CI and releases verify. Reports problems instead of fixing them.
check: check-versions check-fmt check-tidy vet lint check-docs
	go build -mod=readonly ./...
	go test -mod=readonly -count=1 ./...

deps:
	go mod tidy

fmt:
	gofmt -w .
	terraform fmt -recursive

lint:
	golangci-lint run ./...

lint-fix:
	gofmt -w .
	terraform fmt -recursive
	golangci-lint run --fix ./...

# Install a git pre-commit hook that runs formatting and lint checks.
install-hooks:
	@echo '#!/usr/bin/env bash' > .git/hooks/pre-commit
	@echo 'set -e' >> .git/hooks/pre-commit
	@echo '' >> .git/hooks/pre-commit
	@echo '# Check formatting' >> .git/hooks/pre-commit
	@echo 'UNFORMATTED=$$(gofmt -l .)' >> .git/hooks/pre-commit
	@echo 'if [ -n "$$UNFORMATTED" ]; then' >> .git/hooks/pre-commit
	@echo '  echo "Error: Go files not formatted. Run: make fmt"' >> .git/hooks/pre-commit
	@echo '  echo "$$UNFORMATTED"' >> .git/hooks/pre-commit
	@echo '  exit 1' >> .git/hooks/pre-commit
	@echo 'fi' >> .git/hooks/pre-commit
	@echo '' >> .git/hooks/pre-commit
	@echo '# Run go vet' >> .git/hooks/pre-commit
	@echo 'go vet ./...' >> .git/hooks/pre-commit
	@echo '' >> .git/hooks/pre-commit
	@echo '# Run linter (if installed)' >> .git/hooks/pre-commit
	@echo 'if command -v golangci-lint &> /dev/null; then' >> .git/hooks/pre-commit
	@echo '  golangci-lint run ./...' >> .git/hooks/pre-commit
	@echo 'fi' >> .git/hooks/pre-commit
	@chmod +x .git/hooks/pre-commit
	@echo "Pre-commit hook installed."

.PHONY: default build build-terraforming install doc test test-acceptance test-acceptance-local check-docs check-versions check-fmt check-tidy vet check deps fmt lint lint-fix install-hooks
