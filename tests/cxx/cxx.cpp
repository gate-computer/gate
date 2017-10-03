// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include <vector>

#include <gate.h>

int main()
{
	std::vector<char> v;

	for (int i = 0; i < 8; i++)
		v.push_back('A' + i);

	v.push_back('\n');
	v.push_back('\0');

	gate_debug(&v.front());

	return 0;
}
