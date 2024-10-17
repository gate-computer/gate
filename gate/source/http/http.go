// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package http can download objects from HTTP server.
package http

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	. "import.name/type/context"
)

type Config struct {
	Addr string
	Do   func(*http.Request) (*http.Response, error)
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
	if c.config.Do == nil {
		c.config.Do = http.DefaultClient.Do
	}
	return c
}

// CanonicalURI implements gate/server.Source.CanonicalURI.
func (c *Client) CanonicalURI(uri string) (string, error) {
	s := uri[1:] // Skip over first slash.
	if i := strings.IndexByte(s, '/'); i > 0 {
		s = s[i+1:] // Skip over second slash.
		if s == "" {
			goto invalid
		}

		// Accept only printable ASCII.
		for _, c := range []byte(s) {
			if c < ' ' || c > '~' {
				goto invalid
			}
		}

		return uri, nil
	}

invalid:
	return "", fmt.Errorf("invalid HTTP source URI: %q", uri)
}

// OpenURI implements gate/server.Source.OpenURI.
func (c *Client) OpenURI(ctx Context, uri string, maxSize int) (io.ReadCloser, int64, error) {
	url := c.config.Addr + uri
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}

	req.Header.Set("Range", fmt.Sprintf("bytes=0-%d", maxSize-1))

	resp, err := c.config.Do(req.WithContext(ctx))
	if err != nil {
		return nil, 0, err
	}
	defer func() {
		if resp != nil {
			resp.Body.Close()
		}
	}()

	var length int64

	switch resp.StatusCode {
	case http.StatusOK:
		if resp.ContentLength < 0 {
			return nil, 0, errors.New("TODO")
		}
		if resp.ContentLength > int64(maxSize) {
			return nil, resp.ContentLength, nil
		}
		length = resp.ContentLength

	case http.StatusPartialContent:
		rangeLen, totalLen, err := parseContentRange(resp.Header.Get("Content-Range"))
		if err != nil {
			return nil, 0, err
		}
		if rangeLen != totalLen || totalLen > int64(maxSize) {
			return nil, totalLen, nil
		}
		if resp.ContentLength >= 0 && resp.ContentLength != totalLen {
			return nil, 0, errors.New("http: Content-Length does not match Content-Range")
		}
		length = totalLen

	case http.StatusNotFound:
		return nil, 0, nil

	default:
		return nil, 0, fmt.Errorf("%s: status %s", url, resp.Status)
	}

	body := resp.Body
	resp = nil
	return body, length, nil
}

func parseContentRange(headerValue string) (rangeLen, totalLen int64, err error) {
	var lastByte int64

	n, err := fmt.Sscanf(headerValue, "bytes 0-%d/%d", &lastByte, &totalLen)
	if n != 2 || lastByte < 0 || totalLen < 0 || lastByte >= totalLen {
		return 0, 0, fmt.Errorf("http: invalid Content-Range header: %q", headerValue)
	}

	rangeLen = lastByte + 1
	return
}
