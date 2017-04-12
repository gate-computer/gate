#ifndef _GATE_SERVICE_H
#define _GATE_SERVICE_H

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#include "../gate.h"

#ifndef GATE_SERVICE_DECL
#define GATE_SERVICE_DECL
#endif

#ifdef __cplusplus
extern "C" {
#endif

struct gate_service {
	const char *name;
	struct gate_service *next;
	uint16_t code;
	int32_t version;

	void (*discovered)(struct gate_service *) GATE_NOEXCEPT;
	void (*received)(struct gate_service *, void *data, size_t size) GATE_NOEXCEPT;
};

struct gate_service_registry {
	char *packet_buf;
	struct gate_service *service_head;
	unsigned int service_count;
	struct gate_service **service_table;
	size_t request_size;
};

GATE_SERVICE_DECL bool gate_service_registry_init(struct gate_service_registry *) GATE_NOEXCEPT;
GATE_SERVICE_DECL void gate_service_registry_deinit(struct gate_service_registry *) GATE_NOEXCEPT;

GATE_SERVICE_DECL struct gate_service_registry *gate_service_registry_create(void) GATE_NOEXCEPT;
GATE_SERVICE_DECL void gate_service_registry_destroy(struct gate_service_registry *) GATE_NOEXCEPT;

GATE_SERVICE_DECL bool gate_register_service(struct gate_service_registry *, struct gate_service *) GATE_NOEXCEPT;
GATE_SERVICE_DECL bool gate_discover_services(struct gate_service_registry *) GATE_NOEXCEPT;
GATE_SERVICE_DECL int gate_recv_for_services(struct gate_service_registry *, unsigned int flags) GATE_NOEXCEPT;

#ifdef __cplusplus
}
#endif

#endif
