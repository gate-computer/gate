// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>
#include <string.h>

#include <sys/types.h>
#include <unistd.h>

#define BUFFER_ALIGNMENT   64ULL // usual cache line size
#define BUFFER_MAX_ENTRIES 16    // pid buffer fits in single cache line

#define BUFFER_ALIGN_MASK  (BUFFER_ALIGNMENT - 1ULL)

#define BUFFER_STORAGE_SIZE(combined_size) \
	(BUFFER_ALIGN_MASK + (combined_size))

#define BUFFER_INITIALIZER(storage, offset) { \
	.ptr = (void *) ((((uintptr_t) (storage) + BUFFER_ALIGN_MASK) & ~BUFFER_ALIGN_MASK) + offset) \
}

struct buffer {
	void *ptr;
	unsigned int offset;
	unsigned int size;
};

static bool buffer_content(const struct buffer *b)
{
	return b->size > 0;
}

static bool buffer_space(const struct buffer *b, size_t entry_size)
{
	return b->size <= (BUFFER_MAX_ENTRIES - 1) * entry_size;
}

static int buffer_append(struct buffer *b, const void *entry, size_t entry_size)
{
	if (b->offset + b->size > (BUFFER_MAX_ENTRIES - 1) * entry_size) {
		if (b->offset < entry_size)
			return -1;

		memmove(b->ptr, b->ptr + b->offset, b->size);
		b->offset = 0;
	}

	memcpy(b->ptr + b->offset + b->size, entry, entry_size);
	b->size += entry_size;
	return 0;
}

static ssize_t buffer_send(struct buffer *b, int fd, int flags)
{
	ssize_t len = send(fd, b->ptr + b->offset, b->size, flags);
	if (len > 0) {
		b->size -= len;
		if (b->size == 0)
			b->offset = 0;
	}

	return len;
}

static bool buffer_space_pid(const struct buffer *b)
{
	return buffer_space(b, sizeof (pid_t));
}

static int buffer_append_pid(struct buffer *b, pid_t value)
{
	return buffer_append(b, &value, sizeof (value));
}

static bool buffer_remove_pid(struct buffer *b, pid_t value)
{
	pid_t *vector = b->ptr + b->offset;
	unsigned int length = b->size / sizeof (value);
	unsigned int i;

	for (i = 0; i < length; i++)
		if (vector[i] == value)
			goto found;

	return false;

found:
	if (i == 0) {
		b->size -= sizeof (value);
		if (b->size == 0)
			b->offset = 0;
		else
			b->offset += sizeof (value);
	} else {
		unsigned int head_size = i * sizeof (value);
		unsigned int tail_size = b->size - head_size - sizeof (value);
		void *p = b->ptr + b->offset + head_size;

		memmove(p, p + sizeof (value), tail_size);
		b->size -= sizeof (value);
	}

	return true;
}
