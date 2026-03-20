APP := airpool-server
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build build-linux clean run

build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(APP) ./cmd/airpool-server

build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o bin/$(APP)-linux-amd64 ./cmd/airpool-server

clean:
	rm -rf bin/

run: build
	./bin/$(APP)
