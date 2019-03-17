// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#ifndef GATE_RUNTIME_EXECUTOR_MAP_H
#define GATE_RUNTIME_EXECUTOR_MAP_H

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>
#include <string.h>

#include <pthread.h>

#include "executor.h"
#include "queue.h"

#define ALIGNED(n) __attribute__((aligned(n)))

#define CACHE_LINE_SIZE 64

#define ID_NUM 16384

typedef int16_t index_t;
typedef int16_t hash_t;

struct node {
	pid_t pid;
	union {
		int16_t id;    // When in pid bucket.
		index_t index; // When in free list.
	};
	index_t next;
};

struct pid_map {
	struct node arena[ID_NUM];
	index_t buckets[ID_NUM] ALIGNED(CACHE_LINE_SIZE);
	index_t free_list ALIGNED(CACHE_LINE_SIZE);
	pthread_mutex_t lock;
} ALIGNED(CACHE_LINE_SIZE);

static inline hash_t pid_hash(pid_t pid)
{
	return pid & (ID_NUM - 1);
}

static inline index_t pid_map_alloc_node_nolock(struct pid_map *m)
{
	if (m->free_list < 0)
		return -1;

	struct node *node = &m->arena[m->free_list];
	m->free_list = node->next;
	return node->index;
}

static inline void pid_map_free_node_nolock(struct pid_map *m, index_t i)
{
	struct node *node = &m->arena[i];
	node->pid = 0;
	node->index = i;
	node->next = m->free_list;
	m->free_list = i;
}

static inline int pid_map_remove_nolock(struct pid_map *m, pid_t pid, hash_t key)
{
	index_t previ = -1;

	for (index_t i = m->buckets[key]; i >= 0;) {
		struct node *node = &m->arena[i];
		if (node->pid == pid) {
			if (previ < 0)
				m->buckets[key] = node->next;
			else
				m->arena[previ].next = node->next;

			int16_t id = node->id;
			pid_map_free_node_nolock(m, i);

			return id;
		}

		previ = i;
		i = node->next;
	}

	return -1;
}

static inline void pid_map_init(struct pid_map *m)
{
	for (int i = 0; i < ID_NUM; i++) {
		m->arena[i].pid = 0;
		m->arena[i].index = i;
		m->arena[i].next = i + 1;
		m->buckets[i] = -1;
	}
	m->arena[ID_NUM - 1].next = -1;
	m->free_list = 0;
	pthread_mutex_init(&m->lock, NULL);
}

static inline int pid_map_replace(struct pid_map *m, pid_t pid, int16_t new_id, int16_t *old_id_out)
{
	int retval = -1;
	hash_t key = pid_hash(pid);

	pthread_mutex_lock(&m->lock);

	*old_id_out = pid_map_remove_nolock(m, pid, key);

	index_t i = pid_map_alloc_node_nolock(m);
	if (i >= 0) {
		struct node *node = &m->arena[i];
		node->pid = pid;
		node->id = new_id;
		node->next = m->buckets[key];
		m->buckets[key] = i;
		retval = 0;
	}

	pthread_mutex_unlock(&m->lock);

	return retval;
}

static inline unsigned int pid_map_remove_transform(struct pid_map *m, struct exec_status *queue, unsigned int begin, unsigned int end)
{
	pthread_mutex_lock(&m->lock);

	for (; begin != end; begin = queue_wrap(begin + 1)) {
		struct exec_status *status = &queue[begin];

		int16_t id = pid_map_remove_nolock(m, status->pid, pid_hash(status->pid));
		if (id < 0)
			break;

		status->pid = 0; // Clear the surroundings.
		status->id = id;
	}

	pthread_mutex_unlock(&m->lock);

	return begin;
}

#endif
