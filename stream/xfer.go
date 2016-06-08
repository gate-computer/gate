package stream

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"

	"golang.org/x/net/http2"
)

const (
	DefaultWindowSize = 65535
)

var (
	clientPreface = []byte(http2.ClientPreface)
)

// Streamer
type Streamer struct {
	Server   bool
	Creator  Creator
	Acceptor Acceptor
}

// NewStreamer
func NewStreamer(server bool, a Acceptor) *Streamer {
	return &Streamer{
		Server:   server,
		Creator:  make(chan chan *Stream),
		Acceptor: a,
	}
}

// NewStream
func (sr *Streamer) NewStream(ctx context.Context) (s *Stream, err error) {
	return sr.Creator.NewStream(ctx)
}

// Close
func (sr *Streamer) Close() {
	if sr.Creator != nil {
		close(sr.Creator)
		sr.Creator = nil
	}
}

// Do
func (sr *Streamer) Do(ctx context.Context, conn net.Conn) error {
	x := &xfer{
		authority:         conn.RemoteAddr().String(),
		streamMap:         make(map[uint32]*Stream),
		initialSendWindow: DefaultWindowSize,
		sendingStreams:    make(chan frameSender),
		headerDecoder:     newHeaderDecoder(),
		headerEncoder:     newHeaderEncoder(),
	}

	if sr.Server {
		x.nextLocalId = 2
	} else {
		x.nextLocalId = 1
	}

	if sr.Creator != nil {
		x.streamCreator = sr.Creator
	} else {
		c := make(chan chan *Stream)
		close(c)
		x.streamCreator = c
	}

	if sr.Acceptor != nil {
		x.acceptStream = func(s *Stream) {
			defer func() {
				if x := recover(); x != nil {
					if s != nil {
						logf("%s: %s acceptor: %v", x, s, x)
					} else {
						logf("%s: end acceptor: %v", x, x)
					}
				}
			}()

			sr.Acceptor.AcceptStream(sr, s)
		}
	} else {
		x.acceptStream = closeStream
	}

	return x.do(ctx, conn, sr.Server)
}

func closeStream(s *Stream) {
	if s != nil {
		s.Close()
	}
}

// xfer
type xfer struct {
	authority         string
	streamMap         map[uint32]*Stream
	nextLocalId       uint32
	lastRemoteId      uint32
	initialSendWindow int
	streamCreator     <-chan chan *Stream
	acceptStream      func(s *Stream)
	sendingStreams    chan frameSender
	headerDecoder     *headerDecoder
	headerEncoder     *headerEncoder
}

func (x *xfer) String() string {
	return fmt.Sprintf("conn %s", x.authority)
}

func (x *xfer) do(ctx context.Context, conn net.Conn, server bool) (err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	defer func() {
		if x.acceptStream != nil {
			x.acceptStream(nil)
		}
	}()

	var outstandingTask frameSender

	if server {
		buf := make([]byte, len(clientPreface))

		_, err = io.ReadFull(conn, buf)
		if err != nil {
			return
		}

		if !bytes.Equal(buf, clientPreface) {
			err = errors.New("invalid client preface")
			return
		}
	} else {
		_, err = conn.Write(clientPreface)
		if err != nil {
			return
		}
	}

	fr := http2.NewFramer(conn, conn)

	sendTasks := make(chan frameSender)
	defer func() {
		sendTasks <- nil // synchronize; makes sure doSend is not writing when we exit
		close(sendTasks)
	}()

	closedStreams := make(chan uint32, 1)

	sendFeedback := make(chan error, 1)

	go doSend(fr, sendTasks, closedStreams, sendFeedback, x)

	receiveFrames := make(chan receiveFrame, 1)

	receiveAgain := make(chan struct{}, 1)
	defer close(receiveAgain)

	go doReceive(fr, receiveFrames, receiveAgain, x)

	sendTasks <- frameSenderFunc(writeSettingsFrame)

	for x.streamCreator != nil || x.acceptStream != nil || len(x.streamMap) > 0 || outstandingTask != nil {
		var (
			activeStreamCreator  <-chan chan *Stream
			activeSendingStreams <-chan frameSender
			activeReceiveFrames  <-chan receiveFrame
			activeSendTasks      chan<- frameSender
		)

		if outstandingTask == nil {
			activeStreamCreator = x.streamCreator
			activeSendingStreams = x.sendingStreams
			activeReceiveFrames = receiveFrames
			logf("%s: waiting for stream or frame", x)
		} else {
			activeSendTasks = sendTasks
			logf("%s: waiting to send", x)
		}

		select {
		case output := <-activeStreamCreator:
			if output != nil {
				outstandingTask, err = x.createStream(ctx, output)
				if err != nil {
					return
				}
			} else {
				x.streamCreator = nil
				outstandingTask = x.goAwayTask(http2.ErrCodeNo)
				logf("%s: stream creator closed", x)
			}

		case sender := <-activeSendingStreams:
			outstandingTask = sender

		case f := <-activeReceiveFrames:
			if f.err != nil {
				if x.acceptStream != nil {
					err = f.err
				}
				return
			}

			outstandingTask, err = x.handle(ctx, f.frame, receiveAgain, fr)
			if err != nil {
				return
			}

		case activeSendTasks <- outstandingTask:
			outstandingTask = nil

		case e := <-sendFeedback:
			if x.acceptStream != nil {
				err = e
			}
			return

		case streamId := <-closedStreams:
			if s := x.streamMap[streamId]; s != nil && s.isEnded() {
				delete(x.streamMap, streamId)
				logf("%s: %s removed", x, s)
			}

		case <-ctx.Done():
			err = ctx.Err()
			logf("%s: %v", x, err)
			return
		}
	}

	logf("%s: finished", x)
	return
}

