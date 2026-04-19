// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package logger

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/go-logr/logr"
)

const (
	LevelDebug = "debug"
	LevelInfo  = "info"
	LevelWarn  = "warn"
	LevelError = "error"
)

func NormalizeLevel(level string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "", LevelInfo:
		return LevelInfo, nil
	case LevelDebug:
		return LevelDebug, nil
	case LevelWarn:
		return LevelWarn, nil
	case LevelError:
		return LevelError, nil
	default:
		return "", fmt.Errorf("invalid log level %q, expect one of %q, %q, %q, %q", level, LevelDebug, LevelInfo, LevelWarn, LevelError)
	}
}

func ParseLevel(level string) (slog.Level, error) {
	normalized, err := NormalizeLevel(level)
	if err != nil {
		return 0, err
	}

	switch normalized {
	case LevelDebug:
		return slog.LevelDebug, nil
	case LevelWarn:
		return slog.LevelWarn, nil
	case LevelError:
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, nil
	}
}

func New(level string) (*slog.Logger, error) {
	slogLevel, err := ParseLevel(level)
	if err != nil {
		return nil, err
	}

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: true,
		Level:     slogLevel,
	})
	return slog.New(handler), nil
}

func MustNew(level string) *slog.Logger {
	logger, err := New(level)
	if err != nil {
		panic(err)
	}
	return logger
}

func ToLogr(logger *slog.Logger) logr.Logger {
	return logr.FromSlogHandler(logger.Handler())
}

func WithName(logger *slog.Logger, name string) *slog.Logger {
	return logger.With("logger", name)
}
