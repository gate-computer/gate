// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package shell

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os/exec"
	"os/user"

	"gate.computer/gate/packet"
	"gate.computer/gate/scope/program/system"
	"gate.computer/gate/service"
	"github.com/tsavola/threshold"
)

type errorCode int16

const (
	_ errorCode = iota
	errorScope
	errorQuota
	errorUser
	errorWorkDir
	errorExecutable
)

const (
	flagRunning uint8 = 1 << iota
	flagFlowing
)

const (
	maxDataSize = 8192 - packet.DataHeaderSize
	minDataSize = 100
)

type instance struct {
	service.InstanceBase

	code    packet.Code
	running chan struct{}
	flow    *threshold.Uint32
}

func newInstance(config service.InstanceConfig) *instance {
	return &instance{
		code: config.Code,
	}
}

func (inst *instance) restore(snapshot []byte) {
	if len(snapshot) == 0 {
		return
	}

	flags := snapshot[0]
	if flags&flagRunning != 0 {
		inst.running = make(chan struct{}, 1)
	}
	if flags&flagFlowing != 0 {
		inst.flow = threshold.NewUint32(0) // Value is irrelevant.
	}
}

func (inst *instance) Start(ctx context.Context, send chan<- packet.Thunk, abort func(error)) error {
	if inst.running != nil {
		go inst.io(ctx, nil, nil, send, inst.flow)
	}
	return nil
}

func (inst *instance) Handle(ctx context.Context, send chan<- packet.Thunk, p packet.Buf) (packet.Buf, error) {
	if !inst.refreshRunning() {
		return nil, nil
	}

	switch p.Domain() {
	case packet.DomainCall:
		return inst.handleCall(ctx, send, p)

	case packet.DomainFlow:
		return nil, inst.handleFlow(ctx, packet.FlowBuf(p))

	case packet.DomainData:
		return nil, errors.New("shell: received unexpected data packet")
	}

	return nil, nil
}

func (inst *instance) handleCall(ctx context.Context, send chan<- packet.Thunk, p packet.Buf) (packet.Buf, error) {
	errno := errorQuota
	streamID := int32(-1)

	if inst.running == nil && inst.flow == nil {
		var err error

		argv := []string{"/bin/sh", "-l", "-c", string(p.Content())}

		errno, err = inst.startProcess(ctx, send, argv)
		if err != nil {
			return nil, err
		}
		if errno == 0 {
			streamID = 0
		}
	}

	p = packet.Make(inst.code, packet.DomainCall, packet.HeaderSize+8)
	binary.LittleEndian.PutUint16(p.Content()[0:], uint16(errno))
	binary.LittleEndian.PutUint32(p.Content()[4:], uint32(streamID))
	return p, nil
}

func (inst *instance) handleFlow(ctx context.Context, p packet.FlowBuf) error {
	for i := 0; i < p.Num(); i++ {
		id, increment := p.Get(i)
		if inst.flow == nil || id != 0 {
			return errors.New("shell: received flow packet with nonexistent stream id")
		}

		switch {
		case increment > 0:
			inst.flow.Increment(uint32(increment))

		case increment == 0:
			inst.flow.Finish()
			inst.flow = nil
		}
	}

	return nil
}

func (inst *instance) Shutdown(ctx context.Context, suspend bool) ([]byte, error) {
	var flags uint8
	if inst.running != nil {
		if _, exited := <-inst.running; !exited {
			flags |= flagRunning
		}
	}
	if inst.flow != nil {
		flags |= flagFlowing
	}

	if flags != 0 {
		return []byte{flags}, nil
	}
	return nil, nil
}

func (inst *instance) refreshRunning() (ok bool) {
	if inst.running == nil {
		return true
	}

	select {
	case _, exited := <-inst.running:
		if !exited {
			// Shutting down.
			return false
		}

		inst.running = nil
		return true

	default:
		return true
	}
}

func (inst *instance) startProcess(ctx context.Context, send chan<- packet.Thunk, argv []string) (errorCode, error) {
	u, err := user.LookupId(system.ContextUserID(ctx))
	if err != nil {
		return errorUser, nil
	}

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = u.HomeDir

	var ok bool

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return 0, err
	}
	defer func() {
		if !ok {
			stdout.Close()
		}
	}()

	if err := cmd.Start(); err != nil {
		return 0, err
	}

	inst.running = make(chan struct{}, 1)
	inst.flow = threshold.NewUint32(0)
	go inst.io(ctx, cmd, stdout, send, inst.flow)

	ok = true
	return 0, nil
}

func (inst *instance) io(ctx context.Context, cmd *exec.Cmd, stdout io.ReadCloser, send chan<- packet.Thunk, flow *threshold.Uint32) {
	var exited bool
	defer func() {
		if exited {
			inst.running <- struct{}{}
		}
		close(inst.running)
	}()

	const streamID int32 = 0
	var result int32 = -1

	if cmd != nil {
		cmderr, ok := inst.ioCopy(ctx, cmd, stdout, send, flow)
		if !ok {
			return
		}

		result = -2 // TODO
		if e, ok := cmderr.(*exec.ExitError); ok {
			result = -3 // TODO
			if e.Exited() {
				result = int32(e.ExitCode())
			}
		}
	}

	makeEOF := func() (packet.Buf, error) {
		p := packet.MakeData(inst.code, streamID, 0)
		p.SetNote(result)
		return packet.Buf(p), nil
	}

	select {
	case send <- makeEOF:
		exited = true

	case <-ctx.Done():
	}
}

func (inst *instance) ioCopy(ctx context.Context, cmd *exec.Cmd, stdout io.ReadCloser, send chan<- packet.Thunk, flow *threshold.Uint32) (cmderr error, ok bool) {
	defer func() {
		cmderr = cmd.Wait()
	}()

	defer stdout.Close()

	const streamID int32 = 0

	var (
		subscribed uint32
		acquired   uint32
		buf        packet.DataBuf
	)

	for {
		for flow != nil {
			subscribed = flow.Value()
			if subscribed != acquired {
				break
			}

			select {
			case _, ok := <-flow.Chan():
				if !ok {
					flow = nil
				}

			case <-ctx.Done():
				return
			}
		}

		read := int64(subscribed - acquired)
		if read == 0 {
			cmd.Process.Kill()
			ok = true
			return
		}

		if read > int64(buf.DataLen()) {
			if buf.DataLen() < minDataSize {
				buf = packet.MakeData(inst.code, streamID, maxDataSize)
			}
			if read > int64(buf.DataLen()) {
				read = int64(buf.DataLen())
			}
		}

		n, err := stdout.Read(buf.Data()[:read])
		if err != nil {
			ok = true
			return
		}

		var p packet.Buf
		p, buf = buf.Cut(n)

		select {
		case send <- p.Thunk():
			acquired += uint32(n)
		case <-ctx.Done():
			return
		}
	}
}
