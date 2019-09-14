// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package packetio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"testing"
	"time"

	"github.com/tsavola/gate/packet"
)

func writeByteSequence(w io.Writer, flow <-chan packet.FlowBuf, base, length, blockSize int) <-chan error {
	done := make(chan error, 1)
	go func() {
		defer close(done)

		var quota int32

		for i := 0; i < length; {
			if quota == 0 {
				if flow != nil {
					p, ok := <-flow
					if !ok {
						return
					}

					if packet.Buf(p).Code() != testService.Code {
						done <- fmt.Errorf("%s", p)
						return
					}

					for j := 0; j < p.Num(); j++ {
						id, increment := p.Get(j)
						if id == testStreamID {
							quota += increment
						}
					}
				} else {
					quota = math.MaxInt32
				}
			}

			writeLen := blockSize
			if int(quota) < writeLen {
				writeLen = int(quota)
			}

			b := make([]byte, 0, writeLen)
			for j := 0; j < writeLen && i < length; j++ {
				b = append(b, byte(base+i))
				i++
			}
			if _, err := w.Write(b); err != nil {
				done <- err
				return
			}

			quota -= int32(writeLen)
		}
	}()
	return done
}

func readByteSequence(r io.Reader, base, length int) <-chan error {
	done := make(chan error, 1)
	go func() {
		defer close(done)

		buf := make([]byte, 16)
		received := 0

		for {
			n, err := r.Read(buf)
			if err != nil {
				done <- err
				return
			}

			for i := 0; i < n; i++ {
				if buf[i] != byte(base+received) {
					panic(buf[i])
				}
				received++
				if received > length {
					done <- errors.New("read too many bytes")
					return
				}
			}
		}
	}()
	return done
}

func receiveByteSequence(flow *Threshold, output <-chan packet.DataBuf, base, length int) <-chan error {
	done := make(chan error, 1)
	go func() {
		defer close(done)

		flow.Increase(10)
		time.Sleep(time.Millisecond) // Make reader append more to the same
		flow.Increase(5000)          // packet.
		received := 0

		for p := range output {
			if packet.Buf(p).Code() != testService.Code {
				done <- fmt.Errorf("%s", p)
				return
			}

			if p.ID() != testStreamID {
				panic(p)
			}

			if p.DataLen() == 0 {
				if received != length {
					done <- errors.New("received too few bytes")
				}
				return
			}

			for i := 0; i < p.DataLen(); i++ {
				if p.Data()[i] != byte(base+received) {
					panic(p.Data()[i])
				}
				received++
				if received > length {
					done <- errors.New("received too many bytes")
					return
				}
			}

			flow.Increase(int32(p.DataLen()))
		}

		done <- errors.New("output channel was closed")
	}()
	return done
}

func TestStream(t *testing.T) {
	s := NewStream(10000)
	testStream(t, &s.readFlow, &s.writeBuf, s)
}

func TestRestoreStream(t *testing.T) {
	s := NewStream(12345)
	err := s.Restore(StreamState{
		Write:   WriteState{Receiving: true},
		Sending: true,
	})
	if err != nil {
		t.Error(err)
	}
	testStream(t, &s.readFlow, &s.writeBuf, s)
}

func testStream(t *testing.T, readflow *Threshold, writebuf *Buffer, s *Stream) {
	output := make(chan packet.Buf)

	ir, iw := io.Pipe()
	or, ow := io.Pipe()

	flowed := make(chan packet.FlowBuf, 1000)
	received := make(chan packet.DataBuf, 1000)
	go func() {
		defer close(flowed)
		defer close(received)

		for p := range output {
			switch p.Domain() {
			case packet.DomainFlow:
				flowed <- packet.FlowBuf(p)

			case packet.DomainData:
				received <- packet.DataBuf(p)
			}
		}
	}()

	var suspended StreamState

	irdone := receiveByteSequence(readflow, received, 123, 10000)
	iwdone := writeByteSequence(iw, nil, 123, 10000, 550)
	ordone := readByteSequence(or, 456, 20000)
	swdone := writeByteSequence(writebuf, flowed, 456, 20000, 330)
	sxdone := make(chan error, 1)
	go func() {
		var err error
		suspended, err = s.Transfer(context.Background(), testService, testStreamID, ir, ow, output)
		sxdone <- err
	}()

	for irdone != nil || iwdone != nil || ordone != nil || swdone != nil || sxdone != nil {
		select {
		case err := <-irdone:
			irdone = nil
			if err != nil {
				t.Fatal(err)
			}
			readflow.Finish()

		case err := <-iwdone:
			iwdone = nil
			if err != nil {
				t.Fatal(err)
			}
			iw.Close()

		case err := <-ordone:
			ordone = nil
			if isFailure(err) {
				t.Fatal(err)
			}

		case err := <-swdone:
			swdone = nil
			if err != nil {
				t.Fatal(err)
			}
			writebuf.WriteEOF()
			writebuf.Finish()

		case err := <-sxdone:
			sxdone = nil
			if isFailure(err) {
				t.Fatal(err)
			}
			ow.Close()
		}
	}

	if !writebuf.EOF() {
		t.Error("no EOF")
	}
	if suspended.IsMeaningful() {
		t.Error(suspended)
	}
}

