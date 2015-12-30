package stream

// Acceptor
type Acceptor interface {
	AcceptStream(*Streamer, *Stream)
}

// AcceptorFunc
type AcceptorFunc func(*Streamer, *Stream)

func (f AcceptorFunc) AcceptStream(sr *Streamer, s *Stream) {
	f(sr, s)
}
