#!/usr/bin/env bash
set -euo pipefail

fail() {
  printf 'dist.sh: %s\n' "$1" >&2
  exit 1
}

if [[ $# -ne 2 || -z "$1" || -z "$2" ]]; then
  printf 'usage: %s VERSION COMMIT\n' "$0" >&2
  exit 2
fi

version=$1
commit=$2
root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
out=${DIST_DIR:-$root/dist}
arches=${DIST_ARCHES:-amd64 arm64}
components=${DIST_COMPONENTS:-server runner cli}
cli_oses=${DIST_CLI_OSES:-linux darwin windows}
module=github.com/Khaymah/dhow
checkout_image=${CHECKOUT_IMAGE:-}

[[ "$version" =~ ^[0-9A-Za-z][0-9A-Za-z.+-]*$ ]] || fail "VERSION must look like 1.2.3 or 1.2.3-rc.1, got: $version"
if [[ -n "$checkout_image" && ! "$checkout_image" =~ ^[a-z0-9.-]+(:[0-9]+)?/[a-z0-9._/-]+@sha256:[0-9a-f]{64}$ ]]; then
  fail "CHECKOUT_IMAGE must be registry/name@sha256:<64 hex>, got: $checkout_image"
fi
for component in $components; do
  case $component in
    server | runner | cli) ;;
    *) fail "DIST_COMPONENTS lists server, runner and cli only, got: $component" ;;
  esac
done
for os in $cli_oses; do
  case $os in
    linux | darwin | windows) ;;
    *) fail "DIST_CLI_OSES lists linux, darwin and windows only, got: $os" ;;
  esac
done
if [[ " $components " == *" server "* ]]; then
  [[ -f "$root/apps/server/internal/App/WebDist/index.html" ]] || fail "the web app is missing: run make web first"
fi
epoch=${SOURCE_DATE_EPOCH:-$(git -C "$root" log -1 --format=%ct)}

archive() {
  local component=$1 arch=$2 binary="dhow-$1" unit
  case $component in
    server) unit=dhow.service ;;
    runner) unit=dhow-runner.service ;;
  esac
  local name="${binary}_${version}_linux_${arch}"
  local stage="$out/$name"
  local baked=""
  if [[ $component == runner ]]; then
    baked="-X $module/internal/buildinfo.checkoutImage=$checkout_image"
  fi
  mkdir -p -- "$stage"
  (
    cd -- "$root"
    CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath -buildvcs=false \
      -ldflags "-s -w -buildid= -X $module/internal/buildinfo.version=$version -X $module/internal/buildinfo.commit=$commit $baked" \
      -o "$stage/$binary" "./apps/$component/cmd/$binary"
  )
  cp -- "$root/deploy/systemd/$unit" "$root/LICENSE" "$root/README.md" "$stage/"
  chmod 0755 "$stage/$binary"
  chmod 0644 "$stage/$unit" "$stage/LICENSE" "$stage/README.md"
  tar --sort=name --mtime="@$epoch" --owner=0 --group=0 --numeric-owner --format=posix \
    --pax-option=exthdr.name=%d/PaxHeaders/%f,delete=atime,delete=ctime \
    -C "$stage" -cf - "$binary" "$unit" LICENSE README.md | gzip -9n >"$out/$name.tar.gz"
  rm -rf -- "$stage"
}

cli_archive() {
  local os=$1 arch=$2 binary=dhow
  [[ $os == windows ]] && binary=dhow.exe
  local name="dhow_${version}_${os}_${arch}"
  local stage="$out/$name"
  mkdir -p -- "$stage"
  (
    cd -- "$root"
    CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -buildvcs=false \
      -ldflags "-s -w -buildid= -X $module/internal/buildinfo.version=$version -X $module/internal/buildinfo.commit=$commit" \
      -o "$stage/$binary" ./apps/cli/cmd/dhow
  )
  cp -- "$root/LICENSE" "$root/README.md" "$stage/"
  chmod 0755 "$stage/$binary"
  chmod 0644 "$stage/LICENSE" "$stage/README.md"
  tar --sort=name --mtime="@$epoch" --owner=0 --group=0 --numeric-owner --format=posix \
    --pax-option=exthdr.name=%d/PaxHeaders/%f,delete=atime,delete=ctime \
    -C "$stage" -cf - "$binary" LICENSE README.md | gzip -9n >"$out/$name.tar.gz"
  rm -rf -- "$stage"
}

rm -rf -- "$out"
mkdir -p -- "$out"
for component in $components; do
  for arch in $arches; do
    if [[ $component == cli ]]; then
      for os in $cli_oses; do
        cli_archive "$os" "$arch"
      done
    else
      archive "$component" "$arch"
    fi
  done
done
(cd -- "$out" && sha256sum -- *.tar.gz >SHA256SUMS)
printf 'Wrote %s\n' "$out"/*.tar.gz "$out/SHA256SUMS"
