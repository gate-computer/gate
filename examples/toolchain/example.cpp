// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include <cstddef>
#include <cstdlib>
#include <cstring>

#include <gate.h>
#include <gate/service.h>

int main()
{
	struct gate_service_registry *no_services = gate_service_registry_create();
	if (no_services == NULL)
		gate_exit(1);

	if (!gate_discover_services(no_services))
		gate_exit(1);

	const char *str = "ok\n";

	auto buf = new char[strlen(str) + 1];
	if (buf == NULL)
		gate_exit(1);

	strcpy(buf, str);
	gate_debug(buf);

	gate_service_registry_destroy(no_services);
	return 0;
}