GO ?= go
GORELEASER ?= goreleaser
IMAGE ?= lantern:dev
GOEXE := $(shell $(GO) env GOEXE)
BUILD_OUT := dist/lantern$(GOEXE)

.PHONY: build test test-integration lint docker release-dry

build:
	mkdir -p dist
	$(GO) build -o $(BUILD_OUT) ./cmd/lantern

test:
	$(GO) test ./...

test-integration:
	$(GO) test ./test/integration/... -tags integration

lint:
	golangci-lint run ./...

docker:
	docker build -t $(IMAGE) .

release-dry:
	$(GORELEASER) release --snapshot --clean
