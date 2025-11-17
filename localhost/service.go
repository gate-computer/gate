// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package localhost

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"gate.computer/gate/service"

	. "import.name/type/context"
)

const (
	serviceName     = "gate.computer/localhost"
	serviceRevision = "0"
)

type Config struct {
	Addr string
}

func New(config *Config) (*Localhost, error) {
	if config.Addr == "" {
		return nil, errors.New("localhost service: no address")
	}

	u, err := url.Parse(config.Addr)
	if err != nil {
		return nil, err
	}
	if !u.IsAbs() {
		return nil, fmt.Errorf("localhost service: address is relative: %s", u)
	}

	switch u.Scheme {
	case "http", "https":
		if u.Hostname() == "" {
			return nil, fmt.Errorf("localhost service: HTTP address has no host: %s", u)
		}
		if u.Path != "" && u.Path != "/" {
			return nil, fmt.Errorf("localhost service: HTTP address with path is not supported: %s", u)
		}

		l := &Localhost{
			scheme: u.Scheme,
			host:   u.Host,
			client: http.DefaultClient,
		}
		return l, nil

	case "unix":
		if u.Host != "" {
			return nil, fmt.Errorf("localhost service: unix address with host is not supported: %s", u)
		}
		if u.Path == "" {
			return nil, fmt.Errorf("localhost service: unix address has no path: %s", u)
		}

		dialer := net.Dialer{
			Timeout:   30 * time.Second, // Same as http.DefaultTransport (Go 1.12).
			KeepAlive: 30 * time.Second, //
		}

		client := &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx Context, network, addr string) (net.Conn, error) {
					return dialer.DialContext(ctx, "unix", u.Path)
				},
				DisableCompression:    true,
				MaxIdleConns:          1,
				MaxIdleConnsPerHost:   1,
				IdleConnTimeout:       1,
				ExpectContinueTimeout: time.Second, // Same as http.DefaultTransport (Go 1.12).
			},
		}

		l := &Localhost{
			scheme: "http",
			host:   "localhost",
			client: client,
		}
		return l, nil

	default:
		return nil, fmt.Errorf("localhost service: address has unsupported scheme: %s", u)
	}
}

type Localhost struct {
	scheme string
	host   string
	client *http.Client
}

func (*Localhost) Properties() service.Properties {
	return service.Properties{
		Service: service.Service{
			Name:     serviceName,
			Revision: serviceRevision,
		},
	}
}

func (*Localhost) Discoverable(Context) bool {
	return true
}

func (l *Localhost) CreateInstance(ctx Context, config service.InstanceConfig, snapshot []byte) (service.Instance, error) {
	inst := newInstance(l, config)
	if err := inst.restore(snapshot); err != nil {
		return nil, err
	}
	return inst, nil
}
