package tlsconfig

import (
	"crypto/tls"

	"golang.org/x/net/http2"
)

var (
	NextProtos = []string{
		http2.NextProtoTLS,
	}

	CipherSuites = []uint16{
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
	}

	MinVersion uint16 = tls.VersionTLS12
)

func Defaults(c *tls.Config) *tls.Config {
	if c == nil {
		c = new(tls.Config)
	}

	if c.NextProtos == nil {
		c.NextProtos = NextProtos
	}

	if c.CipherSuites == nil {
		c.CipherSuites = CipherSuites
	}

	if c.MinVersion == 0 {
		c.MinVersion = MinVersion
	}

	return c
}
