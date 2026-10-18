#!/bin/sh
# Rebuilds the engine's web build from scratch and deploys it. Run from
# the repo root.
set -e
make -C ../trinity-engine clean-web
BUILD_ENGINE=1 make deploy-frontend