func (x *xfer) createStream(ctx context.Context, output chan<- *Stream) (task frameSender, err error) {
	defer close(output)

	streamId, err := x.newLocalId()
	if err != nil {
		return
	}

	s := newStream(ctx.Done(), streamId, x.sendingStreams, x.initialSendWindow, x)
	x.streamMap[streamId] = s
	logf("%s: local %s added", x, s)
	output <- s

	x.headerEncoder.Set(":scheme", "https")
	x.headerEncoder.Set(":authority", x.authority)
	x.headerEncoder.Set(":path", "/")
	x.headerEncoder.Set(":method", "POST")

	param := http2.HeadersFrameParam{
		StreamID:      streamId,
		BlockFragment: x.headerEncoder.Pop(),
		EndHeaders:    true,
	}

	task = frameSenderFunc(func(fr *http2.Framer, x fmt.Stringer) error {
		logf("%s: send: sending HEADERS of stream %d", x, param.StreamID)
		return fr.WriteHeaders(param)
	})

	return
}

func (x *xfer) handle(ctx context.Context, f http2.Frame, handled chan<- struct{}, fr *http2.Framer) (task frameSender, err error) {
	defer func() {
		handled <- struct{}{}
	}()

	logf("%s: handling %v", x, f)

	switch f := f.(type) {
	case *http2.DataFrame:
		return x.handleData(f)

	case *http2.HeadersFrame:
		return x.handleHeaders(ctx, f, fr)

	case *http2.RSTStreamFrame:
		return x.handleRSTStream(f)

	case *http2.SettingsFrame:
		return x.handleSettings(f)

	case *http2.PushPromiseFrame:
		return x.handlePushPromise(f)

	case *http2.PingFrame:
		return x.handlePing(f)

	case *http2.GoAwayFrame:
		return x.handleGoAway(f)

	case *http2.WindowUpdateFrame:
		return x.handleWindowUpdate(f)

	case *http2.ContinuationFrame:
		return x.handleContinuation(f)

	default:
		return
	}
}

func (x *xfer) handleData(f *http2.DataFrame) (task frameSender, err error) {
	s := x.streamMap[f.StreamID]
	if s == nil {
		if f.StreamID == 0 || f.StreamID >= x.nextLocalId-1 || f.StreamID > x.lastRemoteId {
			task = x.goAwayTask(http2.ErrCodeProtocol)
			err = errors.New("received data for unknown stream")
			logf("%s: %v", err, x)
		}
		return
	}

	if !s.headersDone {
		task = x.goAwayTask(http2.ErrCodeProtocol)
		err = errors.New("received data before end of headers")
		logf("%s: %v", err, x)
		return
	}

	err = s.receive(f.Data())
	if err != nil {
		task = x.goAwayTask(http2.ErrCodeProtocol)
		logf("%s: received stream data: %v", x, err)
		return
	}

	if f.StreamEnded() {
		s.end()
		if s.isWriteClosed() {
			delete(x.streamMap, f.StreamID)
			logf("%s: %s removed", x, s)
		}
	}

	return
}

