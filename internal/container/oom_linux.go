// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package container

import (
	"fmt"
	"os"
)

const maxOOMScoreAdj = "1000"

func writeOOMScoreAdj(pid int) error {
	return os.WriteFile(fmt.Sprintf("/proc/%d/oom_score_adj", pid), []byte(maxOOMScoreAdj), 0)
}
