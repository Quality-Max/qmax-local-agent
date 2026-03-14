BINARY_NAME = qmax
VERSION = 3.0.0
BUILD_DIR = build

.PHONY: build test clean build-all

build:
	go build -ldflags="-s -w" -o $(BINARY_NAME) .

test:
	go test ./...

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
