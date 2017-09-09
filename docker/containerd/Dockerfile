FROM gate

COPY bin/containerd /usr/bin/gate-containerd

USER gate

ENTRYPOINT ["/usr/bin/gate-containerd", "-listen=/run/gate/containerd.sock", "-container-uid=902", "-container-gid=902", "-executor-uid=903", "-executor-gid=903", "-libdir=/usr/lib/gate"]
