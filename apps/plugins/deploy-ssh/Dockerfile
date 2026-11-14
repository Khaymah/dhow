FROM --platform=$BUILDPLATFORM golang:1.27.4-bookworm@sha256:4fa7c968ba93f1635670522dbb01a2c78150443e1385746924566079f36570ea AS build

ARG TARGETOS
ARG TARGETARCH

ENV GOTOOLCHAIN=local \
    CGO_ENABLED=0 \
    GOFLAGS=-mod=readonly

WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY apps/plugins/deploy-ssh ./apps/plugins/deploy-ssh
COPY internal ./internal
COPY packages/proto/gen/go ./packages/proto/gen/go
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags "-s -w -buildid=" \
      -o /out/dhow-deploy-ssh ./apps/plugins/deploy-ssh/cmd/dhow-deploy-ssh

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab

ARG VERSION=dev
ARG COMMIT=unknown

LABEL org.opencontainers.image.title="dhow-deploy-ssh" \
      org.opencontainers.image.description="The official Dhow plugin that deploys a release to one server or to a group of servers over SSH, in batches, with an atomic symlink switch, a health check per batch and an automatic rollback of every switched server" \
      org.opencontainers.image.source="https://github.com/Khaymah/dhow" \
      org.opencontainers.image.licenses="AGPL-3.0-only" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}"

LABEL io.dhow.plugin.manifest='{"Version":1,"Entrypoint":["/dhow-deploy-ssh"],"Inputs":{"target":{"Required":true},"artifact":{"Kind":"artifact"},"mode":{"Default":"deploy"},"release":{},"strategy":{"Default":"atomic-symlink"},"keep_releases":{},"health_check_url":{},"health_check_status":{"Default":"200-399"},"health_check_timeout":{"Default":"5s"},"health_check_retries":{"Default":"10"},"health_check_interval":{"Default":"5s"},"health_check_stable":{"Default":"3"},"health_check_start_delay":{"Default":"0s"},"on_first_failure":{"Default":"keep"}},"Outputs":[],"Mutating":true,"ExitReasons":{"1":"DEPLOY_FAILED","2":"PLUGIN_INPUT_INVALID","10":"DEPLOY_ROLLED_BACK","11":"DEPLOY_HEALTH_FAILED","12":"DEPLOY_ROLLBACK_FAILED","13":"HOST_KEY_MISMATCH","14":"ARTIFACT_UNSAFE","15":"TARGET_LOCKED","16":"CURRENT_NOT_A_SYMLINK","17":"TARGET_HOST_UNREACHABLE","18":"TARGET_STATE_CORRUPT","19":"TARGET_CHECK_FAILED"}}'

COPY --from=build /out/dhow-deploy-ssh /dhow-deploy-ssh

USER 65532:65532
ENTRYPOINT ["/dhow-deploy-ssh"]
