.PHONY: build clean test test-race cover cover-html lint verify

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null | sed 's/^v//' || echo "dev")
LDFLAGS := -ldflags "-X main.Version=$(VERSION)"

build:
	go build $(LDFLAGS) -o oxiwatch ./cmd/oxiwatch

verify:
	go build -o /dev/null ./cmd/oxiwatch

clean:
	rm -f oxiwatch

test:
	go test ./...

test-race:
	go test -race ./...

cover:
	go test -race -covermode=atomic -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1

cover-html: cover
	go tool cover -html=coverage.out

lint:
	go vet ./...

install: build
	sudo cp oxiwatch /usr/local/bin/
	sudo mkdir -p /etc/oxiwatch /var/lib/oxiwatch
	sudo cp docs/config.json.example /etc/oxiwatch/config.json.example
	sudo chown root:root /usr/local/bin/oxiwatch

install-service:
	sudo cp scripts/oxiwatch.service /etc/systemd/system/
	sudo systemctl daemon-reload
