// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cred

import (
	"bufio"
	"fmt"
	"os"
	"os/user"
	"strconv"
	"strings"
	"syscall"
)

const (
	subUidFilename = "/etc/subuid"
	subGidFilename = "/etc/subgid"
)

func formatId(id uint) string {
	return strconv.FormatUint(uint64(id), 10)
}

type subIdMap struct {
	filename string
	reserved []uint

	next uint64
	end  uint64
}

func (m *subIdMap) parse(username string) error {
	f, err := os.Open(m.filename)
	if err != nil {
		return err
	}
	defer f.Close()

	r := bufio.NewReader(f)

	for {
		line, err := r.ReadString('\n')

		if tokens := strings.Split(strings.TrimSpace(line), ":"); len(tokens) >= 3 && tokens[0] == username {
			base, err := strconv.ParseUint(tokens[1], 10, 32)
			if err != nil {
				return err
			}

			count, err := strconv.ParseUint(tokens[2], 10, 32)
			if err != nil {
				return err
			}

			m.next = base + 1 // Skip the "root" id
			m.end = base + count
			return nil
		}

		if err != nil {
			return err
		}
	}
}

func (m *subIdMap) getId() (id uint, err error) {
	for m.next < m.end && m.next <= 0xffffffff {
		id = uint(m.next)
		m.next++

		for _, reservedId := range m.reserved {
			if reservedId > 0 && id == reservedId {
				goto skip
			}
		}

		return

	skip:
	}

	err = fmt.Errorf("%s: not enough ids", m.filename)
	return
}

func Parse(contUid, contGid, execUid, execGid uint,
) (creds [4]string, err error) {
	if contUid == 0 || contGid == 0 || execUid == 0 || execGid == 0 {
		var u *user.User

		u, err = user.Current()
		if err != nil {
			return
		}

		if contUid == 0 || execUid == 0 {
			m := subIdMap{
				filename: subUidFilename,
				reserved: []uint{uint(syscall.Getuid()), contUid, execUid},
			}

			err = m.parse(u.Username)
			if err != nil {
				return
			}

			if contUid == 0 {
				contUid, err = m.getId()
				if err != nil {
					return
				}
			}

			if execUid == 0 {
				execUid, err = m.getId()
				if err != nil {
					return
				}
			}
		}

		if contGid == 0 || execGid == 0 {
			m := subIdMap{
				filename: subGidFilename,
				reserved: []uint{uint(syscall.Getgid()), contGid, execGid},
			}

			err = m.parse(u.Username)
			if err != nil {
				return
			}

			if contGid == 0 {
				contGid, err = m.getId()
				if err != nil {
					return
				}
			}

			if execGid == 0 {
				execGid, err = m.getId()
				if err != nil {
					return
				}
			}
		}
	}

	creds = [4]string{
		formatId(contUid),
		formatId(contGid),
		formatId(execUid),
		formatId(execGid),
	}
	return
}
