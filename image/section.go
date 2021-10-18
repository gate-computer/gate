// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"gate.computer/gate/internal/manifest"
	"gate.computer/wag/section"
)

type SectionMap struct {
	section.Map

	Snapshot   section.ByteRange
	ExportWrap section.ByteRange
	Buffer     section.ByteRange
	Stack      section.ByteRange
}

func (mappings *SectionMap) manifestSections() (sections []*manifest.ByteRange) {
	sections = make([]*manifest.ByteRange, len(mappings.Sections))
	for i, mapping := range mappings.Sections {
		sections[i] = manifestByteRange(mapping)
	}
	return
}

func manifestByteRange(mapping section.ByteRange) *manifest.ByteRange {
	return &manifest.ByteRange{
		Start: mapping.Start,
		Size:  mapping.Size,
	}
}
