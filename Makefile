.PHONY: build install clean test engine

PREFIX ?= /usr/local
ENGINE_DIR ?= ../trinity-engine
BINDIR ?= $(PREFIX)/bin
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

build:
	go build -ldflags "-X main.version=$(VERSION)" -o bin/trinity ./cmd/trinity
ifdef BUILD_ENGINE
	$(MAKE) engine
endif
	rm -rf web/dist/
	cd web && bun run build

install: build
	install -d $(DESTDIR)$(BINDIR)
	install -m 755 bin/trinity $(DESTDIR)$(BINDIR)/

clean:
	rm -rf bin/
	rm -rf web/dist/

engine:
	$(MAKE) -C $(ENGINE_DIR) web
	rm -rf web/public/engine
	mkdir -p web/public/engine
	cp $(ENGINE_DIR)/dist/engine/loader.js web/public/engine/
	cp $(ENGINE_DIR)/dist/engine/trinity.js web/public/engine/
	cp $(ENGINE_DIR)/dist/engine/trinity.wasm web/public/engine/
	cp $(ENGINE_DIR)/dist/engine/demo-config.json web/public/engine/
	cp $(ENGINE_DIR)/dist/engine/client-config.json web/public/engine/

test:
	go test ./...
