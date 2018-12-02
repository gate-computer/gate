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
	subUIDFilename = "/etc/subuid"
	subGIDFilename = "/etc/subgid"
)

func formatID(id uint) string {
	return strconv.FormatUint(uint64(id), 10)
}

type subIDMap struct {
	filename string
	reserved []uint

	next uint64
	end  uint64
}

func (m *subIDMap) parse(username string) error {
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

func (m *subIDMap) getID() (id uint, err error) {
	for m.next < m.end && m.next <= 0xffffffff {
		id = uint(m.next)
		m.next++

		for _, reservedID := range m.reserved {
			if reservedID > 0 && id == reservedID {
				goto skip
			}
		}

		return

	skip:
	}

	err = fmt.Errorf("%s: not enough ids", m.filename)
	return
}

func Parse(contUID, contGID, execUID, execGID uint,
) (creds [4]string, err error) {
	if contUID == 0 || contGID == 0 || execUID == 0 || execGID == 0 {
		var u *user.User

		u, err = user.Current()
		if err != nil {
			return
		}

		if contUID == 0 || execUID == 0 {
			m := subIDMap{
				filename: subUIDFilename,
				reserved: []uint{uint(syscall.Getuid()), contUID, execUID},
			}

			err = m.parse(u.Username)
			if err != nil {
				return
			}

			if contUID == 0 {
				contUID, err = m.getID()
				if err != nil {
					return
				}
			}

			if execUID == 0 {
				execUID, err = m.getID()
				if err != nil {
					return
				}
			}
		}

		if contGID == 0 || execGID == 0 {
			m := subIDMap{
				filename: subGIDFilename,
				reserved: []uint{uint(syscall.Getgid()), contGID, execGID},
			}

			err = m.parse(u.Username)
			if err != nil {
				return
			}

			if contGID == 0 {
				contGID, err = m.getID()
				if err != nil {
					return
				}
			}

			if execGID == 0 {
				execGID, err = m.getID()
				if err != nil {
					return
				}
			}
		}
	}

	creds = [4]string{
		formatID(contUID),
		formatID(contGID),
		formatID(execUID),
		formatID(execGID),
	}
	return
}
