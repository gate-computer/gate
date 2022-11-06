// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package source_test

import (
	"context"
	"io/ioutil"
	"os"
	"strings"
	"testing"
	"time"

	"gate.computer/gate/server"
	"gate.computer/gate/source/http"
	"gate.computer/gate/source/ipfs"
)

const (
	testKey  = "/ipfs/QmZ4tDuvesekSs4qM5ZBKpXiZGun7S2CYtEZRB3DYXkjGx"
	testPath = "/ipfs/QmTDMoVqvyBkNMRhzvukTDznntByUNDwyNdSfV8dZ3VKRC/readme.md"
)

func newIPFSAPI(t *testing.T) *ipfs.Client {
	t.Helper()

	addr := os.Getenv("GATE_TEST_IPFS_API")
	if addr == "" {
		t.Skip("GATE_TEST_IPFS_API not set")
	}

	return ipfs.New(&ipfs.Config{Addr: addr})
}

func newIPFSGateway(t *testing.T) *http.Client {
	t.Helper()

	addr := os.Getenv("GATE_TEST_IPFS_GW")
	if addr == "" {
		t.Skip("GATE_TEST_IPFS_GW not set")
	}

	return http.New(&http.Config{Addr: addr})
}

func TestIPFSAPIKey(t *testing.T)     { testIPFSKey(t, newIPFSAPI(t)) }
func TestIPFSAPIPath(t *testing.T)    { testIPFSPath(t, newIPFSAPI(t)) }
func TestIPFSAPITimeout(t *testing.T) { testIPFSTimeout(t, newIPFSAPI(t)) }
func TestIPFSAPILength(t *testing.T)  { testIPFSLength(t, newIPFSAPI(t)) }

func TestIPFSGatewayKey(t *testing.T)     { testIPFSKey(t, newIPFSGateway(t)) }
func TestIPFSGatewayPath(t *testing.T)    { testIPFSPath(t, newIPFSGateway(t)) }
func TestIPFSGatewayTimeout(t *testing.T) { testIPFSTimeout(t, newIPFSGateway(t)) }
func TestIPFSGatewayLength(t *testing.T)  { testIPFSLength(t, newIPFSGateway(t)) }

func testIPFSKey(t *testing.T, source server.Source) {
	data, tooLong, err := testIPFS(t, source, testKey, 65536, 5*time.Second)
	if err != nil {
		t.Error(err)
	}

	if tooLong {
		t.Error("too long")
	}

	if string(data) != "hello worlds\n" {
		t.Errorf("data: %q", data)
	}
}

func testIPFSPath(t *testing.T, source server.Source) {
	data, tooLong, err := testIPFS(t, source, testPath, 65536, 5*time.Second)
	if err != nil {
		t.Error(err)
	}

	if tooLong {
		t.Error("too long")
	}

	if !strings.HasPrefix(string(data), "# IPFS Examples\n") {
		t.Errorf("data: %q", data)
	}
}

func testIPFSTimeout(t *testing.T, source server.Source) {
	_, tooLong, err := testIPFS(t, source, testKey, 65536, 1)
	if err == nil || !strings.HasSuffix(err.Error(), "context deadline exceeded") {
		t.Error(err)
	}

	if tooLong {
		t.Error("too long")
	}
}

func testIPFSLength(t *testing.T, source server.Source) {
	_, tooLong, err := testIPFS(t, source, testKey, 5, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	if !tooLong {
		t.Error("not too long")
	}
}

func testIPFS(t *testing.T, source server.Source, uri string, maxSize int, timeout time.Duration) (data []byte, tooLong bool, err error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	r, length, err := source.OpenURI(ctx, uri, maxSize)
	if err != nil {
		return
	}

	if r == nil {
		if length == 0 {
			t.Fatal("not found")
		}

		if maxSize >= 13 {
			t.Error("failed without good reason")
		}

		tooLong = true
		return
	}

	defer func() {
		if err := r.Close(); err != nil {
			t.Error("Close:", err)
		}
	}()

	if length > int64(maxSize) {
		t.Error("length:", length)
	}

	data, err = ioutil.ReadAll(r)
	if err != nil {
		t.Fatal("ReadFile:", err)
	}

	if int64(len(data)) != length {
		t.Errorf("%d != %d", len(data), length)
	}

	return
}
