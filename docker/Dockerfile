FROM debian:stretch

COPY lib/container-cgroup-none /usr/lib/gate/gate-container
ADD lib/gate-executor /usr/lib/gate/
ADD lib/gate-loader /usr/lib/gate/
ADD lib/runtime.map /usr/lib/gate/

RUN apt-get update && \
    apt-get --no-install-recommends -y install libcap2-bin uidmap && \
    apt-get clean && \

    adduser --system --uid=901 --group --no-create-home gate && \

    chmod -R go-w /usr/lib/gate && \
    chmod 0500 /usr/lib/gate/gate-container && \
    chown gate:root /usr/lib/gate/gate-container && \
    setcap cap_sys_admin,cap_setuid+ep /usr/lib/gate/gate-container && \

    mkdir -m 0700 /run/gate && \
    chown gate:root /run/gate

VOLUME /run/gate
