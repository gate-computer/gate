FROM gate-server

RUN apt-get --no-install-recommends -y install sudo && \
    apt-get clean

EXPOSE 8888

ENTRYPOINT ["/usr/bin/sudo", "-E", "-n", "-u", "gate", "/usr/bin/gate-server", "-libdir=/usr/lib/gate", "-addr=:8888"]
