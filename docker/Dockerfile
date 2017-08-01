FROM debian:stretch

COPY bin/server /usr/bin/gate-server
COPY docker/entrypoint.sh /usr/bin/gate-entrypoint

COPY lib/container-cgroup-none /usr/lib/gate/container
ADD lib/executor /usr/lib/gate/
ADD lib/loader /usr/lib/gate/
ADD lib/runtime.map /usr/lib/gate/

RUN apt-get update && \
    apt-get --no-install-recommends -y install libcap2-bin sudo && \
    apt-get clean && \

    chmod +x /usr/bin/gate-entrypoint && \

    chmod -R go-w /usr/lib/gate && \
    chmod go-rwx /usr/lib/gate/container && \
    chown daemon:root /usr/lib/gate/container && \
    setcap cap_dac_override,cap_setgid,cap_setuid+ep /usr/lib/gate/container; \

    adduser daemon uucp

VOLUME /var/lib/gate-server-letsencrypt

EXPOSE 8888

ENTRYPOINT ["/usr/bin/gate-entrypoint"]
