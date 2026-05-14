.PHONY: build install clean test engine deploy deploy-backend deploy-frontend

PREFIX ?= /usr/local
ENGINE_DIR ?= ../trinity-engine
BINDIR ?= $(PREFIX)/bin
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

# Deploy targets push freshly-built artifacts to the running install.
# Overridable for non-default service users / install roots.
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

# Deploy = build (as you, for mise PATH) + sudo-only file moves.
# `sudo make deploy` would strip mise's PATH and break the build — run plain `make deploy`.
deploy: deploy-frontend deploy-backend

# NB: no --delete. The webroot also holds runtime-extracted assets from
# `trinity assets` (levelshots/, portraits/, skills/, maps.json, and the
# extracted contents of medals/, flags/, icons/) that live nowhere in
# web/dist/ — a blind --delete would wipe them. The only stale files
# we actually need to purge are the content-hashed bundles from prior
# builds, so we do that explicitly in the second step.
#
# --chown gives fresh files to $(DEPLOY_USER) so the service can read
# them without a post-hoc chown -R. rsync writes to a tmp file then
# renames, so a partial failure can't take the site down the way the
# old rm-then-cp pattern could.
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
