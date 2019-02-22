// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"context"
	"io/ioutil"
	"os"
	pathlib "path"
	"syscall"

	"github.com/tsavola/gate/image/manifest"
	internal "github.com/tsavola/gate/internal/executable"
	"github.com/tsavola/wag/object"
)

const (
	fsSubdirArchive         = "archive"
	fsSubdirTemp            = "tmp"
	fsTempPatternArchive    = "archive-*"
	fsTempPatternExecutable = "executable-*"
)

type FilesystemConfig struct {
	Path    string
	Reflink bool // Does the filesystem support reflinks?
}

// Filesystem implements BackingStore and LocalStorage.
//
// If supported, reflinks are used to avoid or defer copying.
type Filesystem struct {
	reflinkSupport bool
	dirArchive     string
	dirTemp        string
}

func NewFilesystem(config FilesystemConfig) (fs *Filesystem) {
	fs = &Filesystem{
		reflinkSupport: config.Reflink,
		dirArchive:     pathlib.Join(config.Path, fsSubdirArchive),
		dirTemp:        pathlib.Join(config.Path, fsSubdirTemp),
	}

	os.Mkdir(fs.dirArchive, 0700)
	os.Mkdir(fs.dirTemp, 0700)
	return
}

func (fs *Filesystem) newExecutableFile() (f *os.File, err error) {
	f, err = ioutil.TempFile(fs.dirTemp, fsTempPatternExecutable)
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			f.Close()
		}
	}()

	err = os.Remove(f.Name())
	if err != nil {
		return
	}

	return
}

func (fs *Filesystem) newArchiveFile() (f *os.File, err error) {
	return ioutil.TempFile(fs.dirTemp, fsTempPatternArchive)
}

func (fs *Filesystem) reflinkable(back interface{}) bool {
	if fs.reflinkSupport {
		if fs2, ok := back.(*Filesystem); ok {
			return fs == fs2
		}
	}
	return false
}

func (fs *Filesystem) give(key string, man manifest.Archive, ref *internal.FileRef, objectMap object.CallMap,
) (arc LocalArchive, err error) {
	filename := pathlib.Join(fs.dirArchive, key)

	err = syscall.Rename(ref.Name(), filename) // os.Rename makes extraneous lstat call.
	if err != nil {
		return
	}

	arc = &fsArchive{
		memArchive: memArchive{
			f:         ref.Ref(),
			man:       man,
			objectMap: objectMap,
		},
		fs:       fs,
		filename: filename,
	}
	return
}

func (fs *Filesystem) Store(ctx context.Context, man manifest.Archive,
) (storer ArchiveStorer, err error) {
	f, err := ioutil.TempFile(fs.dirTemp, fsTempPatternArchive)
	if err != nil {
		return
	}

	pending := &fsArchive{
		memArchive: memArchive{
			f:   internal.NewFileRef(f),
			man: man,
		},
		fs: fs,
	}

	storer = ArchiveStorer{
		Module: f,
		Text:   f,
		Stack:  f,
		Data:   f,

		Archive: func(key string) (arc Archive, err error) {
			filename := pathlib.Join(fs.dirArchive, key)

			err = syscall.Rename(pending.f.Name(), filename) // os.Rename makes extraneous lstat call.
			if err != nil {
				return
			}

			pending.filename = filename
			arc = pending
			pending = nil // Archived.
			return
		},

		Close: func() (err error) {
			if pending != nil {
				err = pending.Close()
				pending = nil
			}
			return
		},
	}
	return
}

type fsArchive struct {
	memArchive
	fs       *Filesystem
	filename string
}

func (arc *fsArchive) reflinkable(back interface{}) bool {
	return arc.fs.reflinkable(back)
}

func (arc *fsArchive) Close() error {
	removeErr := os.Remove(arc.filename)
	closeErr := arc.memArchive.Close()
	if closeErr != nil {
		return closeErr
	}
	return removeErr
}

func init() {
	var _ BackingStore = new(Filesystem)
	var _ LocalStorage = new(Filesystem)
	var _ Storage = new(Filesystem)
}
