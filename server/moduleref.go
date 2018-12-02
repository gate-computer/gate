// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"github.com/tsavola/gate/internal/serverapi"
)

type ModuleRef = serverapi.ModuleRef
type ModuleRefs []ModuleRef

func (a ModuleRefs) Len() int           { return len(a) }
func (a ModuleRefs) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ModuleRefs) Less(i, j int) bool { return a[i].Key < a[j].Key }
