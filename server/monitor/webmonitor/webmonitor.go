// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webmonitor

import (
	"context"
	"net/http"
	"path"
	"time"

	"gate.computer/gate/internal/defaultlog"
	"gate.computer/gate/server"
	"gate.computer/gate/server/detail"
	"gate.computer/gate/server/event"
	"gate.computer/gate/server/monitor"
	"github.com/gorilla/websocket"
)

var (
	closeGoingAway     = websocket.FormatCloseMessage(websocket.CloseGoingAway, "")
	closeTryAgainLater = websocket.FormatCloseMessage(websocket.CloseTryAgainLater, "")
)

type Config struct {
	Origins   []string // nil means any, empty list means none
	StaticDir string
	ErrorLog  Logger
}

func (config *Config) checkOrigin(r *http.Request) bool {
	if config.Origins == nil {
		return true
	}

	origin := r.Header.Get("Origin")
	if origin == "" {
		return false
	}

	for _, allowed := range config.Origins {
		if origin == allowed {
			return true
		}
	}

	return false
}

func New(ctx context.Context, monitorConfig *monitor.Config, handlerConfig *Config) (func(server.Event, error), http.Handler) {
	m, s := monitor.New(ctx, monitorConfig)
	return m, Handler(ctx, s, handlerConfig)
}

func Handler(ctx context.Context, s *monitor.MonitorState, config *Config) http.Handler {
	var c Config
	if config != nil {
		c = *config
	}
	if c.ErrorLog == nil {
		c.ErrorLog = defaultlog.StandardLogger{}
	}

	initTime := time.Now()

	mux := http.NewServeMux()

	mux.HandleFunc("/websocket.json", func(w http.ResponseWriter, r *http.Request) {
		handle(ctx, w, r, s, &c, initTime)
	})

	if c.StaticDir != "" {
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-cache")
			http.ServeFile(w, r, path.Join(c.StaticDir, "dashboard.html"))
		})

		mux.HandleFunc("/dashboard.js", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-cache")
			http.ServeFile(w, r, path.Join(c.StaticDir, "dashboard.js"))
		})
	}

	return mux
}

func handle(ctx context.Context, w http.ResponseWriter, r *http.Request, s *monitor.MonitorState, c *Config, initTime time.Time) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	u := websocket.Upgrader{
		CheckOrigin: c.checkOrigin,
	}

	conn, err := u.Upgrade(w, r, nil)
	if err != nil {
		c.ErrorLog.Printf("%v", err)
		return
	}
	defer conn.Close()

	handleClose := conn.CloseHandler()
	conn.SetCloseHandler(func(code int, text string) error {
		cancel()
		return handleClose(code, text)
	})

	items := make(chan monitor.Item)
	snapshot, err := s.Subscribe(ctx, items)
	if err != nil {
		conn.WriteMessage(websocket.CloseMessage, closeGoingAway)
		return
	}
	defer s.Unsubscribe(ctx, items)

	frame := map[string]interface{}{
		"server_init": initTime.Unix(),
		"iface_types": detail.Iface_value,
		"event_types": event.Type_value,
		"state":       snapshot,
	}

	if err := conn.WriteJSON(frame); err != nil {
		c.ErrorLog.Printf("%v", err)
		return
	}

	for {
		select {
		case item, open := <-items:
			if !open {
				c.ErrorLog.Printf("closing slow webmonitor connection from %s", r.RemoteAddr)
				conn.WriteMessage(websocket.CloseMessage, closeTryAgainLater)
				return
			}

			frame := map[string]interface{}{
				"type":  item.Event.EventType(),
				"event": item.Event,
			}
			if item.Error != nil {
				frame["error"] = item.Error.Error()
			}

			if err := conn.WriteJSON(frame); err != nil {
				c.ErrorLog.Printf("%v", err)
				return
			}

		case <-ctx.Done():
			conn.WriteMessage(websocket.CloseMessage, closeGoingAway)
			return
		}
	}
}
