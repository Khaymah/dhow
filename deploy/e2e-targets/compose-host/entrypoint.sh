#!/bin/sh
set -eu

ssh-keygen -q -t ed25519 -N '' -f /etc/ssh/ssh_host_ed25519_key
install -d -o deploy -g deploy -m 0700 /home/deploy/.ssh
install -o deploy -g deploy -m 0600 /keys/authorized_keys /home/deploy/.ssh/authorized_keys

/usr/sbin/sshd -e
registry serve /etc/registry.yml &

exec dockerd-entrypoint.sh dockerd --host=unix:///var/run/docker.sock
