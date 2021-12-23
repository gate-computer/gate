// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !gateexecdir
// +build !gateexecdir

package child

import (
	"bytes"
	"compress/gzip"
	"io"

	"gate.computer/gate/internal/container/common"
	"golang.org/x/sys/unix"
)

func setupBinaries() error {
	if err := memfdCreateDup(common.ExecutorFilename, decompress(executorEmbed), common.ExecutorFD, unix.O_CLOEXEC); err != nil {
		return err
	}

	if err := memfdCreateDup(common.LoaderFilename, decompress(loaderEmbed), common.LoaderFD, 0); err != nil {
		return err
	}

	return nil
}

func decompress(data []byte) []byte {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		panic(err)
	}

	data, err = io.ReadAll(r)
	if err != nil {
		panic(err)
	}

	if err := r.Close(); err != nil {
		panic(err)
	}

	return data
}
