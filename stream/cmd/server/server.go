package main

import (
	"context"
	"crypto/tls"
	logpkg "log"
	"net"
	"os"
	"os/signal"
	"runtime/pprof"
	"syscall"

	"github.com/tsavola/gate/stream"
	"github.com/tsavola/gate/stream/tlsconfig"
)

const (
	addr     = "localhost:44321"
	certFile = "cert.pem"
	keyFile  = "key.pem"
)

var (
	log = logpkg.New(os.Stderr, "", logpkg.Lmicroseconds)
)

func init() {
	stream.DebugLog = log
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go handleSignals(cancel)

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		log.Printf("server: error: %v", err)
		os.Exit(1)
	}

	tlsConfig := tlsconfig.Defaults(&tls.Config{
		Certificates: []tls.Certificate{
			cert,
		},
	})

	l, err := tls.Listen("tcp", addr, tlsConfig)
	if err != nil {
		log.Printf("server: error: %v", err)
		os.Exit(1)
	}
	defer l.Close()

	go func() {
		defer l.Close()
		<-ctx.Done()
	}()

	for {
		c, err := l.Accept()

		select {
		case <-ctx.Done():
			if c != nil {
				c.Close()
			}
			return

		default:
		}

		if err != nil {
			log.Printf("server: error: %v", err)
			os.Exit(1)
		}

		go handleConn(ctx, c)
	}
}

func handleConn(ctx context.Context, c net.Conn) {
	defer c.Close()

	accept := func(sr *stream.Streamer, s *stream.Stream) {
		if s != nil {
			go handleStream(s, c)
		}
	}

	sr := &stream.Streamer{
		Server:   true,
		Acceptor: stream.AcceptorFunc(accept),
	}

	if err := sr.Do(ctx, c); err != nil && err != context.Canceled {
		log.Printf("server: error: %v", err)
	}
}

func handleStream(s *stream.Stream, c net.Conn) {
	log.Printf("server: %s with %v created", s, c.RemoteAddr())
	defer log.Printf("server: %s with %v closed", s, c.RemoteAddr())

	defer s.Close()

	buf := make([]byte, stream.DefaultWindowSize)

	for range s.Readable() {
		n, _ := s.Read(buf)
		log.Printf("server: %s received %#v from %v", s, string(buf[:n]), c.RemoteAddr())

		if _, err := s.WriteAndFlush(buf[:n]); err != nil {
			log.Printf("server: %s write error: %v", s, err)
			return
		}
	}
}

func handleSignals(cancel context.CancelFunc) {
	defer cancel()

	c := make(chan os.Signal)
	signal.Ignore()
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	for s := range c {
		switch s {
		case syscall.SIGQUIT:
			pprof.Lookup("goroutine").WriteTo(os.Stderr, 2)

		default:
			return
		}
	}
}
