package stream

import (
	"golang.org/x/net/context"
)

// Creator
type Creator chan chan *Stream

// NewStream
func (c Creator) NewStream(ctx context.Context) (s *Stream, err error) {
	creation := make(chan *Stream)

	select {
	case c <- creation:
		s = <-creation

	case <-ctx.Done():
		err = ctx.Err()
	}
	return
}
