// Copyright (c) 2025 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"errors"
	"fmt"
	"math"
	"sync"

	"gate.computer/gate/packet"
	"gate.computer/uapi/internal"
)

// Registration failure reasons.
var (
	ErrNameAlreadyRegistered = errors.New("service already registered")
	ErrTooManyServices       = errors.New("too many services")
)

var (
	regMu    sync.Mutex
	regNames = make(map[string]struct{})
)

// Service handle.
//
// If a stream is opened as a result of service registration or a call, the
// appropriate stream constructor must be called synchronously in the receptor
// function.  Care must be taken when using input buffering with streams which
// carry stream ids.
type Service struct {
	internal internal.Service
}

// MustRegister a service or panic.  The info receptor is invoked with info
// packets' content.
func MustRegister(name string, infoReceptor func([]byte)) *Service {
	s, err := Register(name, infoReceptor)
	if err != nil {
		panic(err)
	}
	return s
}

// Register a service.  The info receptor is invoked with info packets'
// content.
func Register(name string, infoReceptor func([]byte)) (*Service, error) {
	if len(name) > 255 {
		panic("service name is too long")
	}

	regMu.Lock()
	defer regMu.Unlock()

	internal.Init()

	if _, found := regNames[name]; found {
		return nil, fmt.Errorf("%w: %s", ErrNameAlreadyRegistered, name)
	}

	n := len(regNames)
	if n > math.MaxInt16 {
		return nil, ErrTooManyServices
	}
	s := &Service{internal.Service{
		Name:         name,
		Code:         packet.Code(n),
		InfoReceptor: infoReceptor,
	}}
	internal.RegChan <- &s.internal
	regNames[name] = struct{}{}
	return s, nil
}

func (s *Service) Code() int16 {
	return int16(s.internal.Code)
}

// Call the service.  The receptor is invoked once with the reply packet
// content.
func (s *Service) Call(content []byte, receptor func([]byte)) {
	p := packet.MakeCall(s.internal.Code, len(content))
	copy(p.Content(), content)
	internal.SendPacket(p, receptor)
}
