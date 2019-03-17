// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include "map.h"

#include <cstdio>
#include <cstring>

#include <pthread.h>

#include <gtest/gtest.h>

struct MapTest: testing::Test {
	pid_map m;
	int16_t out;

	MapTest()
	{
		pid_map_init(&m);
	}

	int free_list_length()
	{
		int count = 0;
		for (auto i = m.free_list; i >= 0; i = m.arena[i].next)
			count++;
		return count;
	}

	exec_status status(pid_t pid, int32_t status)
	{
		exec_status s;
		memset(&s, 0, sizeof s);
		s.pid = pid;
		s.status = status;
		return s;
	}

	void dump_pid_buckets(bool all)
	{
		for (int i = 0; i < ID_NUM; i++) {
			auto j = m.buckets[i];
			if (!all && j == -1)
				continue;

			printf("[%04x]", i);

			for (const char *delim = " "; j >= 0; j = m.arena[j].next) {
				struct node *node = &m.arena[j];
				printf("%s%5d: %5d", delim, node->pid, node->id);
				delim = ", ";
			}

			printf("\n");
		}
	}
};

TEST_F(MapTest, SizeOf)
{
	EXPECT_EQ(sizeof m.arena, ID_NUM * 8);
	EXPECT_EQ(sizeof m.buckets, ID_NUM * 2);
}

TEST_F(MapTest, AlignOf)
{
	EXPECT_GE(alignof m, CACHE_LINE_SIZE);
	EXPECT_GE(alignof m.buckets, CACHE_LINE_SIZE);
	EXPECT_GE(alignof m.free_list, CACHE_LINE_SIZE);
}

TEST_F(MapTest, InitPidBuckets)
{
	for (int i = 0; i < ID_NUM; i++)
		ASSERT_EQ(m.buckets[i], -1);
}

TEST_F(MapTest, InitFreeList)
{
	int count = 0;
	for (auto i = m.free_list; i >= 0; i = m.arena[i].next) {
		ASSERT_EQ(m.arena[i].pid, 0);
		ASSERT_EQ(m.arena[i].index, i);
		count++;
	}
	ASSERT_EQ(count, ID_NUM);
}

TEST_F(MapTest, InitLock)
{
	ASSERT_EQ(pthread_mutex_lock(&m.lock), 0);
	ASSERT_NE(pthread_mutex_trylock(&m.lock), 0);
	ASSERT_EQ(pthread_mutex_unlock(&m.lock), 0);
}

TEST_F(MapTest, InsertFull)
{
	for (int i = 0; i < ID_NUM; i++)
		ASSERT_EQ(pid_map_replace(&m, i + 100000, i, &out), 0);

	ASSERT_EQ(free_list_length(), 0);
	ASSERT_EQ(pid_map_replace(&m, 900013, 13, &out), -1);
	ASSERT_EQ(free_list_length(), 0);

	for (int i = 0; i < ID_NUM; i++) {
		ASSERT_EQ(m.arena[i].id, i);
		ASSERT_EQ(m.arena[i].pid, i + 100000);
	}

	int count = 0;
	for (int i = 0; i < ID_NUM; i++) {
		for (auto j = m.buckets[i]; j >= 0; j = m.arena[j].next) {
			ASSERT_EQ(pid_hash(m.arena[j].pid), i);
			ASSERT_GE(m.arena[j].id, 0);
			ASSERT_LT(m.arena[j].id, ID_NUM);
			count++;
		}
	}
	ASSERT_EQ(count, ID_NUM);
}

TEST_F(MapTest, RemoveTransformSingle)
{
	for (int i = 0; i < ID_NUM; i++)
		ASSERT_EQ(pid_map_replace(&m, i + 5000, i, &out), 0);

	auto s = status(5100, 10);
	ASSERT_EQ(pid_map_remove_transform(&m, &s, 0, 1), 1);
	ASSERT_EQ(s.id, 100);
	ASSERT_EQ(s.status, 10);

	s = status(5123, 11);
	ASSERT_EQ(pid_map_remove_transform(&m, &s, 0, 1), 1);
	ASSERT_EQ(s.id, 123);
	ASSERT_EQ(s.status, 11);

	s = status(5990, 12);
	ASSERT_EQ(pid_map_remove_transform(&m, &s, 0, 1), 1);
	ASSERT_EQ(s.id, 990);
	ASSERT_EQ(s.status, 12);

	// TODO: check buckets

	int count = 0;
	for (auto i = m.free_list; i >= 0; i = m.arena[i].next) {
		ASSERT_EQ(m.arena[i].pid, 0);
		ASSERT_EQ(m.arena[i].index, i);
		count++;
	}
	ASSERT_EQ(count, 3);
}

