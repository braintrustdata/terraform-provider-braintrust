NAME=braintrust
BINARY=terraform-provider-$(NAME)

default: install

build:
	go build -ldflags "-w -s" -o $(BINARY) .

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

lint:
	golangci-lint run ./...

.PHONY: default build install generate scaffold-resource scaffold-datasource doc test test-acceptance check-docs deps lint
