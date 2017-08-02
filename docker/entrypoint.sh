#!/bin/sh

set -e

common_gid=$(id -g uucp)

exec sudo -E -n -u daemon -- /usr/bin/gate-server -common-gid=${pipe_gid} -container-uid=900 -container-gid=900 -executor-uid=901 -executor-gid=901 -libdir=/usr/lib/gate "$@"
