// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package detail

import (
	"fmt"
	"strings"
)

func (c *Context) String() string {
	tokens := make([]string, 0, 4)

	if c.Iface != 0 {
		tokens = append(tokens, c.Iface.String())
	}

	if c.Client != "" {
		tokens = append(tokens, "client")
		tokens = append(tokens, c.Client)
	}

	if c.Call != "" {
		tokens = append(tokens, c.Call)
	}

	return strings.Join(tokens, " ")
}

func (p *Position) String() string {
	tokens := make([]string, 0, 4)

	c := p.Context.String()
	if c == "" {
		c = "background"
	}
	tokens = append(tokens, c)

	if p.ProgramId != "" {
		tokens = append(tokens, "program "+p.ProgramId)
	}

	var i string
	if p.InstanceId != "" {
		i = "instance " + p.InstanceId
	}
	if p.InstanceArg != 0 {
		i += fmt.Sprintf(" arg %d", p.InstanceArg)
	}
	if i != "" {
		tokens = append(tokens, i)
	}

	if p.Subsystem != "" {
		tokens = append(tokens, p.Subsystem)
	}

	return strings.Join(tokens, ": ")
}
