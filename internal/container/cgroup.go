// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package container

import (
	config "gate.computer/gate/runtime/container"
)

func configureCgroup(pid int, c *config.CgroupConfig) error {
	title := c.Title
	if title == "" {
		title = config.CgroupTitle
	}

	// TODO
	return nil
}
