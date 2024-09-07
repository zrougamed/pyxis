.PHONY: build test lint clean install run run-web compose-up compose-dex cross-build

# Variables
BINARY    := pyxis
VERSION   := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS   := -ldflags "-s -w -X main.version=$(VERSION)"
GOFLAGS   := -trimpath
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64

# Default target
all: test build

# Build for current platform
build:
	go build $(GOFLAGS) $(LDFLAGS) -o bin/$(BINARY) ./cmd/

# Install to $GOPATH/bin
install:
	go install $(GOFLAGS) $(LDFLAGS) ./cmd/

# Run the TUI
run: build
	./bin/$(BINARY)


# Run the responsive web UI and API
run-web: build
	./bin/$(BINARY) web --no-auth

run-web-auth: build
	./bin/$(BINARY) web --cookie-secret dev-cookie-secret

# Start the full Docker Compose stack (Dex + pyxis web)
compose-up:
	docker compose up --build

# Start only Dex for use with a locally-running `pyxis web`
compose-dex:
	docker compose -f docker-compose.dex.yml up

# Run all tests
test:
	go test -v -race -count=1 ./...

# Run tests with coverage
test-coverage:
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Lint (requires golangci-lint)
lint:
	golangci-lint run ./...

# Cross-compile for all platforms
cross-build:
	@for platform in $(PLATFORMS); do \
		os=$${platform%/*}; \
		arch=$${platform#*/}; \
		ext=""; \
		if [ "$$os" = "windows" ]; then ext=".exe"; fi; \
		echo "Building $$os/$$arch..."; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch \
			go build $(GOFLAGS) $(LDFLAGS) \
			-o bin/$(BINARY)-$$os-$$arch$$ext ./cmd/; \
	done

# Clean build artifacts
clean:
	rm -rf bin/ coverage.out coverage.html

# Update dependencies
tidy:
	go mod tidy

# Update Kubernetes client-go to latest compatible version
update-k8s:
	go get k8s.io/api@latest
	go get k8s.io/apimachinery@latest
	go get k8s.io/client-go@latest
	go mod tidy
	@echo "Kubernetes dependencies updated. Run 'make test' to verify."
