package stream

import (
	"bytes"
	"errors"
	"strings"

	"golang.org/x/net/http2/hpack"
)

const (
	defaultHeaderTableSize = 4096
)

// headerValues
type headerValues struct {
	request  bool
	response bool
	invalid  bool

	method string
	status string
}

// headerDecoder
type headerDecoder struct {
	Decoder *hpack.Decoder

	headerValues
}

func newHeaderDecoder() (hd *headerDecoder) {
	hd = new(headerDecoder)

	hd.Decoder = hpack.NewDecoder(defaultHeaderTableSize, func(h hpack.HeaderField) {
		if strings.HasPrefix(h.Name, ":") {
			switch h.Name {
			case ":method":
				hd.request = true
				hd.method = h.Value

			case ":authority", ":path", ":scheme":
				hd.request = true

			case ":status":
				hd.response = true
				hd.status = h.Value

			default:
				hd.invalid = true
			}
		}
	})

	return
}

func (hd *headerDecoder) reset() {
	hd.headerValues = headerValues{}
}

func (hd *headerDecoder) Decode(data []byte, expectRequest bool) (err error) {
	defer hd.reset()

	_, err = hd.Decoder.Write(data)
	if err != nil {
		return
	}

	if hd.invalid {
		err = errors.New("unknown pseudo-header")
		return
	}

	if hd.request && hd.response {
		err = errors.New("mixed request and response pseudo-headers")
		return
	}

	if !hd.request && !hd.response {
		err = errors.New("no pseudo-headers")
		return
	}

	if expectRequest && !hd.request {
		err = errors.New("expected request pseudo-headers")
		return
	}

	if !expectRequest && !hd.response {
		err = errors.New("expected response pseudo-headers")
		return
	}

	if expectRequest {
		if hd.method != "POST" {
			err = errors.New("invalid method")
			return
		}
	} else {
		if hd.status != "200" {
			err = errors.New("invalid status")
			return
		}
	}

	return
}

// headerEncoder
type headerEncoder struct {
	buf     bytes.Buffer
	encoder *hpack.Encoder
}

func newHeaderEncoder() (he *headerEncoder) {
	he = new(headerEncoder)
	he.encoder = hpack.NewEncoder(&he.buf)
	return
}

func (he *headerEncoder) Set(name, value string) {
	he.encoder.WriteField(hpack.HeaderField{Name: name, Value: value})
}

func (he *headerEncoder) Pop() (data []byte) {
	data = make([]byte, he.buf.Len())
	he.buf.Read(data)
	return
}
