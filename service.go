// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/tsavola/gate/service"
)

const (
	ServiceName    = "gate.computer/localhost"
	ServiceVersion = "0"
)

type Config struct {
	Address string
}

var serviceConfig Config

func ServiceConfig() interface{} {
	return &serviceConfig
}

func InitServices(registry *service.Registry) (err error) {
	if serviceConfig.Address == "" {
		err = errors.New("localhost service: no address")
		return
	}

	u, err := url.Parse(serviceConfig.Address)
	if err != nil {
		return
	}
	if !u.IsAbs() {
		err = fmt.Errorf("localhost service: address is relative: %s", u)
		return
	}

	var l *localhost

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

		l = &localhost{
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

		l = &localhost{
			scheme: "http",
			host:   "localhost",
			client: client,
		}

	default:
		err = fmt.Errorf("localhost service: address has unsupported scheme: %s", u)
		return
	}

	registry.Register(l)
	return
}

type localhost struct {
	scheme string
	host   string
	client *http.Client
}

func (*localhost) ServiceName() string               { return ServiceName }
func (*localhost) ServiceVersion() string            { return ServiceVersion }
func (*localhost) Discoverable(context.Context) bool { return true }

func (l *localhost) CreateInstance(ctx context.Context, config service.InstanceConfig) service.Instance {
	return newInstance(l, config)
}

func (l *localhost) RestoreInstance(ctx context.Context, config service.InstanceConfig, snapshot []byte,
) (service.Instance, error) {
	inst := newInstance(l, config)
	if err := inst.restore(snapshot); err != nil {
		return nil, err
	}

	return inst, nil
}
