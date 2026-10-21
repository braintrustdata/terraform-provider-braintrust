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

# Code generation pipeline: OpenAPI spec -> provider code spec -> Go code
generate:
	tfplugingen-openapi generate \
		--config ./generator_config.yml \
		--output ./provider_code_spec.json \
		./openapi_spec.json
	tfplugingen-framework generate resources \
		--input ./provider_code_spec.json \
		--output ./internal
	tfplugingen-framework generate data-sources \
		--input ./provider_code_spec.json \
		--output ./internal

# Scaffold a new resource (usage: make scaffold-resource NAME=foo)
scaffold-resource:
	tfplugingen-framework scaffold resource \
		--name $(NAME) \
		--output-dir ./internal/provider

# Scaffold a new data source (usage: make scaffold-datasource NAME=foo)
scaffold-datasource:
	tfplugingen-framework scaffold data-source \
		--name $(NAME) \
		--output-dir ./internal/provider

doc:
	go generate ./...

test: deps
	go test -mod=readonly -count=1 ./...

test-acceptance: deps
	TF_ACC=1 go test -v -mod=readonly -count=1 -p 5 -parallel 10 ./...

check-docs: doc
	git diff --exit-code -- docs

deps:
	go mod tidy

fmt:
	gofmt -w internal/

lint:
	golangci-lint run ./...

lint-fix:
	gofmt -w internal/
	golangci-lint run --fix ./...

# Install a git pre-commit hook that runs formatting and lint checks.
install-hooks:
	@echo '#!/usr/bin/env bash' > .git/hooks/pre-commit
	@echo 'set -e' >> .git/hooks/pre-commit
	@echo '' >> .git/hooks/pre-commit
	@echo '# Check formatting' >> .git/hooks/pre-commit
	@echo 'UNFORMATTED=$$(gofmt -l internal/)' >> .git/hooks/pre-commit
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

.PHONY: default build build-terraforming install generate scaffold-resource scaffold-datasource doc test test-acceptance check-docs deps fmt lint lint-fix install-hooks
