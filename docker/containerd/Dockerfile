FROM gate

COPY bin/containerd /usr/bin/gate-containerd

USER gate

ENTRYPOINT ["/usr/bin/gate-containerd", "-listen=/run/gate/containerd.sock", "-libdir=/usr/lib/gate"]
