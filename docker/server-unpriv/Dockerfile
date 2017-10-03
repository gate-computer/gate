FROM gate-server

USER gate

EXPOSE 8888

ENTRYPOINT ["/usr/bin/gate-server", "-daemon-socket=/run/gate/containerd.sock", "-libdir=/usr/lib/gate", "-addr=:8888"]
