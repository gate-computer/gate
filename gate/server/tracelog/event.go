// Copyright (c) 2024 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tracelog

import (
	"log/slog"
	"time"

	"gate.computer/gate/principal"
	"gate.computer/gate/server/api"
	"gate.computer/gate/server/event"
	"gate.computer/gate/trace"

	. "import.name/type/context"
)

var DefaultEventLevels = EventLevels{
	FailInternal: slog.LevelError,
	Fail:         slog.LevelInfo,
	Error:        slog.LevelInfo,
	Other:        slog.LevelInfo,
}

// EventLevels maps server event categories to log levels.
type EventLevels struct {
	FailInternal slog.Level // Internal failure.
	Fail         slog.Level // Other failure.
	Error        slog.Level // Non-failure, but with associated error information.
	Other        slog.Level // No failure or error.
}

// Level maps server event type to log level.  Presence of error value may
// affect the level.
func (ls *EventLevels) Level(ev *event.Event, err error) slog.Level {
	switch t := ev.GetType(); {
	case t <= event.TypeFailInternal:
		return ls.FailInternal
	case t <= event.TypeFailRequest:
		return ls.Fail
	case err != nil:
		return ls.Error
	default:
		return ls.Other
	}
}

// EventAdder returns a function which can be used as AddEvent callback.
// Internal errors are logged at error level, and other events at info level.
// If logger is nil, current default logger is used for each log record.  If
// levels is nil, defaults are used.
func EventAdder(logger *slog.Logger, prefix string, levels func(*event.Event, error) slog.Level) func(Context, *event.Event, error) {
	msg := prefix + "event"

	if levels == nil {
		levels = DefaultEventLevels.Level
	}

	return func(ctx Context, ev *event.Event, err error) {
		l := logger
		if l == nil {
			l = slog.Default()
		}

		level := levels(ev, err)
		if !l.Enabled(ctx, level) {
			return
		}

		_ = l.Handler().Handle(ctx, EventRecord(ctx, level, msg, ev, err))
	}
}

// EventRecord creates a loggable representation of a server event.
func EventRecord(ctx Context, level slog.Level, msg string, ev *event.Event, err error) slog.Record {
	r := slog.NewRecord(time.Time{}, level, msg, 0)
	r.AddAttrs(EventAttrs(ctx, ev, err)...)
	return r
}

// EventAttrs converts an event into log attributes.
func EventAttrs(ctx Context, ev *event.Event, err error) []slog.Attr {
	attrs := make([]slog.Attr, 0, 7)
	attrs = append(attrs, slog.String("type", ev.Type.String()))
	if pri := principal.ContextID(ctx); pri != nil {
		attrs = append(attrs, slog.String("principal", pri.String()))
	}
	if id, ok := trace.ContextTraceID(ctx); ok {
		attrs = append(attrs, slog.String("trace", id.String()))
	}
	if id, ok := trace.ContextSpanID(ctx); ok {
		attrs = append(attrs, slog.String("span", id.String()))
	}

	// TODO: all links, with span ids
	for _, l := range trace.ContextAutoLinks(ctx) {
		attrs = append(attrs, slog.String("link", l.TraceID.String()))
		break
	}

	switch info := ev.Info.(type) {
	case *event.EventFail:
		fields := make([]slog.Attr, 0, 6)

		if ev.Type == event.TypeFailRequest {
			fields = append(fields, slog.String("type", info.Fail.Type.String()))
		}
		if info.Fail.Source != "" {
			fields = append(fields, slog.String("source", info.Fail.Source))
		}
		if info.Fail.Module != "" {
			fields = append(fields, slog.String("module", info.Fail.Module))
		}
		if info.Fail.Function != "" {
			fields = append(fields, slog.String("function", info.Fail.Function))
		}
		if info.Fail.Instance != "" {
			fields = append(fields, slog.String("instance", info.Fail.Instance))
		}
		if info.Fail.Subsystem != "" {
			fields = append(fields, slog.String("subsystem", info.Fail.Subsystem))
		}

		attrs = append(attrs, slog.Attr{
			Key:   "fail",
			Value: slog.GroupValue(fields...),
		})

	case *event.EventModule:
		fields := make([]slog.Attr, 0, 5)

		if info.Module.Module != "" {
			fields = append(fields, slog.String("module", info.Module.Module))
		}
		if info.Module.Source != "" {
			fields = append(fields, slog.String("source", info.Module.Source))
		}
		if info.Module.Compiled {
			fields = append(fields, slog.Bool("compiled", info.Module.Compiled))
		}
		if info.Module.Length != 0 {
			fields = append(fields, slog.Int64("length", info.Module.Length))
		}
		if info.Module.TagCount != 0 {
			fields = append(fields, slog.Int("tagcount", int(info.Module.TagCount)))
		}

		attrs = append(attrs, slog.Attr{
			Key:   "module",
			Value: slog.GroupValue(fields...),
		})

	case *event.EventInstance:
		fields := make([]slog.Attr, 0, 9)

		if info.Instance.Instance != "" {
			fields = append(fields, slog.String("instance", info.Instance.Instance))
		}
		if info.Instance.Module != "" {
			fields = append(fields, slog.String("module", info.Instance.Module))
		}
		if info.Instance.Function != "" {
			fields = append(fields, slog.String("function", info.Instance.Function))
		}
		if info.Instance.Transient {
			fields = append(fields, slog.Bool("transient", info.Instance.Transient))
		}
		if info.Instance.Suspended {
			fields = append(fields, slog.Bool("suspened", info.Instance.Suspended))
		}
		if info.Instance.Persist {
			fields = append(fields, slog.Bool("persist", info.Instance.Persist))
		}
		if info.Instance.Compiled {
			fields = append(fields, slog.Bool("compiled", info.Instance.Compiled))
		}
		if info.Instance.Status != nil {
			props := make([]slog.Attr, 0, 4)

			if info.Instance.Status.State != 0 {
				props = append(props, slog.String("state", info.Instance.Status.State.String()))
			}
			if info.Instance.Status.Cause != 0 {
				props = append(props, slog.String("cause", info.Instance.Status.Cause.String()))
			}
			if x := info.Instance.Status.State; x == api.StateHalted || x == api.StateTerminated {
				props = append(props, slog.Int("result", int(info.Instance.Status.Result)))
			}
			if info.Instance.Status.Error != "" {
				props = append(props, slog.String("error", info.Instance.Status.Error))
			}

			fields = append(fields, slog.Attr{
				Key:   "status",
				Value: slog.GroupValue(props...),
			})
		}
		if info.Instance.TagCount != 0 {
			fields = append(fields, slog.Int("tagcount", int(info.Instance.TagCount)))
		}

		attrs = append(attrs, slog.Attr{
			Key:   "instance",
			Value: slog.GroupValue(fields...),
		})
	}

	if err != nil {
		attrs = append(attrs, slog.Any("error", err))
	}

	return attrs
}
