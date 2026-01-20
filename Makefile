.PHONY: build clean test lint verify

VERSION ?= 0.1.0
LDFLAGS := -ldflags "-X main.Version=$(VERSION)"

build:
	go build $(LDFLAGS) -o oxiwatch ./cmd/oxiwatch

verify:
	go build -o /dev/null ./cmd/oxiwatch

clean:
	rm -f oxiwatch

test:
	go test ./...

lint:
	go vet ./...

install: build
	sudo cp oxiwatch /usr/local/bin/
	sudo mkdir -p /etc/oxiwatch /var/lib/oxiwatch
	sudo chown root:root /usr/local/bin/oxiwatch

install-service:
	sudo cp scripts/oxiwatch.service /etc/systemd/system/
	sudo systemctl daemon-reload
