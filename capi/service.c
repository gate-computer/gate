// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include <gate/service.h>

#include <stddef.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

GATE_SERVICE_DECL
bool gate_service_registry_init(struct gate_service_registry *r)
{
	void *packet_buf = malloc(gate_max_packet_size);
	if (packet_buf == NULL)
		return false;

	r->packet_buf = packet_buf;
	r->service_head = NULL;
	r->service_count = 0;
	r->service_table = NULL;
	r->request_size = sizeof (struct gate_service_name_packet);
	return true;
}

GATE_SERVICE_DECL
void gate_service_registry_deinit(struct gate_service_registry *r)
{
	free(r->service_table);
	free(r->packet_buf);
}

GATE_SERVICE_DECL
struct gate_service_registry *gate_service_registry_create(void)
{
	struct gate_service_registry *r = malloc(sizeof (struct gate_service_registry));
	if (r == NULL)
		goto no_malloc;

	if (!gate_service_registry_init(r))
		goto no_init;

	return r;

no_init:
	free(r);
no_malloc:
	return NULL;
}

GATE_SERVICE_DECL
void gate_service_registry_destroy(struct gate_service_registry *r)
{
	gate_service_registry_deinit(r);
	free(r);
}

GATE_SERVICE_DECL
bool gate_register_service(struct gate_service_registry *registry, struct gate_service *service)
{
	size_t request_size = registry->request_size + strlen(service->name) + 1;
	if (request_size > gate_max_packet_size)
		return false;

	struct gate_service **nodeptr = &registry->service_head;
	while (*nodeptr)
		nodeptr = &(*nodeptr)->next;

	*nodeptr = service;
	registry->service_count++;
	registry->request_size = request_size;
	return true;
}

GATE_SERVICE_DECL
bool gate_discover_services(struct gate_service_registry *registry)
{
	struct gate_service_name_packet *req = registry->packet_buf;
	memset(&req->header, 0, sizeof (req->header));
	req->header.size = registry->request_size;
	req->count = registry->service_count;

	char *namebuf = req->names;
	for (struct gate_service *s = registry->service_head; s; s = s->next) {
		size_t size = strlen(s->name) + 1;
		memcpy(namebuf, s->name, size);
		namebuf += size;
	}

	gate_send_packet(&req->header);
	gate_recv_packet(registry->packet_buf, gate_max_packet_size, 0);

	const struct gate_service_info_packet *resp = registry->packet_buf;
	if (resp->header.code) {
		gate_debug("unexpected packet code while expecting service discovery response\n");
		gate_exit(1);
	}
	if (resp->count != registry->service_count) {
		gate_debug("unexpected number of entries in service discovery response\n");
		gate_exit(1);
	}

	uint16_t max_code = 0;

	int i = 0;
	for (struct gate_service *s = registry->service_head; s; s = s->next) {
		s->code = resp->infos[i].code;
		s->version = resp->infos[i].version;
		i++;

		if (s->code > max_code)
			max_code = s->code;
	}

	struct gate_service **table = calloc(max_code, sizeof (struct gate_service *));
	if (table == NULL)
		return false;

	registry->service_table = table;

	for (struct gate_service *s = registry->service_head; s; s = s->next)
		if (s->code)
			table[s->code - 1] = s;

	for (struct gate_service *s = registry->service_head; s; s = s->next)
		if (s->code && s->discovered)
			s->discovered(s);

	return true;
}

GATE_SERVICE_DECL
int gate_recv_for_services(struct gate_service_registry *registry, unsigned int flags)
{
	size_t size = gate_recv_packet(registry->packet_buf, gate_max_packet_size, flags);
	if (size == 0)
		return -1;

	const struct gate_packet *header = registry->packet_buf;
	if (header->code) {
		struct gate_service *s = registry->service_table[header->code - 1];
		s->received(s, registry->packet_buf, size);
	}

	return (unsigned int) header->flags; // zero-extend
}
