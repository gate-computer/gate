FROM gate-server

USER gate

EXPOSE 8888

ENTRYPOINT ["/usr/bin/gate-server", "-daemon-socket=/run/gate/containerd.sock", "-container-uid=902", "-container-gid=902", "-executor-uid=903", "-executor-gid=903", "-libdir=/usr/lib/gate", "-addr=:8888"]
