// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package plugin

import (
	"os"
	"path/filepath"
	"plugin"
	"strings"
)

// Plugin file names suffix.  Used by the OpenAll function to collect files.
const Suffix = ".so"

type Plugin struct {
	*plugin.Plugin
	Name string
	path string
}

func (p Plugin) String() string {
	return p.path
}

type Plugins struct {
	Plugins []Plugin
}

// OpenAll plugins found under libdir.
func OpenAll(libdir string) (result Plugins, err error) {
	if libdir == "" {
		return
	}

	err = filepath.Walk(libdir, func(filename string, info os.FileInfo, err error) error {
		if err == nil {
			if strings.HasSuffix(filename, Suffix) && !info.IsDir() {
				result.Plugins = append(result.Plugins, Plugin{
					Name: filename[len(libdir)+1 : len(filename)-len(Suffix)],
					path: filename,
				})
			}
		}
		return err
	})
	if err != nil && os.IsNotExist(err) {
		err = nil
	}
	if err != nil {
		return
	}

	for i := range result.Plugins {
		result.Plugins[i].Plugin, err = plugin.Open(result.Plugins[i].path)
		if err != nil {
			return
		}
	}

	return
}
