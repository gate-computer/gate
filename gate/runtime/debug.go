// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import (
	"bufio"
	"io"
	"os"
)

func copyDebug(outputDone chan<- struct{}, output io.Writer, r *os.File) {
	defer func() {
		if outputDone != nil {
			close(outputDone)
		}
		r.Close()
	}()

	w := bufio.NewWriter(output)
	b := make([]byte, 1)
	c := byte('\n')

	for {
		if _, err := r.Read(b); err != nil {
			if c != '\n' {
				if w.WriteByte('\n') == nil {
					w.Flush()
				}
			}
			return
		}

		c = b[0]

		if w.WriteByte(c) != nil {
			break
		}

		if c == '\n' {
			if w.Flush() != nil {
				break
			}
		}
	}

	close(outputDone)
	outputDone = nil

	io.Copy(io.Discard, r)
}
