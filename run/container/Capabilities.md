# Container capabilities


Capabilities needed by the container binary:

  - Unless systemd support was disabled at build time, CAP_SETUID is needed for
    changing the effective user id to root for the duration of cgroup
    initialization.

  - If the kernel doesn't allow user namespace creation for non-root users,
    CAP_SYS_ADMIN is needed for that.  After the container has been configured,
    it drops all privileges and uses a non-root user id.


Privileged things controlled by users who can execute a capable container
binary:

  - Choose any cgroup as the parent for the container's cgroup.

  - Supply the file descriptor used for interacting with the executor process
    inside the container.  It can be used to spawn and kill processes inside
    the container, and execute arbitrary code in the processes.


Environmental factors:

  - The container binary should be executable only by a single, trusted user.

  - Configuration of the user namespace is delegated to /usr/bin/newuidmap and
    /usr/bin/newgidmap.

  - The binaries executed inside the container are determined by the location
    of the container binary itself: it looks for the "executor" and "loader"
    files in the same directory where it is located.  The write permissions of
    the directory and the binaries should be limited.  (Note that executor and
    loader don't need capabilities, and they need to have more relaxed read and
    execution permissions.)

  - Cgroup configuration needs to be done via systemd.  By default a container
    instance gets its own cgroup under system.slice, but that's it.

