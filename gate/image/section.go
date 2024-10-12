// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	pb "gate.computer/internal/pb/image"
	"gate.computer/wag/section"
)

type SectionMap struct {
	section.Map

	Snapshot   section.ByteRange
	ExportWrap section.ByteRange
	Buffer     section.ByteRange
	Stack      section.ByteRange
}

func (mappings *SectionMap) manifestSections() (sections []*pb.ByteRange) {
	sections = make([]*pb.ByteRange, len(mappings.Sections))
	for i, mapping := range mappings.Sections {
		sections[i] = manifestByteRange(mapping)
	}
	return
}

func manifestByteRange(mapping section.ByteRange) *pb.ByteRange {
	return &pb.ByteRange{
		Start: mapping.Start,
		Size:  mapping.Size,
	}
}
