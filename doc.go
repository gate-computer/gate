// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/*

Package gate contains documentation for its subpackages.


Errors

Some errors returned by the run, server, and webserver packages are wrappers
for an underlying error.  If direct access to the underlying error object is
needed, the wrapper can be opened using this interface definition:

	interface {
		Cause() error
	}

Use a type assertion to check if the error is a wrapper, and extract the cause.


*/
package gate
