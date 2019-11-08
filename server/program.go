// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"bufio"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	goruntime "runtime"
	"sync"

	"github.com/tsavola/gate/build"
	"github.com/tsavola/gate/image"
	"github.com/tsavola/gate/internal/error/badmodule"
	"github.com/tsavola/gate/runtime/abi"
	"github.com/tsavola/gate/server/event"
	"github.com/tsavola/gate/server/internal/error/failrequest"
	"github.com/tsavola/gate/snapshot"
	"github.com/tsavola/wag/compile"
	"github.com/tsavola/wag/object"
)

var (
	newHash      = sha512.New384
	hashEncoding = base64.RawURLEncoding
)

var errModuleSizeMismatch = &badmodule.Dual{
	Private: "content length does not match existing module size",
	Public:  "invalid module content",
}

func validateHashBytes(hash1 string, digest2 []byte) (err error) {
	digest1, err := hashEncoding.DecodeString(hash1)
	if err != nil {
		return
	}

	if subtle.ConstantTimeCompare(digest1, digest2) != 1 {
		err = failrequest.New(event.FailModuleHashMismatch, "module hash does not match content")
		return
	}

	return
}

func validateHashContent(hash1 string, r io.Reader) (err error) {
	hash2 := newHash()

	_, err = io.Copy(hash2, r)
	if err != nil {
		err = wrapContentError(err)
		return
	}

	return validateHashBytes(hash1, hash2.Sum(nil))
}

type program struct {
	key     string
	image   *image.Program
	buffers snapshot.Buffers

	storeLock sync.Mutex
	stored    bool

	// Protected by Server.lock:
	refCount int
}

// buildProgram returns an instance if instance policy is defined.  Entry name
// can be provided only when building an instance.
func buildProgram(storage image.Storage, progPolicy *ProgramPolicy, instPolicy *InstancePolicy, allegedHash string, content io.ReadCloser, contentSize int, entryName string,
) (prog *program, inst *image.Instance, err error) {
	defer func() {
		if content != nil {
			content.Close()
		}
	}()

	var codeMap object.CallMap

	b, err := build.New(storage, contentSize, progPolicy.MaxTextSize, &codeMap, instPolicy != nil)
	if err != nil {
		return
	}
	defer b.Close()

	hasher := newHash()
	reader := bufio.NewReader(io.TeeReader(io.TeeReader(content, b.Image.ModuleWriter()), hasher))

	b.InstallEarlySnapshotLoaders()

	b.Module, err = compile.LoadInitialSections(b.ModuleConfig(), reader)
	if err != nil {
		return
	}

	b.StackSize = progPolicy.MaxStackSize
	if instPolicy != nil {
		if b.StackSize > instPolicy.StackSize {
			b.StackSize = instPolicy.StackSize
		}
		err = b.SetMaxMemorySize(instPolicy.MaxMemorySize)
		if err != nil {
			return
		}
	}

	err = b.BindFunctions(entryName)
	if err != nil {
		return
	}

	err = compile.LoadCodeSection(b.CodeConfig(&codeMap), reader, b.Module, abi.Library())
	if err != nil {
		return
	}

	b.InstallSnapshotDataLoaders()

	err = compile.LoadCustomSections(&b.Config, reader)
	if err != nil {
		return
	}

	err = b.FinishImageText()
	if err != nil {
		return
	}

	b.InstallLateSnapshotLoaders()

	err = compile.LoadDataSection(b.DataConfig(), reader, b.Module)
	if err != nil {
		return
	}

	err = compile.LoadCustomSections(&b.Config, reader)
	if err != nil {
		return
	}

	err = content.Close()
	content = nil
	if err != nil {
		err = wrapContentError(err)
		return
	}

	actualHash := hasher.Sum(nil)

	if allegedHash != "" {
		err = validateHashBytes(allegedHash, actualHash)
		if err != nil {
			return
		}
	}

	progImage, err := b.FinishProgramImage()
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			progImage.Close()
		}
	}()

	if instPolicy != nil {
		inst, err = b.FinishInstanceImage(progImage)
		if err != nil {
			return
		}
	}

	prog = newProgram(hashEncoding.EncodeToString(actualHash), progImage, b.Buffers)
	return
}

func newProgram(key string, image *image.Program, buffers snapshot.Buffers) *program {
	prog := &program{
		key:      key,
		image:    image,
		buffers:  buffers,
		refCount: 1,
	}
	goruntime.SetFinalizer(prog, finalizeProgram)
	return prog
}

func finalizeProgram(prog *program) {
	if prog.refCount != 0 {
		if prog.refCount > 0 {
			log.Printf("closing unreachable program %q with reference count %d", prog.key, prog.refCount)
			prog.image.Close()
			prog.image = nil
		} else {
			log.Printf("unreachable program %q with reference count %d", prog.key, prog.refCount)
		}
	}
}

// ref must be called with Server.lock held.
func (prog *program) ref() *program {
	if prog.refCount <= 0 {
		panic(fmt.Sprintf("referencing program %q with reference count %d", prog.key, prog.refCount))
	}

	prog.refCount++
	return prog
}

// unref must be called with Server.lock held.
func (prog *program) unref() {
	if prog.refCount <= 0 {
		panic(fmt.Sprintf("unreferencing program %q with reference count %d", prog.key, prog.refCount))
	}

	prog.refCount--
	if prog.refCount == 0 {
		prog.image.Close()
		prog.image = nil
		goruntime.KeepAlive(prog)
	}
}

func (prog *program) ensureStorage() (err error) {
	prog.storeLock.Lock()
	defer prog.storeLock.Unlock()

	if prog.stored {
		return
	}

	err = prog.image.Store(prog.key)
	if err != nil {
		return
	}

	prog.stored = true
	return
}
