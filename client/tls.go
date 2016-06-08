package main

import (
	"context"
	"crypto/tls"
	"net"
)

func dialContextTLS(ctx context.Context, network, addr string, config *tls.Config) (*tls.Conn, error) {
	var dialer net.Dialer

	rawConn, err := dialer.DialContext(ctx, network, addr)
	if err != nil {
		return nil, err
	}

	conn := tls.Client(rawConn, config)

	handshaked := make(chan error, 1)

	go func() {
		handshaked <- conn.Handshake()
	}()

	select {
	case err := <-handshaked:
		if err != nil {
			rawConn.Close()
			return nil, err
		}

		return conn, nil

	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
