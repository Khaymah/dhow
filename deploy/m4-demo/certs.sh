#!/usr/bin/env bash
set -euo pipefail
umask 077

dir=${1:?usage: certs.sh DIR}
cd -- "$dir"
openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -days 2 -subj "/CN=Dhow demo CA" \
  -addext basicConstraints=critical,CA:TRUE -addext keyUsage=critical,keyCertSign -keyout ca.key -out ca.crt 2>/dev/null
openssl req -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -subj "/CN=registry.dhow-demo.test" \
  -keyout registry.key -out registry.csr 2>/dev/null
printf 'subjectAltName=DNS:registry.dhow-demo.test,DNS:ghcr.io,DNS:localhost,IP:127.0.0.1\nbasicConstraints=CA:FALSE\nextendedKeyUsage=serverAuth\n' >registry.ext
openssl x509 -req -in registry.csr -CA ca.crt -CAkey ca.key -CAcreateserial -days 2 -extfile registry.ext -out registry.crt 2>/dev/null
rm -f -- ca.key ca.srl registry.csr registry.ext
chmod 0644 ca.crt registry.crt
chmod 0755 .
