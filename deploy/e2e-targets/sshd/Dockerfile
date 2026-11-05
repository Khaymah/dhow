ARG BASE=docker.io/library/alpine:3.22@sha256:2e0b81d60597f449baa56dc25d355367869d2dca4cd57e866f1e2b52af3eb55f
FROM ${BASE}

RUN --mount=type=secret,id=ca,target=/etc/ssl/certs/ca-certificates.crt,required=false \
    apk add --no-cache \
        openssh-server=10.0_p1-r11 \
        openssh-server-common=10.0_p1-r11 \
        openssh-keygen=10.0_p1-r11 \
        openssh-sftp-server=10.0_p1-r11 \
        busybox-extras=1.37.0-r20 \
    && adduser -D -u 1000 deploy \
    && sed -i 's/^deploy:!:/deploy:*:/' /etc/shadow \
    && install -d -o deploy -g deploy -m 0755 /srv/app \
    && rm -f /etc/ssh/ssh_host_*_key /etc/ssh/ssh_host_*_key.pub

COPY sshd_config /etc/ssh/sshd_config
COPY entrypoint.sh /entrypoint.sh

LABEL org.opencontainers.image.title="dhow-e2e-sshd" \
      org.opencontainers.image.description="A throwaway sshd with an sftp subsystem and a static web server over /srv/app, for the deploy end to end tests only" \
      org.opencontainers.image.source="https://github.com/Khaymah/dhow" \
      org.opencontainers.image.licenses="AGPL-3.0-only"

EXPOSE 22 8080
ENTRYPOINT ["/entrypoint.sh"]
