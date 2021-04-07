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
	Addr   string
	Client *http.Client
}

func (c *Config) Configured() bool {
	return c.Addr != ""
}

type Client struct {
	config Config
}

func New(config *Config) *Client {
	c := &Client{
		config: *config,
	}
	if c.config.Client == nil {
		c.config.Client = http.DefaultClient
	}
	return c
}

// OpenURI implements server.Source.OpenURI.
func (c *Client) OpenURI(ctx context.Context, uri string, maxSize int) (io.ReadCloser, int64, error) {
	query := url.Values{
		"arg":    []string{uri},
		"length": []string{strconv.Itoa(maxSize + 1)},
	}.Encode()

	req, err := http.NewRequest(http.MethodPost, c.config.Addr+"/api/v0/cat?"+query, nil)
	if err != nil {
		return nil, 0, err
	}

	resp, err := c.config.Client.Do(req.WithContext(ctx))
	if err != nil {
		return nil, 0, err
	}
	defer func() {
		if resp != nil {
			resp.Body.Close()
		}
	}()

	switch resp.StatusCode {
	case http.StatusOK:
		length, err := strconv.ParseInt(resp.Header.Get("X-Content-Length"), 10, 64)
		if err != nil {
			return nil, 0, fmt.Errorf("ipfs: X-Content-Length header: %w", err)
		}
		if length > int64(maxSize) {
			return nil, length, nil
		}

		body := resp.Body
		resp = nil
		return body, length, nil

	case http.StatusNotFound:
		return nil, 0, nil

	default:
		return nil, 0, fmt.Errorf("ipfs: status %s", resp.Status)
	}
}
