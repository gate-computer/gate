// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include <gate/service.h>

#include <stddef.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

GATE_SERVICE_DECL
bool gate_service_registry_init(struct gate_service_registry *r, void *packet_buf, size_t packet_size)
{
	if (packet_size < gate_max_packet_size)
		return false;

	r->packet_buf = packet_buf;
	r->service_head = NULL;
	r->service_tail = NULL;
	r->service_count = 0;
	r->service_table = NULL;
	r->request_size = sizeof (struct gate_service_name_packet);
	return true;
}

GATE_SERVICE_DECL
void gate_service_registry_deinit(struct gate_service_registry *r)
{
	free(r->service_table);
}

GATE_SERVICE_DECL
struct gate_service_registry *gate_service_registry_create()
{
	void *buf = malloc(gate_max_packet_size);
	if (buf == NULL)
		goto no_buf;

	struct gate_service_registry *r = malloc(sizeof (struct gate_service_registry));
	if (r == NULL)
		goto no_r;

	if (!gate_service_registry_init(r, buf, gate_max_packet_size))
		goto no_init;

	return r;

no_init:
	free(r);
no_r:
	free(buf);
no_buf:
	return NULL;
}

GATE_SERVICE_DECL
void gate_service_registry_destroy(struct gate_service_registry *r)
{
	void *buf = r->packet_buf;

	gate_service_registry_deinit(r);
	free(r);
	free(buf);
}

GATE_SERVICE_DECL
bool gate_register_service(struct gate_service_registry *registry, struct gate_service *s)
{
	if (registry->service_count == INT16_MAX)
		return false;

	size_t request_size = registry->request_size + strlen(s->name) + 1;
	if (request_size > gate_max_packet_size)
		return false;

	s->code = registry->service_count;
	s->next_service = NULL;

	if (registry->service_head == NULL)
		registry->service_head = s;
	else
		registry->service_tail->next_service = s;

	registry->service_tail = s;
	registry->service_count++;
	registry->request_size = request_size;
	return true;
}

static void update_services(struct gate_service_registry *r, const struct gate_service_info_packet *p)
{
	struct gate_service *s = r->service_head;

	for (unsigned i = 0; i < p->count; i++, s = s->next_service) {
		const struct gate_service_info *info = &p->infos[i];

		r->service_table[i] = s;

		if (s->flags != info->flags || s->version != info->version) {
			s->flags = info->flags;
			s->version = info->version;

			if (s->changed)
				s->changed(s);
		}
	}
}

GATE_SERVICE_DECL
bool gate_discover_services(struct gate_service_registry *registry)
{
	struct gate_service_name_packet *req = registry->packet_buf;
	memset(&req->header, 0, sizeof (req->header));
	req->header.size = registry->request_size;
	req->header.code = GATE_PACKET_CODE_SERVICES;
	req->count = registry->service_count;

	char *namebuf = req->names;
	for (struct gate_service *s = registry->service_head; s; s = s->next_service) {
		size_t size = strlen(s->name) + 1;
		memcpy(namebuf, s->name, size);
		namebuf += size;
	}

	gate_send_packet(&req->header);
	gate_recv_packet(registry->packet_buf, gate_max_packet_size, 0);

	const struct gate_service_info_packet *resp = registry->packet_buf;
	if (resp->header.code != GATE_PACKET_CODE_SERVICES) {
		gate_debug("unexpected packet code while expecting service discovery response\n");
		gate_exit(1);
	}

	registry->service_table = calloc(registry->service_count, sizeof (struct gate_service *));
	if (registry->service_table == NULL)
		return false;

	update_services(registry, registry->packet_buf);
	return true;
}

GATE_SERVICE_DECL
int gate_recv_for_services(struct gate_service_registry *r, unsigned flags)
{
	size_t size = gate_recv_packet(r->packet_buf, gate_max_packet_size, flags);
	if (size == 0)
		return -1;

	const struct gate_packet *header = r->packet_buf;
	if (header->code >= 0 && header->code < r->service_count) {
		struct gate_service *s = r->service_table[header->code];
		s->received(s, r->packet_buf, size);
	} else {
		switch (header->code) {
		case GATE_PACKET_CODE_NOTHING:
			break;

		case GATE_PACKET_CODE_SERVICES:
			update_services(r, r->packet_buf);
			break;

		default:
			gate_debug("unknown packet code\n");
			break;
		}
	}

	return (unsigned int) header->flags; // zero-extend
}
