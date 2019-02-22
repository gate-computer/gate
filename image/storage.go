// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"context"
	"io"
	"io/ioutil"
	"os"

	"github.com/tsavola/gate/image/manifest"
	internal "github.com/tsavola/gate/internal/executable"
	"github.com/tsavola/wag/object"
)

// Archive contains a persistent WebAssembly binary module and a precompiled
// program.
type Archive interface {
	Manifest() manifest.Archive         // TODO: error?
	ObjectMap() (object.CallMap, error) // TODO: combine with ArchiveLoader
	Load(context.Context) (ArchiveLoader, error)
	Close() error
}

type LocalArchive interface {
	Archive

	file() *internal.FileRef
}

type ArchiveLoader struct {
	// Reader for module contents.
	Module ReaderOption

	// Readers for executable contents.  When using Text.Stream, Stack.Stream
	// and Data.Stream, manifest.Executable.TextSize bytes must be read from
	// Text and manifest.Executable.StackSize bytes from Stack before reading
	// from Data, as they might be the same stream.
	Text  ReaderOption
	Stack ReaderOption
	Data  ReaderOption

	// Close method releases temporary resources.
	Close func() error
}

type Storage interface {
	Store(context.Context, manifest.Archive) (ArchiveStorer, error)
}

type LocalStorage interface {
	Storage

	newArchiveFile() (*os.File, error)

	// give is like the Storage.Store, but text, stack and data are supplied
	// via as a FileRef instead of through ArchiveStorer writers.  The FileRef
	// must have been created with newArchiveFile.
	give(moduleKey string, m manifest.Archive, f *internal.FileRef, objectMap object.CallMap,
	) (LocalArchive, error)
}

type ArchiveStorer struct {
	// Writers for archive contents.  They must be written in order.
	Module io.Writer // Write manifest.Archive.ModuleSize bytes.
	Text   io.Writer // Write manifest.Executable.TextSize bytes.
	Stack  io.Writer // Write manifest.Executable.StackSize bytes.
	Data   io.Writer // Write manifest.Executable.DataSize bytes.

	// Archive method for getting the complete archive after module, text,
	// stack and data have been written.  It must be called before Close.
	Archive func(key string) (Archive, error)

	// Close method releases temporary resources, and the incomplete archive
	// if Archive method was not called (due to error).
	Close func() error
}

// ReaderOption must provide Stream or Random.  If both are non-nil, only one
// will be read from.
type ReaderOption struct {
	// Option A
	Stream io.Reader

	// Option B
	RandomAccess io.ReaderAt
	Offset       int64 // Position to read at.
}

func (ropt ReaderOption) Reader() io.Reader {
	if ropt.Stream != nil {
		return ropt.Stream
	}
	return &randomAccessReader{ropt.RandomAccess, ropt.Offset}
}

// ReaderAt which must be read at ascending offsets.  ReadAt may panic if the
// offsets are descending or the ranges overlap.
func (ropt ReaderOption) ReaderAt() io.ReaderAt {
	if ropt.RandomAccess != nil {
		if ropt.Offset == 0 {
			return ropt.RandomAccess
		}
		return &randomAccessReaderAt{ropt.RandomAccess, ropt.Offset}
	}
	return &streamReaderAt{ropt.Stream, ropt.Offset}
}

func (ropt *ReaderOption) copyToFile(wfile *os.File, woff int64, length int) (err error) {
	if ropt.RandomAccess != nil {
		if r, ok := ropt.RandomAccess.(descriptorFile); ok {
			return copyFileRange(r.Fd(), &ropt.Offset, wfile.Fd(), &woff, length)
		}
	}

	if ropt.Stream != nil {
		if r, ok := ropt.Stream.(descriptorFile); ok {
			return copyFileRange(r.Fd(), nil, wfile.Fd(), &woff, length)
		}
	}

	w := &randomAccessWriter{wfile, woff}

	if ropt.Stream != nil {
		_, err = io.CopyN(w, ropt.Stream, int64(length))
		return
	}

	r := &randomAccessReader{ropt.RandomAccess, ropt.Offset}

	_, err = io.CopyN(w, r, int64(length))
	return
}

// skip forward in Stream if copyToFile would use it.
func (ropt *ReaderOption) skip(length int) (err error) {
	if length == 0 {
		return
	}

	if ropt.RandomAccess != nil {
		if _, ok := ropt.RandomAccess.(descriptorFile); ok {
			return
		}
	}

	if ropt.Stream != nil {
		_, err = io.CopyN(ioutil.Discard, ropt.Stream, int64(length))
		return
	}

	return
}

type streamReaderAt struct {
	stream io.Reader
	read   int64
}

func (r *streamReaderAt) ReadAt(b []byte, offset int64) (n int, err error) {
	if r.read != offset {
		if r.read > offset {
			panic("stream read at non-ascending offset")
		}

		var m int64

		m, err = io.CopyN(ioutil.Discard, r.stream, offset-r.read)
		r.read += m
		n += int(m)
		if err != nil {
			return
		}
	}

	m, err := r.stream.Read(b)
	r.read += int64(m)
	n += m
	return
}

type randomAccessReader struct {
	randomAccess io.ReaderAt
	offset       int64
}

func (r *randomAccessReader) Read(b []byte) (n int, err error) {
	n, err = r.randomAccess.ReadAt(b, r.offset)
	r.offset += int64(n)
	return
}

type randomAccessReaderAt struct {
	randomAccess io.ReaderAt
	base         int64
}

func (r randomAccessReaderAt) ReadAt(b []byte, offset int64) (n int, err error) {
	return r.randomAccess.ReadAt(b, r.base+offset)
}

type randomAccessWriter struct {
	randomAccess *os.File
	offset       int64
}

func (w *randomAccessWriter) Write(b []byte) (n int, err error) {
	n, err = w.randomAccess.WriteAt(b, w.offset)
	w.offset += int64(n)
	return
}
