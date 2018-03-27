// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#define _GNU_SOURCE

#include "cgroup.h"

#include <errno.h>
#include <stdbool.h>
#include <stddef.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include <sys/types.h>
#include <unistd.h>

#include <systemd/sd-bus.h>

const char cgroup_backend[] = "systemd";

void init_cgroup(pid_t pid, const struct cgroup_config *config)
{
	char *scope;

	if (asprintf(&scope, "%s-%u.scope", config->title, pid) < 0) {
		perror("asprintf");
		exit(1);
	}

	const uid_t orig_uid = getuid();
	const bool have_root = (seteuid(0) == 0);
	const int root_errno = errno;

	int ret;
	sd_bus *bus = NULL;

	if (have_root) {
		ret = sd_bus_default_system(&bus);
		if (ret < 0) {
			fprintf(stderr, "sd_bus_default_system: %s\n", strerror(-ret));
			exit(1);
		}
	} else {
		ret = sd_bus_default(&bus);
		if (ret < 0) {
			fprintf(stderr, "set effective user id to root: %s\n", strerror(root_errno));
			fprintf(stderr, "sd_bus_default: %s\n", strerror(-ret));
			exit(1);
		}
	}

	sd_bus_message *reply = NULL;
	sd_bus_error error = SD_BUS_ERROR_NULL;

	if (strlen(config->parent) > 0) {
		ret = sd_bus_call_method(
			bus,
			"org.freedesktop.systemd1",
			"/org/freedesktop/systemd1",
			"org.freedesktop.systemd1.Manager",
			"StartTransientUnit",
			&error,
			&reply,
			"ssa(sv)a(sa(sv))",
			scope,                        // name
			"fail",                       // mode
			2,                            // properties
			"PIDs", "au", 1, pid,         //
			"Slice", "s", config->parent, //
			0);                           // aux
	} else {
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
			1,                    // properties
			"PIDs", "au", 1, pid, //
			0);                   // aux
	}
	if (ret < 0) {
		fprintf(stderr, "sd_bus_call_method: StartTransientUnit: %s\n", error.message);
		exit(1);
	}

	if (have_root && seteuid(orig_uid) != 0) {
		perror("set original effective user id");
		exit(1);
	}

	sd_bus_error_free(&error);
	sd_bus_message_unref(reply);
	sd_bus_unref(bus);

	free(scope);
}
