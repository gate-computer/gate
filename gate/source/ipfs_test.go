// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package source_test

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"gate.computer/gate/source"
	"gate.computer/gate/source/http"
	"gate.computer/gate/source/ipfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	. "import.name/testing/mustr"
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

func testIPFSKey(t *testing.T, src source.Source) {
	data, tooLong, err := testIPFS(t, src, testKey, 65536, 5*time.Second)
	require.NoError(t, err)
	assert.False(t, tooLong)
	assert.Equal(t, string(data), "hello worlds\n")
}

func testIPFSPath(t *testing.T, source source.Source) {
	data, tooLong, err := testIPFS(t, source, testPath, 65536, 5*time.Second)
	require.NoError(t, err)
	assert.False(t, tooLong)
	assert.True(t, strings.HasPrefix(string(data), "# IPFS Examples\n"))
}

func testIPFSTimeout(t *testing.T, source source.Source) {
	_, tooLong, err := testIPFS(t, source, testKey, 65536, 1)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.False(t, tooLong)
}

func testIPFSLength(t *testing.T, source source.Source) {
	_, tooLong, err := testIPFS(t, source, testKey, 5, 5*time.Second)
	require.NoError(t, err)
	assert.True(t, tooLong)
}

func testIPFS(t *testing.T, source source.Source, uri string, maxSize int, timeout time.Duration) (data []byte, tooLong bool, err error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	r, length, err := source.OpenURI(ctx, uri, maxSize)
	if err != nil {
		return
	}

	if r == nil {
		require.NotEqual(t, length, 0, "not found")
		require.Less(t, maxSize, 13, "failed without good reason")
		tooLong = true
		return
	}

	defer func() {
		assert.NoError(t, r.Close())
	}()

	assert.LessOrEqual(t, length, int64(maxSize))

	data = Must(t, R(io.ReadAll(r)))
	assert.Equal(t, int64(len(data)), length)
	return
}
