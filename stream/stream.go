package stream

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"sync"

	"golang.org/x/net/http2"
)

var (
	errClosed         = errors.New("already closed")
	errAborted        = errors.New("streaming aborted")
	errStreamReset    = errors.New("stream reset by peer")
	errWindowOverflow = errors.New("window overflow")
)

// streamReceiver
type streamReceiver struct {
	readable chan struct{}
	sync.Mutex
	buf    bytes.Buffer
	ended  bool
	closed bool
}

func (r *streamReceiver) receive(data []byte, x, s fmt.Stringer) (err error) {
	// receive() and end() are called only from the xfer goroutine
	if r.ended {
		err = errClosed
		logf("%s: %s: receive: %v", x, s, err)
		return
	}

	r.Lock()

	if r.closed {
		r.Unlock()
		return
	}

	if len(data) > 0 {
		oldLen := r.buf.Len()

		if oldLen+len(data) > DefaultWindowSize {
			r.Unlock()

			err = errWindowOverflow
			logf("%s: %s: receive: %v", x, s, err)
			return
		}

		r.buf.Write(data)

		if oldLen == 0 {
			r.setReadable()
		}
	}

	bufLen := r.buf.Len()

	r.Unlock()

	logf("%s: %s: received %d bytes (%d bytes in buffer)", x, s, len(data), bufLen)
	return
}

func (r *streamReceiver) end(x, s fmt.Stringer) (ok bool) {
	// end() is called only from the xfer goroutine
	if r.ended {
		return
	}

	r.Lock()

	var bufLen int

	if !r.closed {
		r.ended = true

		bufLen = r.buf.Len()
		if bufLen == 0 {
			close(r.readable)
		}
	}

	ok = true

	r.Unlock()

	if ok {
		logf("%s: %s: reception ended (%d bytes in buffer)", x, s, bufLen)
	}

	return
}

func (r *streamReceiver) close() (ok bool) {
	r.Lock()
	defer r.Unlock()

	if !r.ended && !r.closed {
		r.closed = true
		ok = true
	}
	return
}

func (r *streamReceiver) read(aborted <-chan struct{}, buf []byte, x, s fmt.Stringer) (n int, err error) {
	logf("%s: %s: reading %d bytes", x, s, len(buf))

	for {
		r.Lock()

		n, err = r.buf.Read(buf)
		if err == io.EOF {
			ended := r.ended

			r.Unlock()

			if !ended {
				logf("%s: %s: read blocking", x, s)

				select {
				case <-r.readable:
					continue

				case <-aborted:
					err = errAborted
				}
			}

			logf("%s: %s: read: %v", x, s, err)
		} else {
			bufLen := r.buf.Len()
			ended := r.ended

			if bufLen > 0 {
				r.setReadable()
			} else if ended {
				close(r.readable)
			}

			r.Unlock()

			if !ended {
				logf("%s: %s: read %d bytes (%d bytes remain in buffer)", x, s, n, bufLen)
			} else {
				logf("%s: %s: read %d bytes (%d bytes remain before EOF)", x, s, n, bufLen)
			}
		}

		return
	}
}

func (r *streamReceiver) setReadable() {
	select {
	case r.readable <- struct{}{}:
	default:
	}
}

// streamSender
type streamSender struct {
	id        uint32
	writable  chan struct{}
	scheduler chan<- frameSender
	sync.Mutex
	window    int
	buf       bytes.Buffer
	closing   error
	scheduled bool
}

func (s *streamSender) updateWindow(increment int, x fmt.Stringer) {
	s.Lock()

	oldWindow := s.window
	newWindow := oldWindow + increment
	s.window = newWindow

	if oldWindow == 0 {
		s.setWritable()
	}

	s.Unlock()

	logf("%s: %s: send window changed from %d to %d", x, s, oldWindow, newWindow)
}

func (s *streamSender) write(aborted <-chan struct{}, data []byte, flush bool, x fmt.Stringer) (int, error) {
	written := 0

	for {
		schedule, n, err := s.writeSome(aborted, data, flush, x)
		written += n

		if schedule {
			err = s.schedule(aborted, false, x)
		}

		if n == len(data) || err != nil {
			return written, err
		}

		data = data[n:]
	}
}

func (s *streamSender) writeSome(aborted <-chan struct{}, data []byte, flush bool, x fmt.Stringer) (schedule bool, n int, err error) {
	logf("%s: %s: writing %d bytes", x, s, len(data))

	s.Lock()

	err = s.closing
	if err != nil {
		s.Unlock()

		logf("%s: %s: write: %v", x, s, err)
		return
	}

	if len(data) > 0 {
		for s.window == 0 {
			err = s.closing

			s.Unlock()

			if err != nil {
				logf("%s: %s: write: %v", x, s, err)
				return
			} else {
				logf("%s: %s: write blocking", x, s)

				select {
				case <-s.writable:

				case <-aborted:
					err = errAborted
					logf("%s: %s: write: %v", x, s, err)
					return
				}
			}

			s.Lock()
		}

		n = s.window
		if n > len(data) {
			n = len(data)
		}

		s.buf.Write(data[:n])
		s.window -= n

		if s.window == 0 {
			flush = true
		}
	}

	bufLen := s.buf.Len()

	if flush && bufLen > 0 && !s.scheduled {
		s.scheduled = true
		schedule = true
	}

	s.Unlock()

	logf("%s: %s: wrote %d bytes (%d bytes in buffer)", x, s, n, bufLen)
	return
}

func (s *streamSender) setWritable() {
	select {
	case s.writable <- struct{}{}:
	default:
	}
}

