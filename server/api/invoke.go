// Copyright (c) 2022 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

// InvokeOptions for instance creation or resumption.  Nil InvokeOptions
// pointer is equivalent to zero-value InvokeOptions.
type InvokeOptions struct {
	DebugLog string
}
