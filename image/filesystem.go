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

	internal "github.com/tsavola/gate/internal/executable"
)

const (
	fsSubdirModule          = "module"
	fsSubdirArchive         = "archive"
	fsSubdirTemp            = "tmp"
	fsTempPatternModule     = "module-*"
	fsTempPatternArchive    = "archive-*"
	fsTempPatternExecutable = "executable-*"
)

// Filesystem implements BackingStore and Storage.  It's optimized for
// filesystems which support reflinks.
type Filesystem struct {
	pageSize   int
	dirModule  string
	dirArchive string
	dirTemp    string
}

func NewFilesystem(path string, pageSize int) *Filesystem {
	if pageSize < internal.PageSize {
		pageSize = internal.PageSize
	}

	return &Filesystem{
		pageSize:   pageSize,
		dirModule:  pathlib.Join(path, fsSubdirModule),
		dirArchive: pathlib.Join(path, fsSubdirArchive),
		dirTemp:    pathlib.Join(path, fsSubdirTemp),
	}
}

func (fs *Filesystem) getPageSize() int {
	return fs.pageSize
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

func (*Filesystem) sealFile(*os.File) error {
	return nil
}

func (fs *Filesystem) CreateModule(ctx context.Context, size int) (store ModuleStore, err error) {
	f, err := ioutil.TempFile(fs.dirTemp, fsTempPatternModule)
	if err != nil {
		return
	}

	success := false

	store = ModuleStore{
		Writer: f,

		Module: func(key string) (m Module, err error) {
			filename := pathlib.Join(fs.dirModule, key)

			err = syscall.Rename(f.Name(), filename) // os.Rename makes extraneous lstat call.
			if err != nil {
				return
			}

			m = &fsModule{filename, size}
			success = true
			return
		},

		Close: func() error {
			if !success {
				os.Remove(f.Name())
			}
			return f.Close()
		},
	}

	return
}

func (fs *Filesystem) CreateArchive(ctx context.Context, manifest ArchiveManifest,
) (store ExecutableStore, err error) {
	f, err := ioutil.TempFile(fs.dirTemp, fsTempPatternArchive)
	if err != nil {
		return
	}

	success := false

	store = ExecutableStore{
		Text:          f,
		GlobalsMemory: f,

		Archive: func(key string) (ar Archive, err error) {
			filename := pathlib.Join(fs.dirArchive, key)

			err = syscall.Rename(f.Name(), filename) // os.Rename makes extraneous lstat call.
			if err != nil {
				return
			}

			ar = &fsArchive{
				file:     f,
				filename: filename,
				manifest: manifest,
			}
			success = true
			return
		},

		Close: func() (err error) {
			if !success {
				removeErr := os.Remove(f.Name())
				closeErr := f.Close()
				if closeErr != nil {
					err = closeErr
				} else {
					err = removeErr
				}
			}
			return
		},
	}

	return
}

func (fs *Filesystem) archive(key string, metadata Metadata, manifest *internal.Manifest, ref *internal.FileRef,
) (ar Archive, err error) {
	if manifest.StackSize != 0 {
		return
	}

	// The file cannot be used directly as it has been unlinked, and creating a
	// link requires a path.

	f, err := ioutil.TempFile(fs.dirTemp, fsTempPatternArchive)
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			f.Close()
			os.Remove(f.Name())
		}
	}()

	// Reflink the contents (if supported by filesystem).

	err = copyFileRange(ref.Fd(), new(int64), f.Fd(), new(int64), manifest.TextSize+manifest.GlobalsSize+manifest.MemorySize)
	if err != nil {
		return
	}

	filename := pathlib.Join(fs.dirArchive, key)

	err = syscall.Rename(f.Name(), filename) // os.Rename makes extraneous lstat call.
	if err != nil {
		return
	}

	ar = &fsArchive{
		file:     f,
		filename: filename,
		manifest: makeArchiveManifest(manifest, metadata),
	}
	return
}

type fsModule struct {
	filename string
	size     int
}

func (m *fsModule) Open(context.Context) (load ModuleLoad, err error) {
	f, err := os.Open(m.filename)
	if err != nil {
		return
	}

	load = ModuleLoad{
		Length:       int64(m.size),
		ReaderOption: ReaderOption{RandomAccess: f},
		Close:        f.Close,
	}
	return
}

func (m *fsModule) Close() error {
	return os.Remove(m.filename)
}

type fsArchive struct {
	file     *os.File
	filename string
	manifest ArchiveManifest
}

func (ar *fsArchive) Manifest() ArchiveManifest {
	return ar.manifest
}

func (ar *fsArchive) Open(context.Context) (load ExecutableLoad, err error) {
	load = ExecutableLoad{
		Text: ReaderOption{
			RandomAccess: ar.file,
			Offset:       0,
		},
		GlobalsMemory: ReaderOption{
			RandomAccess: ar.file,
			Offset:       int64(ar.manifest.TextSize),
		},
		Close: func() error { return nil },
	}
	return
}

func (ar *fsArchive) Close() error {
	removeErr := os.Remove(ar.filename)
	closeErr := ar.file.Close()
	if closeErr != nil {
		return closeErr
	}
	return removeErr
}
