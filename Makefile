.PHONY: build install clean test

PREFIX ?= /usr/local
BINDIR ?= $(PREFIX)/bin
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

build:
	go build -ldflags "-X main.version=$(VERSION)" -o bin/trinity ./cmd/trinity
	npm --prefix web run build

install: build
	install -d $(DESTDIR)$(BINDIR)
	install -m 755 bin/trinity $(DESTDIR)$(BINDIR)/

clean:
	rm -rf bin/
	rm -rf web/dist/

test:
	go test ./...
