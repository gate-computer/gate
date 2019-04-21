// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package ipfs can download objects via IPFS API server.
package ipfs

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

const Source = "/ipfs"

type Config struct {
	Addr string
}

func (c Config) Configured() bool {
	return c.Addr != ""
}

type Client struct {
	Config
	HTTP http.Client
}

func New(config Config) *Client {
	return &Client{
		Config: config,
	}
}

func (c *Client) OpenURI(ctx context.Context, uri string, maxSize int,
) (length int64, content io.ReadCloser, err error) {
	query := url.Values{
		"arg":    []string{uri},
		"length": []string{strconv.Itoa(maxSize + 1)},
	}.Encode()

	req, err := http.NewRequest(http.MethodGet, c.Addr+"/api/v0/cat?"+query, nil)
	if err != nil {
		return
	}

	resp, err := c.HTTP.Do(req.WithContext(ctx))
	if err != nil {
		return
	}
	defer func() {
		if content == nil {
			resp.Body.Close()
		}
	}()

	switch resp.StatusCode {
	case http.StatusOK:
		length, err = strconv.ParseInt(resp.Header.Get("X-Content-Length"), 10, 64)
		if err != nil {
			err = fmt.Errorf("ipfs: cat: X-Content-Length: %v", err)
			return
		}

		if length <= int64(maxSize) {
			content = resp.Body
		}
		return

	case http.StatusNotFound:
		return

	default:
		err = fmt.Errorf("ipfs: cat: %s", resp.Status)
		return
	}
}