func (s *streamSender) close(aborted <-chan struct{}, reset bool, x fmt.Stringer) (err error) {
	s.Lock()

	err = s.closing
	if err != nil {
		s.Unlock()

		logf("%s: %s: close: %v", x, s, err)

		if reset {
			err = s.schedule(aborted, reset, x)
		}
		return
	}

	s.closing = errClosed

	sched := reset
	if !s.scheduled {
		s.scheduled = true
		sched = true
	}

	s.Unlock()

	logf("%s: %s: closed", x, s)

	if sched {
		err = s.schedule(aborted, reset, x)
	}
	return
}

func (s *streamSender) reset() {
	s.Lock()
	defer s.Unlock()

	if s.closing == nil || s.scheduled {
		s.closing = errStreamReset
	}
	s.scheduled = false
}

func (s *streamSender) isClosed() bool {
	s.Lock()
	defer s.Unlock()

	return s.closing != nil && !s.scheduled
}

func (s *streamSender) schedule(aborted <-chan struct{}, reset bool, x fmt.Stringer) (err error) {
	logf("%s: %s: scheduling send", x, s)

	var payload frameSender
	if reset {
		payload = streamResetSender{s}
	} else {
		payload = streamDataSender{s}
	}

	select {
	case s.scheduler <- payload:
		logf("%s: %s: scheduled send", x, s)

	case <-aborted:
		err = errAborted
		logf("%s: %s: scheduling: %v", x, s, err)
	}
	return
}

func (s *streamSender) String() string {
	return fmt.Sprintf("stream %d", s.id)
}

// streamDataSender
type streamDataSender struct {
	*streamSender
}

func (s streamDataSender) sendFrame(fr *http2.Framer, x fmt.Stringer) (closedId uint32, err error) {
	s.Lock()
	defer s.Unlock()

	end := (s.closing != nil)

	if end {
		closedId = s.id
	}

	if !s.scheduled {
		return
	}

	s.scheduled = false

	if end {
		logf("%s: send: sending DATA (END_STREAM) frame of %s", x, s)
	} else {
		logf("%s: send: sending DATA frame of %s", x, s)
	}

	err = fr.WriteData(s.id, end, s.buf.Bytes())
	if err != nil {
		return
	}

	s.buf.Reset()
	return
}

// streamResetSender
type streamResetSender struct {
	*streamSender
}

func (s streamResetSender) sendFrame(fr *http2.Framer, x fmt.Stringer) (closedId uint32, err error) {
	s.Lock()
	defer s.Unlock()

	closedId = s.id

	if !s.scheduled {
		return
	}

	s.scheduled = false

	if s.buf.Len() > 0 {
		logf("%s: send: sending DATA frame of %s", x, s)

		err = fr.WriteData(s.id, false, s.buf.Bytes())
		if err != nil {
			return
		}

		s.buf.Reset()
	}

	logf("%s: send: sending RST STREAM frame of %s", x, s)

	err = fr.WriteRSTStream(s.id, http2.ErrCodeNo)
	return
}

// Stream
type Stream struct {
	aborted  <-chan struct{}
	receiver streamReceiver
	sender   streamSender
	x        fmt.Stringer

	headersDone bool // used directly by connection driver
}

func newStream(aborted <-chan struct{}, id uint32, sendScheduler chan<- frameSender, initialSendWindow int, x fmt.Stringer) (s *Stream) {
	s = &Stream{
		aborted: aborted,
		receiver: streamReceiver{
			readable: make(chan struct{}, 1),
		},
		sender: streamSender{
			id:        id,
			writable:  make(chan struct{}, 1),
			scheduler: sendScheduler,
			window:    initialSendWindow,
		},
		x: x,
	}
	s.sender.setWritable()
	return
}

func (s *Stream) Writable() <-chan struct{} {
	return s.sender.writable
}

func (s *Stream) Write(data []byte) (n int, err error) {
	return s.sender.write(s.aborted, data, false, s.x)
}

func (s *Stream) WriteAndFlush(data []byte) (n int, err error) {
	return s.sender.write(s.aborted, data, true, s.x)
}

func (s *Stream) Flush() (err error) {
	_, err = s.sender.write(s.aborted, nil, true, s.x)
	return
}

func (s *Stream) CloseWrite() error {
	return s.sender.close(s.aborted, false, s.x)
}

func (s *Stream) isWriteClosed() bool {
	return s.sender.isClosed()
}

func (s *Stream) updateWindow(increment int) {
	s.sender.updateWindow(increment, s.x)
}

func (s *Stream) UnbufferedWriteCloser() io.WriteCloser {
	return unbufferedStreamWriteCloser{s}
}

func (s *Stream) Readable() <-chan struct{} {
	return s.receiver.readable
}

func (s *Stream) Read(buf []byte) (n int, err error) {
	return s.receiver.read(s.aborted, buf, s.x, s)
}

func (s *Stream) receive(data []byte) error {
	return s.receiver.receive(data, s.x, s)
}

func (s *Stream) end() bool {
	return s.receiver.end(s.x, s)
}

func (s *Stream) isEnded() bool {
	return s.receiver.ended
}

func (s *Stream) Close() (err error) {
	reset := s.receiver.close()
	return s.sender.close(s.aborted, reset, s.x)
}

func (s *Stream) reset() {
	s.receiver.end(s.x, s)
	s.sender.reset()
}

func (s *Stream) String() string {
	return s.sender.String()
}

// unbufferedStreamWriteCloser
type unbufferedStreamWriteCloser struct {
	stream *Stream
}

func (w unbufferedStreamWriteCloser) Write(data []byte) (n int, err error) {
	return w.stream.WriteAndFlush(data)
}

func (w unbufferedStreamWriteCloser) Close() error {
	return w.stream.CloseWrite()
}
