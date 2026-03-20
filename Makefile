BINARY_NAME = qmax
VERSION = 4.0.0
BUILD_DIR = build

.PHONY: build test test-race lint vet check clean build-all setup-hooks

build:
	go build -ldflags="-s -w" -o $(BINARY_NAME) .

test:
	go test ./...

test-race:
	go test -race ./...

lint:
	@which golangci-lint > /dev/null 2>&1 || { echo "Install: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; exit 1; }
	golangci-lint run

vet:
	go vet ./...

check: lint vet test-race
	@echo "All checks passed"

setup-hooks:
	@cp scripts/pre-commit .git/hooks/pre-commit
	@chmod +x .git/hooks/pre-commit
	@echo "Pre-commit hook installed"

clean:
	rm -rf $(BUILD_DIR) $(BINARY_NAME)

build-all: clean
	@mkdir -p $(BUILD_DIR)
	GOOS=darwin  GOARCH=amd64 go build -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 .
	GOOS=darwin  GOARCH=arm64 go build -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 .
	GOOS=linux   GOARCH=amd64 go build -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 .
	GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe .
	@echo "Binaries built in $(BUILD_DIR)/"
	@ls -lh $(BUILD_DIR)/
