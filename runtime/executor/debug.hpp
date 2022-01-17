// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#pragma once

#include <cstdarg>
#include <cstdio>

namespace runtime::executor {

void debugf(char const* format, ...)
{
	va_list arg;
	va_start(arg, format);

#if 0
	std::vfprintf(stderr, format, arg);
#endif

	va_end(arg);
}

} // namespace runtime::executor
