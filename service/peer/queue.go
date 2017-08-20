// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package peer

import (
	"sync"

	"github.com/tsavola/gate/packet"
)

type queue struct {
	buffer   []packet.Buf
	shutdown bool
	wakeup   chan struct{}
	stopped  chan struct{}
	sink     chan<- packet.Buf
}

func (q *queue) inited() bool {
	return q.wakeup != nil
}

func (q *queue) init(lock sync.Locker, sink chan<- packet.Buf) {
	q.wakeup = make(chan struct{}, 1)
	q.stopped = make(chan struct{})
	q.sink = sink
	go q.loop(lock)
}

func (q *queue) enqueue(item packet.Buf, shutdown bool) {
	if shutdown {
		q.shutdown = true
	} else {
		q.buffer = append(q.buffer, item)
	}

	select {
	case q.wakeup <- struct{}{}:
	default:
	}
}

func (q *queue) loop(lock sync.Locker) {
	defer close(q.stopped)

	var item packet.Buf

	for {
		lock.Lock()
		if item == nil && len(q.buffer) > 0 {
			item = q.buffer[0]
			q.buffer = q.buffer[1:]
		}
		shutdown := q.shutdown
		lock.Unlock()

		if shutdown {
			break
		}

		var doSink chan<- packet.Buf

		if item != nil {
			doSink = q.sink
		}

		select {
		case <-q.wakeup:
			// ok

		case doSink <- item:
			item = nil
		}
	}
}
