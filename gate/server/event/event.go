// Copyright (c) 2022 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package event

import (
	"log/slog"
	"time"

	"gate.computer/gate/server/api"
	"gate.computer/gate/server/event/pb"
)

type (
	Event         = pb.Event
	EventFail     = pb.Event_Fail
	EventInstance = pb.Event_Instance
	EventModule   = pb.Event_Module
	Fail          = pb.Fail
	FailType      = pb.Fail_Type
	Instance      = pb.Instance
	Module        = pb.Module
	Type          = pb.Type
)

func NewRecord(ev *Event, err error) slog.Record {
	level := slog.LevelInfo
	if ev.Type == TypeFailInternal {
		level = slog.LevelError
	}

	r := slog.NewRecord(time.Time{}, level, "server: event", 0)

	attrs := make([]slog.Attr, 0, 10)
	attrs = append(attrs, slog.String("type", ev.Type.String()))

	if m := ev.Meta; m != nil {
		if m.Iface != 0 {
			attrs = append(attrs, slog.String("iface", m.Iface.String()))
		}
		if m.Req != 0 {
			attrs = append(attrs, slog.Uint64("req", m.Req))
		}
		if m.Addr != "" {
			attrs = append(attrs, slog.String("addr", m.Addr))
		}
		if m.Op != 0 {
			attrs = append(attrs, slog.String("op", m.Op.String()))
		}
		if m.Principal != "" {
			attrs = append(attrs, slog.String("principal", m.Principal))
		}
	}

	switch info := ev.Info.(type) {
	case *EventFail:
		fields := make([]slog.Attr, 0, 6)

		if ev.Type == TypeFailRequest {
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

	case *EventModule:
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

	case *EventInstance:
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

	r.AddAttrs(attrs...)
	return r
}
