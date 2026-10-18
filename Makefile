.PHONY: build install clean test engine deploy deploy-backend deploy-frontend

PREFIX ?= /usr/local
ENGINE_DIR ?= ../trinity-engine
BINDIR ?= $(PREFIX)/bin
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

# Override these for a different service user or install root.
DEPLOY_USER ?= quake
DEPLOY_WEB ?= /var/lib/trinity/web
SERVICE ?= trinity

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

# Run plain `make deploy`: it builds as you, since sudo drops mise's PATH,
# and uses sudo only to move files.
deploy: deploy-frontend deploy-backend

# The webroot also holds what `trinity assets` extracts (levelshots/,
# portraits/, skills/, maps.json, and the contents of medals/, flags/
# and icons/), so the second step removes only stale content-hashed
# bundles. rsync writes each file under a temporary name and renames
# it, so a failed deploy leaves the site serving the old files.
deploy-frontend: build
	sudo rsync -a --chown=$(DEPLOY_USER):$(DEPLOY_USER) web/dist/ $(DEPLOY_WEB)/
	@for f in $(DEPLOY_WEB)/assets/main-*.js $(DEPLOY_WEB)/assets/main-*.css; do \
		[ -e "$$f" ] || continue; \
		base=$$(basename "$$f"); \
		if [ ! -f web/dist/assets/$$base ]; then \
			echo "rm stale bundle: $$base"; \
			sudo rm -f "$$f"; \
		fi; \
	done

deploy-backend: build
	sudo install -m 755 bin/trinity $(BINDIR)/
	sudo systemctl restart $(SERVICE)

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
