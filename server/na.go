// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

// NotApplicable error is returned when a method cannot be used with a
// particular resource.
type NotApplicable interface {
	error
	NotApplicable()
}
