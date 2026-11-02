ARG BASE=docker.io/library/alpine:3.22@sha256:2e0b81d60597f449baa56dc25d355367869d2dca4cd57e866f1e2b52af3eb55f
FROM ${BASE}

RUN apk add --no-cache \
        git=2.49.1-r0 \
        git-init-template=2.49.1-r0 \
        openssh-client-default=10.0_p1-r10 \
        openssh-client-common=10.0_p1-r10 \
        openssh-keygen=10.0_p1-r10 \
        brotli-libs=1.1.1-r0 \
        c-ares=1.34.9-r0 \
        libcurl=8.14.1-r4 \
        libedit=20250104.3.1-r1 \
        libexpat=2.10.0-r0 \
        libidn2=2.3.7-r0 \
        libncursesw=6.5_p20250503-r0 \
        libpsl=0.21.5-r3 \
        libunistring=1.3-r0 \
        ncurses-terminfo-base=6.5_p20250503-r0 \
        nghttp2-libs=1.69.0-r0 \
        pcre2=10.46-r1 \
        zstd-libs=1.5.7-r0 \
    && rm -rf /usr/share/git-core/templates /etc/ssh/ssh_config /etc/ssh/ssh_config.d /etc/gitconfig \
    && find / -xdev -type f -perm /6000 -exec chmod a-s {} + \
    && git --version \
    && ssh -V

LABEL org.opencontainers.image.title="dhow-git" \
      org.opencontainers.image.description="The checkout image of the Dhow runner: git and openssh-client only" \
      org.opencontainers.image.source="https://github.com/Khaymah/dhow" \
      org.opencontainers.image.licenses="AGPL-3.0-only"

WORKDIR /workspace
