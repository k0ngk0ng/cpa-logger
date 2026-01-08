.PHONY: build clean install test

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
BUILD_TIME ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS := -s -w \
	-X main.version=$(VERSION) \
	-X main.commit=$(COMMIT) \
	-X main.buildTime=$(BUILD_TIME)

build:
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o cpa-logger ./cmd/cpa-logger

build-linux-amd64:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o cpa-logger-linux-amd64 ./cmd/cpa-logger

build-linux-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o cpa-logger-linux-arm64 ./cmd/cpa-logger

clean:
	rm -f cpa-logger cpa-logger-linux-*

install: build
	sudo cp cpa-logger /usr/local/bin/
	sudo mkdir -p /etc/cpa-logger
	@if [ ! -f /etc/cpa-logger/config.yaml ]; then \
		sudo cp deploy/config.yaml /etc/cpa-logger/; \
	fi
	sudo cp deploy/cpa-logger.service /etc/systemd/system/
	sudo systemctl daemon-reload

test:
	go test -v ./...

run:
	go run ./cmd/cpa-logger -config deploy/config.yaml
