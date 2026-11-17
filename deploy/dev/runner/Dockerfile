FROM golang:1.27.4-trixie@sha256:b54b2639de8c1bfe538ec5ffc45f29b1932235ac5f6d63c059853646677112d9

RUN apt-get update \
    && apt-get install --yes --no-install-recommends ca-certificates crun git podman socat uidmap \
    && rm -rf /var/lib/apt/lists/* \
    && dpkg --compare-versions "$(dpkg-query --show --showformat '${Version}' podman)" ge 5.4

RUN useradd --system --user-group --home-dir /var/lib/dhow-runner --shell /usr/sbin/nologin dhow-runner \
    && touch /etc/subuid /etc/subgid \
    && sed -i '/^dhow-runner:/d' /etc/subuid /etc/subgid \
    && printf 'dhow-runner:1300000:1048576\n' | tee -a /etc/subuid >>/etc/subgid \
    && install -d -m 0700 -o dhow-runner -g dhow-runner /var/lib/dhow-runner \
    && install -d -m 0755 /opt/dhow-dev \
    && install -d -m 0700 /var/lib/dhow-dev \
    && find / -xdev -type f -perm /6000 ! -path /usr/bin/newuidmap ! -path /usr/bin/newgidmap -exec chmod ug-s {} + \
    && chmod 0700 /usr/bin/mount /usr/bin/umount

COPY runner.sh runner-env.sh runner-register.sh /usr/local/lib/dhow-dev/
RUN chown -R root:root /usr/local/lib/dhow-dev \
    && chmod 0755 /usr/local/lib/dhow-dev \
    && chmod 0644 /usr/local/lib/dhow-dev/runner-env.sh \
    && chmod 0755 /usr/local/lib/dhow-dev/runner.sh /usr/local/lib/dhow-dev/runner-register.sh \
    && ln -s /usr/local/lib/dhow-dev/runner-register.sh /usr/local/bin/dhow-dev-register
