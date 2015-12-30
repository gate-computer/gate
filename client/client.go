package main

import (
	"crypto/tls"
	"io/ioutil"
	logpkg "log"
	"os"
	"os/signal"
	"runtime/pprof"
	"syscall"

	"golang.org/x/net/context"

	"github.com/tsavola/gate/stream"
	"github.com/tsavola/gate/stream/tlsconfig"
)

const (
	addr = "localhost:44321"
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

	tlsConfig := tlsconfig.Defaults(nil)
	tlsConfig.InsecureSkipVerify = true

	c, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		log.Printf("client: error: %v", err)
		os.Exit(1)
	}
	defer c.Close()

	sr := stream.NewStreamer(false, nil)

	go sendFile(ctx, sr.Creator)

	if err := sr.Do(ctx, c); err != nil {
		if err != context.Canceled {
			log.Printf("client: error: %v", err)
			os.Exit(1)
		}
	}
}

func sendFile(ctx context.Context, creator stream.Creator) {
	defer close(creator)

	data, err := ioutil.ReadFile("/etc/os-release")
	if err != nil {
		log.Printf("client: error: %v", err)
		return
	}

	s, err := creator.NewStream(ctx)
	if err != nil {
		log.Printf("client: error: %v", err)
		return
	}
	defer s.Close()

	if _, err := s.WriteAndFlush(data); err != nil {
		log.Printf("client: %s: write error: %v", s, err)
		return
	}

	buf := make([]byte, len(data))

	n, err := s.Read(buf)
	if err != nil {
		log.Printf("client: %s: read error: %v", s, err)
		return
	}

	log.Printf("client: received %#v", string(buf[:n]))
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
