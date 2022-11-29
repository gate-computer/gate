// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"context"
	"log"
)

// Extension declaration.
type Extension struct {
	Name   string // Can be overridden to avoid conflicts.
	Config any    // Pointer to a custom struct type, or nil.
	Init   func(context.Context, *Registry) error
}

func (e *Extension) zeroconf() bool {
	if e.Config == nil {
		return true
	}

	_, empty := e.Config.(*struct{})
	return empty
}

// Extensions that have been registered via Extend or otherwise.
var Extensions []*Extension

// Extend Gate with configurable services.  Name should be the extension
// package's identifier: if full the package name is example.net/foo, it should
// be foo.
func Extend(
	name string,
	config any,
	init func(context.Context, *Registry) error,
) *Extension {
	e := &Extension{name, config, init}
	Extensions = append(Extensions, e)
	return e
}

// Config for global services (including Extensions).  If there are multiple
// entries with the same identifier and non-empty config, they are excluded.
func Config() map[string]any {
	m := make(map[string]any, len(Extensions))
	var dupes []string

	for _, e := range Extensions {
		if !e.zeroconf() {
			if _, found := m[e.Name]; found {
				log.Printf("duplicate extension name: %s", e.Name)
				dupes = append(dupes, e.Name)
			} else {
				m[e.Name] = e.Config
			}
		}
	}

	for _, s := range dupes {
		delete(m, s)
	}

	return m
}

// Init global services (including Extensions).
func Init(ctx context.Context, r *Registry) error {
	for _, e := range Extensions {
		if err := e.Init(ctx, r); err != nil {
			return err
		}
	}
	return nil
}