func TestStreamFinish(t *testing.T) {
	ctx := context.Background()

	output := make(chan packet.Buf)

	r, _ := io.Pipe()
	_, w := io.Pipe()

	s := NewStream(512)
	s.Finish()

	_, err := s.Transfer(ctx, testService, testStreamID, r, w, output)
	if err != nil {
		t.Error(err)
	}

	if s.EOF() {
		t.Error("EOF")
	}
}

func TestStreamCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // No defer.

	output := make(chan packet.Buf)

	r, _ := io.Pipe()
	_, w := io.Pipe()

	s := NewStream(512)

	suspended, err := s.Transfer(ctx, testService, testStreamID, r, w, output)
	if err != nil {
		t.Error(err)
	}

	if !suspended.IsMeaningful() {
		t.Error(suspended)
	}
	if !suspended.Write.Receiving {
		t.Error(suspended)
	}
	if !suspended.Sending {
		t.Error(suspended)
	}

	if s.EOF() {
		t.Error("EOF")
	}
}

func TestRestoreStreamState(t *testing.T) {
	ctx := context.Background()

	s := NewStream(10)
	err := s.Restore(StreamState{Write: WriteState{Buffers: [2][]byte{make([]byte, 20), nil}}})
	if err != errBufferOverflow {
		t.Error(err)
	}

	s = NewStream(512)
	err = s.Restore(StreamState{Write: WriteState{Subscribed: math.MaxInt32}})
	if err != nil {
		t.Error(err)
	}
	if !s.EOF() {
		t.Error("no EOF when not receiving")
	}

	s = NewStream(512)
	err = s.Restore(StreamState{Write: WriteState{Subscribed: math.MaxInt32}})
	if err != nil {
		t.Error(err)
	}
	suspended, err := s.Transfer(ctx, testService, testStreamID, nil, nil, nil)
	if !isFailure(err) {
		t.Error(err)
	}
	if suspended.IsMeaningful() {
		t.Error(suspended)
	}
}

func TestRestoreStreamCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // No defer.

	output := make(chan packet.Buf)

	r, _ := io.Pipe()
	_, w := io.Pipe()

	s := NewStream(512)

	err := s.Restore(StreamState{
		Write:   WriteState{Buffers: [2][]byte{make([]byte, 1), nil}},
		Read:    ReadState{Buffer: packet.MakeData(testService.Code, testStreamID, 0)},
		Sending: true,
	})
	if err != nil {
		t.Error(err)
	}

	suspended, err := s.Transfer(ctx, testService, testStreamID, r, w, output)
	if err != nil {
		t.Error(err)
	}

	if !suspended.IsMeaningful() {
		t.Error(suspended)
	}
	if suspended.Write.Receiving {
		t.Error(suspended)
	}
	if suspended.Read.isMeaningful() {
		t.Error(suspended)
	}
	if !suspended.Sending {
		t.Error(suspended)
	}

	if !s.EOF() {
		t.Error("no EOF")
	}
}

func TestStreamReadError(t *testing.T) {
	ctx := context.Background()

	output := make(chan packet.Buf, 2) // Write flow and EOF.

	r, rw := io.Pipe()
	rw.CloseWithError(io.ErrClosedPipe)

	s := NewStream(512)

	err := s.Restore(StreamState{
		Read:    ReadState{Subscribed: math.MaxInt32},
		Sending: true,
	})
	if err != nil {
		t.Error(err)
	}

	_, err = s.Transfer(ctx, testService, testStreamID, r, nil, output)
	if err != io.ErrClosedPipe {
		t.Error(err)
	}
}

func TestStreamWriteEOF(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // No defer.

	r, _ := io.Pipe()

	s := NewStream(512)

	_, err := s.Transfer(ctx, testService, testStreamID, r, nil, nil)
	if err != io.EOF {
		t.Error(err)
	}
}

func TestStreamWriteError(t *testing.T) {
	ctx := context.Background()

	r, rw := io.Pipe()
	rw.Close()

	wr, w := io.Pipe()
	wr.Close()

	s := NewStream(512)

	err := s.Restore(StreamState{
		Write: WriteState{
			Buffers:   [2][]byte{make([]byte, 1), nil},
			Receiving: true,
		},
	})
	if err != nil {
		t.Error(err)
	}

	s.Subscribe(1)

	_, err = s.Transfer(ctx, testService, testStreamID, r, w, nil)
	if err != io.ErrClosedPipe {
		t.Error(err)
	}
}
