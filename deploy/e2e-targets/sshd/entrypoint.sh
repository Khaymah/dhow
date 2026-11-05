#!/bin/sh
set -eu

ssh-keygen -q -t ed25519 -N '' -f /etc/ssh/ssh_host_ed25519_key
install -d -o deploy -g deploy -m 0700 /home/deploy/.ssh
install -o deploy -g deploy -m 0600 /keys/authorized_keys /home/deploy/.ssh/authorized_keys

httpd -f -p 8080 -h /srv/app -u deploy &

exec /usr/sbin/sshd -D -e
