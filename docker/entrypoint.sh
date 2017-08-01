#!/bin/sh

set -e

pipe_gid=$(id -g uucp)

exec sudo -E -n -u daemon -- /usr/bin/gate-server -libdir=/usr/lib/gate -boot-uid=900 -boot-gid=900 -exec-uid=901 -exec-gid=901 -pipe-gid=${pipe_gid} "$@"
