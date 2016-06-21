package stream_test

import (
	"context"
	"net"
	"runtime"
	"strings"
	"testing"

	"github.com/tsavola/gate/stream"
)

type testLogger struct {
	t *testing.T
}

func (tl *testLogger) Printf(format string, v ...interface{}) {
	var id string

	b := make([]byte, 16)
	n := runtime.Stack(b, false)
	if n > 0 {
		parts := strings.SplitN(string(b[:n]), " ", 3)
		if parts != nil && len(parts) == 3 {
			id = parts[1]
		}
	}

	tl.t.Logf("%3s "+format, append([]interface{}{id}, v...)...)
}

type testAddr struct {
	s string
}

func (a testAddr) Network() string {
	return "test"
}

func (a testAddr) String() string {
	return a.s
}

type testConn struct {
	net.Conn
	remoteAddr net.Addr
}

func (c *testConn) RemoteAddr() net.Addr {
	return c.remoteAddr
}

func Test(t *testing.T) {
	ctx := context.Background()

	log := &testLogger{t}
	stream.DebugLog = log

	log.Printf("test: begin")
	defer log.Printf("test: end")

	serverPipe, clientPipe := net.Pipe()

	clientConn := &testConn{clientPipe, testAddr{"client"}}
	serverConn := &testConn{serverPipe, testAddr{"server"}}

	clientDone := make(chan struct{})
	serverDone := make(chan struct{})
	clientDone2 := make(chan struct{})
	serverDone2 := make(chan struct{})

	go func() {
		defer close(clientDone)
		defer log.Printf("test client: exit")

		sr := stream.NewStreamer(false, nil)
		defer sr.Close()

		go func() {
			defer close(clientDone2)
			defer log.Printf("test client: do: exit")
			defer clientConn.Close()

			if err := sr.Do(ctx, clientConn); err != nil {
				t.Errorf("client conn: do: %v", err)
			}
		}()

		log.Printf("test client: creating stream")

		s, err := sr.NewStream(ctx)
		if err != nil {
			t.Errorf("client conn: %v", err)
			return
		}

		defer func() {
			log.Printf("test client: closing %s", s)
			s.Close()
		}()

		log.Printf("test client: writing to %s", s)

		n, err := s.Write([]byte{42})
		if err != nil {
			t.Errorf("stream write error: %v", err)
			return
		} else if n != 1 {
			t.Errorf("stream write error: %d bytes was written to stream", n)
			return
		}

		log.Printf("test client: flushing %s", s)

		if err := s.Flush(); err != nil {
			t.Errorf("stream flush error: %v", err)
			return
		}

		log.Printf("test client: finished")
	}()

	go func() {
		defer close(serverDone)
		defer log.Printf("test client: exit")

		remoteStream := make(chan *stream.Stream)
		gotStream := false

		sr := stream.Streamer{
			Server: true,
			Acceptor: stream.AcceptorFunc(func(sr *stream.Streamer, s *stream.Stream) {
				if gotStream {
					if s != nil {
						log.Printf("test client: draining remote streams")
						s.Close()
					}
				} else {
					remoteStream <- s
					gotStream = true
				}
			}),
		}

		go func() {
			defer close(serverDone2)
			defer log.Printf("test client: do: exit")
			defer serverConn.Close()

			if err := sr.Do(ctx, serverConn); err != nil && err != context.Canceled {
				t.Errorf("server conn: do: %v", err)
			}
		}()

		s := <-remoteStream
		if s == nil {
			t.Errorf("server conn: no streams")
			return
		}

		log.Printf("test client: handling stream: %s", s)

		defer func() {
			log.Printf("test client: closing %s", s)
			s.Close()
		}()

		log.Printf("test client: reading from %s", s)

		buf := make([]byte, 256)
		n, err := s.Read(buf)
		if err != nil {
			t.Errorf("stream read error: %v", err)
			return
		} else if n != 1 {
			t.Errorf("stream read error: %d bytes was read from stream", n)
			return
		}
		log.Printf("test client: data: %#v", buf[:n])

		log.Printf("test client: finished")
	}()

	<-clientDone
	<-serverDone
	<-clientDone2
	<-serverDone2
}
