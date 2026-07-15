// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package logger

import (
	"os"
	"time"

	"github.com/rs/zerolog"
)

var Log zerolog.Logger

func Init(level, format string) {
	lvl, err := zerolog.ParseLevel(level)
	if err != nil {
		lvl = zerolog.InfoLevel
	}
	var output zerolog.ConsoleWriter
	if format == "console" {
		output = zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
			NoColor:    false,
		}
		Log = zerolog.New(output).Level(lvl).With().Timestamp().Logger()
	} else {
		Log = zerolog.New(os.Stdout).Level(lvl).With().Timestamp().Logger()
	}
	zerolog.SetGlobalLevel(lvl)
}
