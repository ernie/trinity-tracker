#!/bin/sh
# Usual path for syncing WASM builds: clean engine rebuild + deploy in one
# step. Run from the repo root.
set -e
make -C ../trinity-engine clean-web
BUILD_ENGINE=1 make deploy-frontend
