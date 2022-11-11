// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package container

import (
	"bufio"
	"fmt"
	"os"
	"os/user"
	"strconv"
	"strings"

	config "gate.computer/gate/runtime/container"
)

type subIDMap struct {
	filename string
	reserved []int

	next int
	end  int
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
			base, err := strconv.ParseInt(tokens[1], 10, 32)
			if err != nil {
				return err
			}

			count, err := strconv.ParseInt(tokens[2], 10, 32)
			if err != nil {
				return err
			}

			m.next = int(base + 1) // Skip root uid/gid.
			m.end = int(base + count)
			return nil
		}

		if err != nil {
			return err
		}
	}
}

func (m *subIDMap) getID() (int, error) {
	for m.next < m.end && m.next <= 0xffffffff {
		id := m.next
		m.next++

		for _, reservedID := range m.reserved {
			if reservedID > 0 && id == reservedID {
				goto skip
			}
		}

		return id, nil

	skip:
	}

	return 0, fmt.Errorf("%s: not enough ids", m.filename)
}

// NamespaceCreds for user namespace.
type NamespaceCreds struct {
	Container config.Cred
	Executor  config.Cred
}

func ParseCreds(c *config.NamespaceConfig) (*NamespaceCreds, error) {
	var (
		container = c.Container
		executor  = c.Executor
	)

	if container.UID == 0 || container.GID == 0 || executor.UID == 0 || executor.GID == 0 {
		u, err := user.Current()
		if err != nil {
			return nil, err
		}

		if container.UID == 0 || executor.UID == 0 {
			m := subIDMap{
				filename: getSubuid(c),
				reserved: []int{os.Getuid(), container.UID, executor.UID},
			}

			if err := m.parse(u.Username); err != nil {
				return nil, err
			}

			if container.UID == 0 {
				container.UID, err = m.getID()
				if err != nil {
					return nil, err
				}
			}

			if executor.UID == 0 {
				executor.UID, err = m.getID()
				if err != nil {
					return nil, err
				}
			}
		}

		if container.GID == 0 || executor.GID == 0 {
			m := subIDMap{
				filename: getSubgid(c),
				reserved: []int{os.Getgid(), container.GID, executor.GID},
			}

			if err := m.parse(u.Username); err != nil {
				return nil, err
			}

			if container.GID == 0 {
				container.GID, err = m.getID()
				if err != nil {
					return nil, err
				}
			}

			if executor.GID == 0 {
				executor.GID, err = m.getID()
				if err != nil {
					return nil, err
				}
			}
		}
	}

	return &NamespaceCreds{container, executor}, nil
}
