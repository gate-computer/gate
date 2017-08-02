FROM gate-server

RUN apt-get --no-install-recommends -y install sudo && \
    apt-get clean

EXPOSE 8888

ENTRYPOINT ["/usr/bin/sudo", "-E", "-n", "-u", "gate", "/usr/bin/gate-server", "-common-gid=900", "-container-uid=902", "-container-gid=902", "-executor-uid=903", "-executor-gid=903", "-libdir=/usr/lib/gate", "-addr=:8888"]
