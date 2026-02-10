// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package scope

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
)

func IsValid(s string) bool {
	if s == "" || len(s) > 255 {
		return false
	}

	for _, c := range []byte(s) {
		if c >= '0' && c <= '9' {
			continue
		}
		if c >= 'A' && c <= 'Z' {
			continue
		}
		if c >= 'a' && c <= 'z' {
			continue
		}
		switch c {
		case '-', '.', '/', ':', '_':
			continue
		}

		return false
	}

	if strings.HasPrefix(s, ":") || strings.HasSuffix(s, ":") {
		return false
	}
	if strings.Contains(s, "::") {
		return false
	}

	return true
}

var AliasRegexp = regexp.MustCompile(`.*\b([a-z0-9-._]+)$`)

func MatchAlias(s string) string {
	if m := AliasRegexp.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return ""
}

func ComputeAliases(scope []string) map[string]string {
	aliases := make(map[string]string)
	unalias := make(map[string]struct{})

	for _, s := range scope {
		if alias := MatchAlias(s); alias != "" {
			if _, found := aliases[alias]; found {
				unalias[alias] = struct{}{}
			} else {
				aliases[alias] = s
			}
		}
	}

	for _, s := range scope {
		alias := MatchAlias(s)
		if _, undo := unalias[alias]; undo {
			delete(aliases, alias)
		}
	}

	return aliases
}

var (
	names   []string
	aliases map[string]string
)

func Register(name string) {
	for _, s := range names {
		if s == name {
			panic(fmt.Sprintf("scope %q already registered", name))
		}
	}
	names = append(names, name)
	sort.Strings(names)
	aliases = ComputeAliases(names)
}

func Names() []string {
	return append([]string(nil), names...)
}

func Resolve(s string) string {
	if scope, found := aliases[s]; found {
		return scope
	}
	return s
}

type contextKey struct{}

func Context(ctx context.Context, scope []string) context.Context {
	if len(scope) == 0 && ctx.Value(contextKey{}) == nil {
		return ctx
	}

	scope = append([]string(nil), scope...)

	for i, s := range scope {
		if canonical, found := aliases[s]; found {
			scope[i] = canonical
		}
	}

	return context.WithValue(ctx, contextKey{}, scope)
}

func ContextContains(ctx context.Context, scope string) bool {
	x := ctx.Value(contextKey{})
	if x == nil {
		return false
	}

	return slices.Contains(x.([]string), scope)
}
