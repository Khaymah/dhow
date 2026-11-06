ARG REGISTRY=docker.io/library/registry:3@sha256:79c026787b4607fb4633d415966cf7c059c1f46f92f597c346eb23f818fe4a3d
ARG BASE=docker.io/library/docker:28-dind@sha256:2a232a42256f70d78e3cc5d2b5d6b3276710a0de0596c145f627ecfae90282ac

FROM ${REGISTRY} AS registry

FROM ${BASE}

RUN --mount=type=secret,id=ca,target=/etc/ssl/certs/ca-certificates.crt,required=false \
    apk add --no-cache \
        openssh-server=10.0_p1-r11 \
        openssh-server-common=10.0_p1-r11 \
        openssh-keygen=10.0_p1-r11 \
        openssh-sftp-server=10.0_p1-r11 \
    && adduser -D -u 1000 deploy \
    && sed -i 's/^deploy:!:/deploy:*:/' /etc/shadow \
    && addgroup deploy docker \
    && ln -s /usr/local/bin/docker /usr/bin/docker \
    && install -d -o deploy -g deploy -m 0755 /srv/app \
    && install -d -m 0755 /var/lib/registry \
    && rm -f /etc/ssh/ssh_host_*_key /etc/ssh/ssh_host_*_key.pub

COPY --from=registry /bin/registry /usr/local/bin/registry
COPY registry.yml /etc/registry.yml
COPY sshd_config /etc/ssh/sshd_config
COPY entrypoint.sh /entrypoint.sh

LABEL org.opencontainers.image.title="dhow-e2e-compose-host" \
      org.opencontainers.image.description="A throwaway Docker host (Docker in Docker) with sshd, the compose plugin and a registry on localhost:5000, for the deploy end to end tests only" \
      org.opencontainers.image.source="https://github.com/Khaymah/dhow" \
      org.opencontainers.image.licenses="AGPL-3.0-only"

EXPOSE 22 8080
ENTRYPOINT ["/entrypoint.sh"]