func (x *xfer) handleHeaders(ctx context.Context, f *http2.HeadersFrame, fr *http2.Framer) (task frameSender, err error) {
	if f.StreamID == 0 {
		task = x.goAwayTask(http2.ErrCodeProtocol)
		err = errors.New("received headers without stream id")
		logf("%s: %v", x, err)
		return
	}

	if !f.HeadersEnded() {
		task = x.goAwayTask(http2.ErrCodeInternal)
		err = errors.New("headers continuation not implemented")
		logf("%s: %v", x, err)
		return
	}

	s := x.streamMap[f.StreamID]
	if s != nil {
		if s.headersDone {
			// TODO: trailer...
			task = x.goAwayTask(http2.ErrCodeInternal)
			err = errors.New("received headers again")
			logf("%s: %v", x, err)
			return
		}

		s.headersDone = true
	} else {
		if x.acceptStream == nil {
			task = x.goAwayTask(http2.ErrCodeProtocol)
			err = errors.New("received headers after go-away")
			logf("%s: %v", x, err)
			return
		}

		if f.StreamID <= x.lastRemoteId {
			task = x.goAwayTask(http2.ErrCodeProtocol)
			err = errors.New("received new stream with old id")
			logf("%s: error: %v", x, err)
			return
		}
	}

	err = x.headerDecoder.Decode(f.HeaderBlockFragment(), s == nil)
	if err != nil {
		task = x.goAwayTask(http2.ErrCodeProtocol)
		logf("%s: error: %v", x, err)
		return
	}

	if s == nil {
		s = newStream(ctx.Done(), f.StreamID, x.sendingStreams, x.initialSendWindow, x)
		s.headersDone = true
		x.streamMap[f.StreamID] = s
		x.lastRemoteId = f.StreamID
		logf("%s: remote %s added", x, s)

		x.headerEncoder.Set(":status", "200")
		x.headerEncoder.Set("access-control-allow-origin", "*")

		param := http2.HeadersFrameParam{
			StreamID:      f.StreamID,
			BlockFragment: x.headerEncoder.Pop(),
			EndHeaders:    true,
		}

		task = frameSenderFunc(func(fr *http2.Framer, x fmt.Stringer) error {
			logf("%s: send: sending HEADERS of stream %d", x, param.StreamID)
			return fr.WriteHeaders(param)
		})

		x.acceptStream(s)

		if f.StreamEnded() {
			s.end()
		}
	}

	return
}

func (x *xfer) handleRSTStream(f *http2.RSTStreamFrame) (task frameSender, err error) {
	s := x.streamMap[f.StreamID]
	if s == nil {
		task = x.goAwayTask(http2.ErrCodeProtocol)
		err = errors.New("received reset of unknown stream")
		logf("%s: %v", x, err)
		return
	}

	s.reset()

	delete(x.streamMap, f.StreamID)
	logf("%s: %s removed", x, s)
	return
}

func (x *xfer) handleSettings(f *http2.SettingsFrame) (task frameSender, err error) {
	if f.IsAck() {
		return
	}

	err = f.ForeachSetting(func(s http2.Setting) (err error) {
		switch s.ID {
		case http2.SettingHeaderTableSize:
			x.headerDecoder.Decoder.SetMaxDynamicTableSize(s.Val)

		case http2.SettingMaxConcurrentStreams:
			logf("%s: TODO: implement MAX_CONCURRENT_STREAMS setting", x)

		case http2.SettingInitialWindowSize:
			x.initialSendWindow = int(s.Val)

		case http2.SettingMaxFrameSize:
			logf("%s: TODO: implement MAX_FRAME_SIZE setting", x)

		case http2.SettingMaxHeaderListSize:
			logf("%s: TODO: implement MAX_HEADER_LIST_SIZE setting", x)
		}
		return
	})
	if err != nil {
		task = x.goAwayTask(http2.ErrCodeProtocol)
		logf("%s: error: %v", x, err)
		return
	}

	task = frameSenderFunc(writeSettingsAckFrame)
	return
}

func (x *xfer) handlePushPromise(f *http2.PushPromiseFrame) (task frameSender, err error) {
	task = x.goAwayTask(http2.ErrCodeProtocol)
	err = errors.New("received push promise")
	logf("%s: %v", x, err)
	return
}

func (x *xfer) handlePing(f *http2.PingFrame) (task frameSender, err error) {
	if f.IsAck() {
		return
	}

	data := f.Data

	task = frameSenderFunc(func(fr *http2.Framer, x fmt.Stringer) error {
		logf("%s: send: sending PING ACK", x)
		return fr.WritePing(true, data)
	})

	return
}

