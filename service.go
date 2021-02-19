// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package localhost

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"gate.computer/gate/service"
)

const (
	serviceName     = "gate.computer/localhost"
	serviceRevision = "0"
)

type Config struct {
	Address string
}

func New(config *Config) (l *Localhost, err error) {
	if config.Address == "" {
		err = errors.New("localhost service: no address")
		return
	}

	u, err := url.Parse(config.Address)
	if err != nil {
		return
	}
	if !u.IsAbs() {
		err = fmt.Errorf("localhost service: address is relative: %s", u)
		return
	}

	switch u.Scheme {
	case "http", "https":
		if u.Hostname() == "" {
			err = fmt.Errorf("localhost service: HTTP address has no host: %s", u)
			return
		}
		if u.Path != "" && u.Path != "/" {
			err = fmt.Errorf("localhost service: HTTP address with path is not supported: %s", u)
			return
		}

		l = &Localhost{
			scheme: u.Scheme,
			host:   u.Host,
			client: http.DefaultClient,
		}

	case "unix":
		if u.Host != "" {
			err = fmt.Errorf("localhost service: unix address with host is not supported: %s", u)
			return
		}
		if u.Path == "" {
			err = fmt.Errorf("localhost service: unix address has no path: %s", u)
			return
		}

		dialer := net.Dialer{
			Timeout:   30 * time.Second, // Same as http.DefaultTransport (Go 1.12).
			KeepAlive: 30 * time.Second, //
		}

		client := &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
					return dialer.DialContext(ctx, "unix", u.Path)
				},
				DisableCompression:    true,
				MaxIdleConns:          1,
				MaxIdleConnsPerHost:   1,
				IdleConnTimeout:       1,
				ExpectContinueTimeout: time.Second, // Same as http.DefaultTransport (Go 1.12).
			},
		}

		l = &Localhost{
			scheme: "http",
			host:   "localhost",
			client: client,
		}

	default:
		err = fmt.Errorf("localhost service: address has unsupported scheme: %s", u)
		return
	}

	return
}

type Localhost struct {
	scheme string
	host   string
	client *http.Client
}

func (*Localhost) Service() service.Service {
	return service.Service{
		Name:     serviceName,
		Revision: serviceRevision,
	}
}

func (*Localhost) Discoverable(context.Context) bool {
	return true
}

func (l *Localhost) CreateInstance(ctx context.Context, config service.InstanceConfig, snapshot []byte,
) (service.Instance, error) {
	inst := newInstance(l, config)
	if err := inst.restore(snapshot); err != nil {
		return nil, err
	}

	return inst, nil
}
