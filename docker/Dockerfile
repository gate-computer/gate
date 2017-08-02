FROM debian:stretch

COPY lib/container-cgroup-none /usr/lib/gate/container
ADD lib/executor /usr/lib/gate/
ADD lib/loader /usr/lib/gate/
ADD lib/runtime.map /usr/lib/gate/

RUN apt-get update && \
    apt-get --no-install-recommends -y install libcap2-bin && \
    apt-get clean && \

    addgroup --system --gid=900 gate-common && \
    adduser --system --uid=901 --group --no-create-home gate && \
    adduser --system --uid=902 --group --no-create-home gate-container && \
    adduser --system --uid=903 --group --no-create-home gate-executor && \

    chmod -R go-w /usr/lib/gate && \
    chmod 0500 /usr/lib/gate/container && \
    chown gate:root /usr/lib/gate/container && \
    setcap cap_dac_override,cap_setgid,cap_setuid+ep /usr/lib/gate/container && \

    mkdir -m 0700 /run/gate && \
    chown gate:root /run/gate

VOLUME /run/gate
