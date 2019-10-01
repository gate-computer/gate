// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtimeinfo

const (
	MaxABIVersion = 0
	MinABIVersion = 0
)

type Info struct {
	MaxABIVersion int `json:"max_abi_version"`
	MinABIVersion int `json:"min_abi_version"`
}

func Get() Info {
	return Info{
		MaxABIVersion: MaxABIVersion,
		MinABIVersion: MinABIVersion,
	}
}
