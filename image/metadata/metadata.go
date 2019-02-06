// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package metadata contains image metadata utilities.
package metadata

import (
	"github.com/tsavola/gate/entry"
	"github.com/tsavola/gate/image"
	"github.com/tsavola/wag/compile"
	"github.com/tsavola/wag/section"
)

func Make(mod *compile.Module, sectionMap *section.Map, funcAddrs []uint32) image.Metadata {
	return image.Metadata{
		MemorySizeLimit: mod.MemorySizeLimit(),
		GlobalTypes:     mod.GlobalTypes(),
		SectionRanges:   sectionMap.Sections[:],
		EntryAddrs:      entry.FuncAddrs(mod, funcAddrs),
	}
}
