// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/*
Package gate contains general documentation for its subpackages.  See
https://gate.computer for information about the Gate project.

# Errors

Error strings may contain sensitive details.  Some errors returned by Gate
implement this interface:

	interface {
		PublicError() string
	}

The public error string is intended to be exposed to the client (if the API was
called via a server endpoint).  If there is no PublicError method or it returns
the empty string, it's an internal error with no public explanation.
*/
package gate