func (x *xfer) handleGoAway(f *http2.GoAwayFrame) (task frameSender, err error) {
	if x.acceptStream == nil {
		task = x.goAwayTask(http2.ErrCodeProtocol)
		err = errors.New("received another go-away")
		logf("%s: %v", err, x)
		return
	}

	x.acceptStream(nil)
	x.acceptStream = nil

	if f.ErrCode != http2.ErrCodeNo {
		task = x.goAwayTask(http2.ErrCodeNo)
		err = errors.New("received connection error: " + f.ErrCode.String())
		logf("%s: %v", x, err)
		return
	}

	return
}

func (x *xfer) handleWindowUpdate(f *http2.WindowUpdateFrame) (task frameSender, err error) {
	if f.StreamID == 0 {
		logf("%s: TODO: implement WINDOW_UPDATE for connection", x)
	} else {
		s := x.streamMap[f.StreamID]
		if s == nil {
			task = x.goAwayTask(http2.ErrCodeProtocol)
			err = errors.New("received window update for unknown stream")
			logf("%s: %v", x, err)
			return
		}

		s.updateWindow(int(f.Increment))
	}

	return
}

func (x *xfer) handleContinuation(f *http2.ContinuationFrame) (task frameSender, err error) {
	task = x.goAwayTask(http2.ErrCodeProtocol)
	err = errors.New("received spurious continuation frame")
	logf("%s: %v", x, err)
	return
}

func (x *xfer) newLocalId() (id uint32, err error) {
	if x.nextLocalId >= MaxStreamId-1 {
		err = errStreamIdOverflow
		return
	}

	id = x.nextLocalId
	x.nextLocalId += 2
	return
}

func (x *xfer) goAwayTask(errCode http2.ErrCode) frameSender {
	streamId := x.lastRemoteId
	return frameSenderFunc(func(fr *http2.Framer, x fmt.Stringer) error {
		if errCode == http2.ErrCodeNo {
			logf("%s: send: sending GOAWAY", x)
		} else {
			logf("%s: send: sending GOAWAY (%s)", x, errCode)
		}
		return fr.WriteGoAway(streamId, errCode, nil)
	})
}

func writeSettingsFrame(fr *http2.Framer, x fmt.Stringer) error {
	logf("%s: send: sending SETTINGS", x)
	return fr.WriteSettings(http2.Setting{
		ID:  http2.SettingEnablePush,
		Val: 0,
	})
}

func writeSettingsAckFrame(fr *http2.Framer, x fmt.Stringer) error {
	logf("%s: send: sending SETTINGS ACK", x)
	return fr.WriteSettingsAck()
}

// receiveFrame
type receiveFrame struct {
	frame http2.Frame
	err   error
}

// doReceive
func doReceive(fr *http2.Framer, frames chan<- receiveFrame, again <-chan struct{}, x fmt.Stringer) {
	defer close(frames)

	for {
		logf("%s: receive: receiving", x)

		f, err := fr.ReadFrame()

		if err == nil {
			logf("%s: receive: received frame", x)
		} else {
			logf("%s: receive: error: %v", x, err)
		}

		frames <- receiveFrame{f, err}
		if err != nil {
			return
		}

		logf("%s: receive: stalling", x)

		if _, open := <-again; !open {
			break
		}
	}

	logf("%s: receive: finished", x)
}

// frameSender
type frameSender interface {
	sendFrame(*http2.Framer, fmt.Stringer) (uint32, error)
}

// frameSenderFunc
type frameSenderFunc func(*http2.Framer, fmt.Stringer) error

func (f frameSenderFunc) sendFrame(fr *http2.Framer, x fmt.Stringer) (closedId uint32, err error) {
	err = f(fr, x)
	return
}

// doSend
func doSend(fr *http2.Framer, tasks <-chan frameSender, closedStreams chan<- uint32, feedback chan<- error, x fmt.Stringer) {
	defer func() {
		for range tasks {
		}
	}()

	logf("%s: send: waiting", x)

	for t := range tasks {
		if t == nil {
			// synchronization before close
			continue
		}

		closedId, err := t.sendFrame(fr, x)
		if err != nil {
			logf("%s: send: error: %v", x, err)
			feedback <- err
			return
		}

		if closedId != 0 {
			logf("%s: send: stream %d closed", x, closedId)
			closedStreams <- closedId
		}

		logf("%s: send: waiting", x)
	}

	logf("%s: send: finished", x)
}
