// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import (
	"bufio"
	"io"
	"io/ioutil"
	"os"
)

const (
	debugSeparator = '\u001e'
	debugEllipsis  = '\u2026'
)

func copyDebug(outputDone chan<- struct{}, output io.Writer, input *os.File) {
	defer input.Close()

	w := bufio.NewWriter(output)
	r := bufio.NewReader(input)

	defer func() {
		if outputDone != nil {
			w.Flush()
			close(outputDone)
		}
	}()

	atSeparator := true // At start of stream.

reading:
	for {
		char, _, err := r.ReadRune()
		if err != nil {
			return
		}

		switch char {
		case '\n':
			if _, err := w.WriteRune(char); err != nil {
				break reading
			}
			if err := w.Flush(); err != nil {
				break reading
			}

			atSeparator = true

		case debugSeparator:
			if !atSeparator {
				if _, err := w.WriteRune(debugEllipsis); err != nil {
					break reading
				}

				if _, err := w.WriteRune('\n'); err != nil {
					break reading
				}

				atSeparator = true
			}

		default:
			if _, err := w.WriteRune(char); err != nil {
				break reading
			}

			atSeparator = false
		}
	}

	w.Flush()
	close(outputDone)
	outputDone = nil

	io.Copy(ioutil.Discard, r)
}
