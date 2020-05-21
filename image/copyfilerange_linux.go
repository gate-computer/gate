// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"fmt"
	"io"

	"gate.computer/gate/internal/file"
	"golang.org/x/sys/unix"
)

func copyFileRange(r *file.File, roff *int64, w *file.File, woff *int64, length int) (err error) {
	for {
		if length == 0 {
			return
		}

		var n int

		n, err = unix.CopyFileRange(int(r.Fd()), roff, int(w.Fd()), woff, length, 0)
		if err != nil {
			if err == unix.EXDEV {
				goto fallback
			}
			err = fmt.Errorf("copy_file_range: %v", err)
			return
		}

		length -= n
	}

fallback:
	n, err := io.Copy(fileRangeWriter{w, woff}, io.NewSectionReader(r, *roff, int64(length)))
	*roff += n
	return
}

type fileRangeWriter struct {
	f   *file.File
	off *int64
}

func (x fileRangeWriter) Write(b []byte) (n int, err error) {
	n, err = x.f.WriteAt(b, *x.off)
	*x.off += int64(n)
	return
}
