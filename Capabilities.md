# Container capabilities

The program setting up the runtime container (gate-daemon, gate-runtime or
gate-server) may need
[capabilities](https://man7.org/linux/man-pages/man7/capabilities.7.html)
to create and/or configure the
[namespaces](https://man7.org/linux/man-pages/man7/namespaces.7.html).
Gate programs should never be run as root.

Possible configurations:

1. If the kernel configuration allows user namespace creation without
   privileges, it is possible to run Gate without any capabilities by setting
   the `runtime.container.namespace.singleuid` config option to `true`.  The
   container will be restricted to a single user and group id, mapped to the
   user creating the container.

2. If the kernel configuration allows user namespace creation without
   privileges, and the `newuidmap` and `newgidmap` tools
   ([uidmap](https://github.com/shadow-maint/shadow)) and the associated
   `/etc/subuid` and `/etc/subgid` files are installed, user namespace
   configuration may be delegated to them to avoid the singleuid restriction.
   The `runtime.container.namespace.newuidmap` and
   `runtime.container.namespace.newgidmap` config options need to be set to the
   respective program names.

3. If user namespace creation is a privileged operation, capabilities need to
   be provided to the Gate program.  That also enables multi-uid user namespace
   setup.  The `CAP_DAC_OVERRIDE`, `CAP_SETGID`, `CAP_SETUID` and
   `CAP_SYS_ADMIN` capabilities are needed.

   By default the `/etc/subuid` and `/etc/subgid` files are used to discover
   appropriate ids for the container configuration (but the uidmap programs are
   not needed).  It is also possible to specify ids via Gate config options.

4. Namespace creation can be disabled altogether.  It is highly unsafe.

gate-daemon and gate-server drop all capabilities after initialization.
(gate-runtime keeps them; its sole purpose is to create containers on request.)

