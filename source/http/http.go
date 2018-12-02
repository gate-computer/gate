// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package http can download objects from HTTP server.
package http

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
)

type Config struct {
	Addr string
}

func (c *Config) Configured() bool {
	return c.Addr != ""
}

type Client struct {
	Config
	HTTP http.Client
}

func New(config *Config) *Client {
	return &Client{
		Config: *config,
	}
}

func (c *Client) OpenURI(ctx context.Context, uri string, maxSize int,
) (length int64, content io.ReadCloser, err error) {
	req, err := http.NewRequest(http.MethodGet, c.Addr+uri, nil)
	if err != nil {
		return
	}

	req.Header.Set("Range", fmt.Sprintf("bytes=0-%d", maxSize-1))

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
		return handleOK(resp, maxSize)

	case http.StatusPartialContent:
		return handlePartialContent(resp, maxSize)

	case http.StatusNotFound:
		return

	default:
		err = fmt.Errorf("http source: %s", resp.Status)
		return
	}
}

func handleOK(resp *http.Response, maxSize int) (length int64, content io.ReadCloser, err error) {
	if resp.ContentLength < 0 {
		err = errors.New("TODO")
		return
	}

	if resp.ContentLength > int64(maxSize) {
		length = resp.ContentLength
		return
	}

	length = resp.ContentLength
	content = resp.Body
	return
}

func handlePartialContent(resp *http.Response, maxSize int,
) (length int64, content io.ReadCloser, err error) {
	rangeLength, completeLength, err := parseContentRange(resp.Header.Get("Content-Range"))
	if err != nil {
		return
	}

	if completeLength > int64(maxSize) {
		length = completeLength
		return
	}

	if resp.ContentLength >= 0 && resp.ContentLength != rangeLength {
		err = errors.New("TODO")
		return
	}

	length = rangeLength
	content = resp.Body
	return
}

func parseContentRange(headerValue string) (rangeLength, completeLength int64, err error) {
	var last int64

	n, err := fmt.Sscanf(headerValue, "bytes 0-%d/%d", &last, &completeLength)
	if n != 2 || last < 0 || completeLength < 0 || last >= completeLength {
		err = errors.New("TODO")
		return
	}

	rangeLength = last + 1
	return
}
