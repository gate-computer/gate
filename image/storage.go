// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"context"
	"io"
	"io/ioutil"
	"os"

	internal "github.com/tsavola/gate/internal/executable"
)

// Module is a persistent WebAssembly binary module.
type Module interface {
	io.Closer
	Open(context.Context) (ModuleLoad, error)
}

// Archive is a persistent precompiled program.
type Archive interface {
	io.Closer
	Manifest() ArchiveManifest
	Open(context.Context) (ExecutableLoad, error)
}

type ArchiveManifest struct {
	TextSize    int
	GlobalsSize int
	MemorySize  int
	Metadata
}

func makeArchiveManifest(m *internal.Manifest, metadata Metadata) ArchiveManifest {
	return ArchiveManifest{
		TextSize:    m.TextSize,
		GlobalsSize: m.GlobalsSize,
		MemorySize:  m.MemorySize,
		Metadata:    metadata,
	}
}

type ModuleStorage interface {
	CreateModule(ctx context.Context, size int) (ModuleStore, error)
}

type ArchiveStorage interface {
	CreateArchive(context.Context, ArchiveManifest) (ExecutableStore, error)
}

type internalArchiveStorage interface {
	ArchiveStorage
	archive(key string, _ Metadata, _ *internal.Manifest, _ *internal.FileRef) (Archive, error)
}

type Storage interface {
	ModuleStorage
	ArchiveStorage
}

type ModuleLoad struct {
	Length int64

	// Reader for module contents.  Call Close after reading.
	ReaderOption

	// Close method releases temporary resources.
	Close func() error
}

type ModuleStore struct {
	// Writer for module contents.  Call Module and Close after writing.
	io.Writer

	// Module method for getting the complete module after its contents have
	// been written.  It must be called before Close.
	Module func(key string) (Module, error)

	// Close method releases temporary resources, and the incomplete module
	// if Module method was not called (due to error).
	Close func() error
}

type ExecutableLoad struct {
	// Readers for executable contents.  Call Close after reading.
	// When using Text.Stream and GlobalsMemory.Stream,
	// ArchiveManifest.TextSize bytes must be read from Text before reading
	// from GlobalsMemory, as they might be the same stream.
	Text          ReaderOption
	GlobalsMemory ReaderOption

	// Close method releases temporary resources.
	Close func() error
}

type ExecutableStore struct {
	// Writers for archive contents.  Call Archive and Close after writing.
	// ArchiveManifest.TextSize bytes must be written to Text before writing
	// GlobalsMemory, as they might point to the same stream.
	Text          io.Writer
	GlobalsMemory io.Writer

	// Archive method for getting the complete archive after text, globals and
	// memory contents have been written.  It must be called before Close.
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
