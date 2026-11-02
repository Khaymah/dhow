ARG BASE=docker.io/library/alpine:3.22@sha256:2e0b81d60597f449baa56dc25d355367869d2dca4cd57e866f1e2b52af3eb55f
FROM ${BASE}

RUN apk add --no-cache git openssh-client \
    && rm -rf /usr/share/git-core/templates /etc/ssh/ssh_config /etc/ssh/ssh_config.d /etc/gitconfig \
    && find / -xdev -type f -perm /6000 -exec chmod a-s {} + \
    && git --version \
    && ssh -V

LABEL org.opencontainers.image.title="dhow-git" \
      org.opencontainers.image.description="The checkout image of the Dhow runner: git and openssh-client only" \
      org.opencontainers.image.source="https://github.com/Khaymah/dhow" \
      org.opencontainers.image.licenses="AGPL-3.0-only"

WORKDIR /workspace
