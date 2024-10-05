// Copyright (c) 2024 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package logging

import (
	"log/slog"
	"time"

	"import.name/sjournal"
)

// Init returns some kind of logger on error.
func Init(journal bool) (log *slog.Logger, err error) {
	log = slog.Default()

	if journal {
		opts := &sjournal.HandlerOptions{
			Delimiter:  sjournal.ColonDelimiter,
			TimeFormat: time.RFC3339Nano,
		}

		var h slog.Handler

		h, err = sjournal.NewHandler(opts)
		if err == nil {
			log = slog.New(h)
		}
	}

	slog.SetDefault(log)
	slog.SetLogLoggerLevel(slog.LevelError)
	return
}
