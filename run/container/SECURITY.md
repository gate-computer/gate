The container binary needs capabilities for:

  - Configuring namespaces.

  - Configuring cgroup via systemd.  Effective uid is temporarily set to root.

  - Setting supplementary groups in user namespace.

Things controlled by the user who can execute the container binary:

  - Specify which of the parent namespace's user and group ids are mapped to the
    container's user namespace.  The identities are used to (1) set up the
    mount namespace, and (2) for the executor process and its children.

  - Specify which of the parent namespace's group ids is mapped to the
    container's user namespace.  It is used as a supplementary group of the
    contained processes, which need it for opening files in /proc/self/fd/.

  - Specify a parent cgroup (and name) for the container's cgroup.

  - Supply the file descriptor used for interacting with the executor process
    inside the container.  It can be used to execute arbitrary code inside the
    fully initialized container.

Environmental factors:

  - The container binary needs CAP_DAC_OVERRIDE, CAP_SETGID, and CAP_SETUID
    capabilities.  It should be executable only by a single, trusted user.

  - The binaries executed inside the container are determined by the location
    of the container binary itself: it looks for the "executor" and "loader"
    files in the same directory where it is located.  The write permissions of
    the directory and the binaries should be limited.  (Note that executor and
    loader don't need capabilities, and should actually be executable by the
    identity used inside the container.)

  - Systemd and D-Bus.

