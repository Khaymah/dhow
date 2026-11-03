#!/usr/bin/env bash
set -euo pipefail

bin=/tmp/dhow-server
blobs=/var/lib/dhow-demo/blobs

export DHOW_DEV=true
export DHOW_LISTEN_ADDR=0.0.0.0:8080
export DHOW_PUBLIC_URL=$DHOW_DEMO_PUBLIC_URL
export DHOW_DATABASE_URL="postgres://dhow:dhow-dev-only@db:5432/dhow?sslmode=disable"
export DHOW_SECRET_KEY_FILE=/run/dhow-demo/secret-key
export DHOW_BLOB_DIR=$blobs

install -d -m 0700 "$blobs"
go build -trimpath \
  -ldflags "-X github.com/Khaymah/dhow/internal/buildinfo.checkoutImage=${DHOW_DEMO_CHECKOUT_IMAGE:-}" \
  -o "$bin" ./apps/server/cmd/dhow-server
exec "$bin"
