// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package container

const FallbackCgroupTitle = "gate-runtime"

type CgroupConfig struct {
	Parent string
	Title  string
}

func (c *CgroupConfig) title() string {
	if c.Title == "" {
		return FallbackCgroupTitle
	}
	return c.Title
}

func configureCgroup(pid int, config *CgroupConfig) error {
	// TODO
	return nil
}
