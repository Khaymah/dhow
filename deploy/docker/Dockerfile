FROM --platform=$BUILDPLATFORM node:26-bookworm-slim@sha256:add5c17c14752c54d2f3d519ec46a3815684d41c33f198bcb09fb998f7dcbf68 AS web

ENV COREPACK_ENABLE_DOWNLOAD_PROMPT=0
RUN npm install --global corepack@0.36.0 && corepack enable && corepack prepare pnpm@10.28.0 --activate

WORKDIR /src/apps/web
COPY apps/web/package.json apps/web/pnpm-lock.yaml ./
RUN --mount=type=cache,target=/root/.local/share/pnpm/store \
    pnpm install --frozen-lockfile
COPY apps/web ./
RUN pnpm build

FROM --platform=$BUILDPLATFORM golang:1.27.3-bookworm@sha256:d9b6a1beb9474adc2a1ce842573639b44b2a0dd49b6669f51d3821baf9ec37c9 AS build

ARG VERSION=dev
ARG COMMIT=unknown
ARG DEPLOY_SSH_PINNED=
ARG TARGETOS
ARG TARGETARCH

ENV GOTOOLCHAIN=local \
    CGO_ENABLED=0 \
    GOFLAGS=-mod=readonly

WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY apps/server ./apps/server
COPY internal ./internal
COPY packages/proto/gen/go ./packages/proto/gen/go
COPY --from=web /src/apps/web/dist/ ./apps/server/internal/App/WebDist/
RUN if [ -n "$DEPLOY_SSH_PINNED" ]; then \
      printf '%s' "$DEPLOY_SSH_PINNED" | grep -Eq '^[a-z0-9.-]+(:[0-9]+)?/[a-z0-9._/-]+@sha256:[0-9a-f]{64}$' \
        || { echo "DEPLOY_SSH_PINNED must be registry/name@sha256:<64 hex>, got: $DEPLOY_SSH_PINNED" >&2; exit 1; }; \
    fi
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath \
      -ldflags "-s -w -buildid= \
        -X github.com/Khaymah/dhow/internal/buildinfo.version=${VERSION} \
        -X github.com/Khaymah/dhow/internal/buildinfo.commit=${COMMIT} \
        -X github.com/Khaymah/dhow/internal/buildinfo.deploySSHImage=${DEPLOY_SSH_PINNED}" \
      -o /out/dhow-server ./apps/server/cmd/dhow-server
RUN mkdir -p /out/state/blobs

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab

ARG VERSION=dev
ARG COMMIT=unknown

LABEL org.opencontainers.image.title="Dhow" \
      org.opencontainers.image.description="Self-hosted continuous deployment control plane" \
      org.opencontainers.image.source="https://github.com/Khaymah/dhow" \
      org.opencontainers.image.licenses="AGPL-3.0-only" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}"

COPY --from=build /out/dhow-server /dhow-server
COPY --from=build --chown=65532:65532 --chmod=0700 /out/state /var/lib/dhow

ENV DHOW_LISTEN_ADDR=0.0.0.0:8080
ENV DHOW_BLOB_DIR=/var/lib/dhow/blobs
ENV DHOW_ON_INCOMPATIBLE=wait
EXPOSE 8080
USER 65532:65532
HEALTHCHECK --interval=15s --timeout=5s --start-period=20s --retries=3 CMD ["/dhow-server", "healthcheck"]
ENTRYPOINT ["/dhow-server"]
