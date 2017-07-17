#define _GNU_SOURCE

#include "cgroup.h"

#include <stddef.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include <sys/types.h>
#include <unistd.h>

#include <systemd/sd-bus.h>

#define DEFAULT_SLICE "system.slice"

const char cgroup_backend[] = "systemd";

void init_cgroup(pid_t pid, const struct cgroup_config *config)
{
	const char *slice = config->parent;
	if (strcmp(slice, "") == 0)
		slice = DEFAULT_SLICE;

	char *scope = NULL;
	sd_bus *bus = NULL;
        sd_bus_message *reply = NULL;
	sd_bus_error error = SD_BUS_ERROR_NULL;
	int ret;

	if (asprintf(&scope, "%s-%u.scope", config->title, pid) < 0) {
		perror("asprintf");
		exit(1);
	}

	uid_t orig_uid = getuid();
	if (seteuid(0) != 0) {
		perror("seteuid to root");
		exit(1);
	}

	ret = sd_bus_default_system(&bus);
	if (ret < 0) {
		fprintf(stderr, "sd_buf_default_system: %s\n", strerror(-ret));
		exit(1);
	}

	ret = sd_bus_call_method(
		bus,
		"org.freedesktop.systemd1",
		"/org/freedesktop/systemd1",
		"org.freedesktop.systemd1.Manager",
		"StartTransientUnit",
		&error,
		&reply,
		"ssa(sv)a(sa(sv))",
		scope,                // name
		"fail",               // mode
		2,                    // properties
		"PIDs", "au", 1, pid,
		"Slice", "s", slice,
		0);                   // aux
	if (ret < 0) {
		fprintf(stderr, "sd_bus_call_method StartTransientUnit: %s\n", error.message);
		exit(1);
	}

	if (seteuid(orig_uid) != 0) {
		perror("seteuid to original");
		exit(1);
	}

	sd_bus_error_free(&error);
	sd_bus_message_unref(reply);
	sd_bus_unref(bus);
	free(scope);
}