TEST_F(MapTest, RemoveTransformWrap)
{
	for (int i = 0; i < ID_NUM; i++)
		ASSERT_EQ(pid_map_replace(&m, i + 15000, i, &out), 0);

	exec_status queue[QUEUE_BUFLEN];
	memset(queue, 0, sizeof queue);
	for (int i = 0; i < QUEUE_BUFLEN; i++)
		queue[i].pid = i + 15000;

	ASSERT_EQ(pid_map_remove_transform(&m, queue, QUEUE_BUFLEN - 56, 30), 30);
	for (int i = 0; i < 30; i++)
		ASSERT_EQ(queue[i].id, i);
	for (int i = 30; i < QUEUE_BUFLEN - 56; i++)
		ASSERT_EQ(queue[i].pid, i + 15000);
	for (int i = QUEUE_BUFLEN - 56; i < QUEUE_BUFLEN; i++)
		ASSERT_EQ(queue[i].id, i);
}

TEST_F(MapTest, RemoveTransformNonexistent)
{
	for (int i = 0; i < 100; i++)
		ASSERT_EQ(pid_map_replace(&m, i, i + 500, &out), 0);

	auto s = status(16000, 0);
	ASSERT_EQ(pid_map_remove_transform(&m, &s, 0, 1), 0);

	exec_status queue[QUEUE_BUFLEN];
	memset(queue, 0, sizeof queue);
	for (int i = 0; i < QUEUE_BUFLEN; i++)
		queue[i].pid = i;

	ASSERT_EQ(pid_map_remove_transform(&m, queue, 50, 150), 100);
	for (int i = 0; i < 50; i++)
		ASSERT_EQ(queue[i].pid, i);
	for (int i = 50; i < 100; i++)
		ASSERT_EQ(queue[i].id, i + 500);
	for (int i = 100; i < QUEUE_BUFLEN; i++)
		ASSERT_EQ(queue[i].pid, i);
}

TEST_F(MapTest, InsertRemoveAll)
{
	for (int i = 0; i < ID_NUM; i++)
		ASSERT_EQ(pid_map_replace(&m, i + 500, i, &out), 0);

	auto s = status(6000, 0);
	ASSERT_EQ(pid_map_remove_transform(&m, &s, 0, 1), 1);

	s = status(500, 0);
	ASSERT_EQ(pid_map_remove_transform(&m, &s, 0, 1), 1);

	s = status(6300, 0);
	ASSERT_EQ(pid_map_remove_transform(&m, &s, 0, 1), 1);

	s = status(ID_NUM + 499, 0);
	ASSERT_EQ(pid_map_remove_transform(&m, &s, 0, 1), 1);

	ASSERT_EQ(free_list_length(), 4);

	ASSERT_EQ(pid_map_replace(&m, 66666, 0, &out), 0);
	ASSERT_EQ(pid_map_replace(&m, 77777, ID_NUM - 1, &out), 0);
	ASSERT_EQ(pid_map_replace(&m, 88888, 6000 - 500, &out), 0);
	ASSERT_EQ(pid_map_replace(&m, 99999, 6300 - 500, &out), 0);

	ASSERT_EQ(free_list_length(), 0);

	for (int i = 0; i < 100000; i++) {
		s = status(i, 0);
		pid_map_remove_transform(&m, &s, 0, 1);
	}

	ASSERT_EQ(free_list_length(), ID_NUM);
}

TEST_F(MapTest, WorstCase)
{
	for (int i = 0; i < ID_NUM; i++)
		ASSERT_EQ(pid_map_replace(&m, i * ID_NUM, i, &out), 0);

	ASSERT_EQ(free_list_length(), 0);

	ASSERT_NE(m.buckets[0], -1);
	for (int i = 1; i < ID_NUM; i++)
		ASSERT_EQ(m.buckets[i], -1);

	for (int i = 0; i < ID_NUM; i += 4) {
		auto s = status(i * ID_NUM, 0);
		ASSERT_EQ(pid_map_remove_transform(&m, &s, 0, 1), 1);
	}

	ASSERT_EQ(free_list_length(), ID_NUM / 4);

	for (int i = ID_NUM - 1; i >= 0; i--) {
		if (i & 3) {
			auto s = status(i * ID_NUM, 0);
			ASSERT_EQ(pid_map_remove_transform(&m, &s, 0, 1), 1);
		}
	}

	ASSERT_EQ(free_list_length(), ID_NUM);
}
