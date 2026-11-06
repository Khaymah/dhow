FROM --platform=$BUILDPLATFORM golang:1.27.3-bookworm@sha256:d9b6a1beb9474adc2a1ce842573639b44b2a0dd49b6669f51d3821baf9ec37c9 AS build

ARG TARGETOS
ARG TARGETARCH

ENV GOTOOLCHAIN=local \
    CGO_ENABLED=0 \
    GOFLAGS=-mod=readonly

WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY apps/plugins/deploy-compose ./apps/plugins/deploy-compose
COPY internal ./internal
COPY packages/proto/gen/go ./packages/proto/gen/go
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags "-s -w -buildid=" \
      -o /out/dhow-deploy-compose ./apps/plugins/deploy-compose/cmd/dhow-deploy-compose

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab

ARG VERSION=dev
ARG COMMIT=unknown

LABEL org.opencontainers.image.title="dhow-deploy-compose" \
      org.opencontainers.image.description="The official Dhow plugin that deploys a Docker Compose project to one server or to a group of servers over SSH, with every image pinned by digest, a safe compose file policy, a wait for healthy services and an automatic rollback to the images of the previous release" \
      org.opencontainers.image.source="https://github.com/Khaymah/dhow" \
      org.opencontainers.image.licenses="AGPL-3.0-only" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}"

LABEL io.dhow.plugin.manifest='{"Version":1,"Entrypoint":["/dhow-deploy-compose"],"Inputs":{"target":{"Required":true},"artifact":{"Required":true,"Kind":"artifact"},"images":{},"wait_timeout":{"Default":"120s"},"pull_timeout":{"Default":"10m"},"keep_releases":{},"health_check_url":{},"health_check_status":{"Default":"200-399"},"health_check_timeout":{"Default":"5s"},"health_check_retries":{"Default":"10"},"health_check_interval":{"Default":"5s"},"health_check_stable":{"Default":"3"},"health_check_start_delay":{"Default":"0s"},"on_first_failure":{"Default":"keep"}},"Outputs":[],"Mutating":true}'

COPY --from=build /out/dhow-deploy-compose /dhow-deploy-compose

USER 65532:65532
ENTRYPOINT ["/dhow-deploy-compose"]
