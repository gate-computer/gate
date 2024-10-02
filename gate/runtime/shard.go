// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import (
	"math/rand"

	. "import.name/type/context"
)

// DistributeProcesses among multiple executors.
func DistributeProcesses(executors ...ProcessFactory) ProcessFactory {
	if len(executors) == 1 {
		return executors[0]
	}

	cs := chanSharder{make([]<-chan ResultProcess, 0, len(executors))}

	for _, x := range executors {
		if c, ok := x.(ProcessChan); ok {
			cs.channels = append(cs.channels, c)
		} else {
			return sharder{executors}
		}
	}

	return cs
}

// DistributeGroupProcesses among multiple executors.
func DistributeGroupProcesses(executors ...GroupProcessFactory) GroupProcessFactory {
	if len(executors) == 1 {
		return executors[0]
	}

	return groupSharder{executors}
}

type sharder struct {
	factories []ProcessFactory
}

func (s sharder) NewProcess(ctx Context) (*Process, error) {
	return s.factories[rand.Intn(len(s.factories))].NewProcess(ctx)
}

type groupSharder struct {
	factories []GroupProcessFactory
}

func (s groupSharder) NewProcess(ctx Context) (*Process, error) {
	return s.NewGroupProcess(ctx, nil)
}

func (s groupSharder) NewGroupProcess(ctx Context, g *ProcessGroup) (*Process, error) {
	return s.factories[rand.Intn(len(s.factories))].NewGroupProcess(ctx, g)
}

type chanSharder struct {
	channels []<-chan ResultProcess
}

func (cs chanSharder) NewProcess(ctx Context) (*Process, error) {
	var firstChoice int
	var unseen []int

	for {
		var choice int

		if unseen == nil {
			firstChoice = rand.Intn(len(cs.channels))
			choice = firstChoice
		} else {
			choice = unseen[rand.Intn(len(unseen))]
		}

		select {
		case x, ok := <-cs.channels[choice]:
			if !ok {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()

				default:
					return nil, errProcessChanClosed
				}
			}

			return x.Process, x.Err

		default:
			if unseen == nil {
				unseen = make([]int, len(cs.channels)-1)

				var value int
				for i := 0; i < len(unseen); i++ {
					if value == choice {
						value++
					}
					unseen[i] = value
					value++
				}
			} else {
				unseen = append(unseen[:choice], unseen[choice+1:]...)
			}
		}

		if len(unseen) == 0 {
			break
		}
	}

	return ProcessChan(cs.channels[firstChoice]).NewProcess(ctx)
}
