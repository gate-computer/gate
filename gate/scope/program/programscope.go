// Copyright (c) 2022 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package program

import (
	"context"
	"errors"
	"fmt"
	"sync"

	parent "gate.computer/gate/scope"
)

// Scope is dynamic program scope filter.  It is unrestricted by default.
type Scope struct {
	mu    sync.Mutex
	scope map[string]struct{} // nil means unrestricted.
}

// Restrict sets the scope to the intersection of the existing scope and the
// argument.
func (x *Scope) Restrict(scope []string) error {
	if len(scope) > 255 {
		return errors.New("scope is too large")
	}
	for _, s := range scope {
		if !parent.IsValid(s) {
			return fmt.Errorf("scope string is invalid: %q", s)
		}
	}

	x.mu.Lock()
	defer x.mu.Unlock()

	m := make(map[string]struct{})

	if x.scope == nil {
		for _, s := range scope {
			m[s] = struct{}{}
		}
	} else {
		for _, s := range scope {
			if _, found := x.scope[s]; found {
				m[s] = struct{}{}
			}
		}
	}

	x.scope = m
	return nil
}

// Contains returns true if the argument is encompassed in the current scope.
func (x *Scope) Contains(scope string) bool {
	x.mu.Lock()
	defer x.mu.Unlock()

	if x.scope == nil {
		return true
	}

	_, found := x.scope[scope]
	return found
}

// Scope returns a scope array if restricted, or nil if unrestricted.
func (x *Scope) Scope() (scope []string, restricted bool) {
	x.mu.Lock()
	defer x.mu.Unlock()

	if x.scope == nil {
		return nil, false
	}

	scope = make([]string, 0, len(x.scope))
	for s := range x.scope {
		scope = append(scope, s)
	}
	return scope, true
}

type contextKey struct{}

// ContextWithScope adds program scope.
func ContextWithScope(ctx context.Context) context.Context {
	return context.WithValue(ctx, contextKey{}, new(Scope))
}

// ContextScope returns the contextual program scope or nil.
func ContextScope(ctx context.Context) *Scope {
	x := ctx.Value(contextKey{})
	if x == nil {
		return nil
	}

	return x.(*Scope)
}

// ContextContains returns true if the argument is encompassed in general scope
// and current program scope.  If the context doesn't have a Scope, this
// function behaves like gate/scope.ContextContains.
func ContextContains(ctx context.Context, scope string) bool {
	if !parent.ContextContains(ctx, scope) {
		return false
	}

	x := ContextScope(ctx)
	if x == nil {
		return true
	}
	return x.Contains(scope)
}
