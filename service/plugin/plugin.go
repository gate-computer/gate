// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"plugin"
	"reflect"
	"strings"

	"github.com/tsavola/gate/service"
)

// Plugin file names suffix.  Used by the List function to collect files.
const Suffix = ".so"

// Function names exported by plugins.
const (
	PluginConfigSymbol = "PluginConfig"
	InitServicesSymbol = "InitServices"
)

type Plugin struct {
	name string
	path string
}

type Plugins struct {
	Config map[string]interface{}

	plugins []Plugin
}

func List(libdir string) (result Plugins, err error) {
	if libdir == "" {
		return
	}

	err = filepath.Walk(libdir, func(filename string, info os.FileInfo, err error) error {
		if err == nil {
			if strings.HasSuffix(filename, Suffix) && !info.IsDir() {
				result.plugins = append(result.plugins, Plugin{
					name: filename[len(libdir)+1 : len(filename)-len(Suffix)],
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

	result.Config = make(map[string]interface{})

	for _, pl := range result.plugins {
		var x interface{}

		x, err = pl.pluginConfig()
		if err != nil {
			return
		}

		result.Config[pl.name] = x
	}

	return
}

func (pls Plugins) InitServices(config service.Config) (err error) {
	for _, pl := range pls.plugins {
		err = pl.initServices(config)
		if err != nil {
			return
		}
	}

	return
}

func (pl *Plugin) pluginConfig() (config interface{}, err error) {
	p, err := plugin.Open(pl.path)
	if err != nil {
		return
	}

	x, err := p.Lookup(PluginConfigSymbol)
	if err != nil {
		return
	}

	f, ok := x.(func() interface{})
	if !ok {
		err = fmt.Errorf("%s: %s is a %s; expected a %s", pl.path, PluginConfigSymbol, reflect.TypeOf(x), reflect.TypeOf(f))
		return
	}

	config = f()
	return
}

func (pl *Plugin) initServices(config service.Config) (err error) {
	p, err := plugin.Open(pl.path)
	if err != nil {
		return
	}

	x, err := p.Lookup(InitServicesSymbol)
	if err != nil {
		return
	}

	f, ok := x.(func(service.Config) error)
	if !ok {
		err = fmt.Errorf("%s: %s is a %s; expected a %s", pl.path, InitServicesSymbol, reflect.TypeOf(x), reflect.TypeOf(f))
		return
	}

	return f(config)
}
