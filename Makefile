VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: all build build-all test lint docker clean

all: build

build:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/dockrouter ./cmd/dockrouter

build-all:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/dockrouter-linux-amd64 ./cmd/dockrouter
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/dockrouter-linux-arm64 ./cmd/dockrouter
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/dockrouter-darwin-amd64 ./cmd/dockrouter
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/dockrouter-darwin-arm64 ./cmd/dockrouter

test:
	go test -v -race -cover ./...

lint:
	go vet ./...

docker:
	docker build -t dockrouter:$(VERSION) .

docker-push:
	docker tag dockrouter:$(VERSION) ghcr.io/dockrouter/dockrouter:$(VERSION)
	docker push ghcr.io/dockrouter/dockrouter:$(VERSION)

clean:
	rm -rf bin/

run: build
	./bin/dockrouter
