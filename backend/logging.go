package main

// logging.go — подпатч 6.23 (B14.1): structured logging.
//
// setupLogging(env) настраивает slog.SetDefault:
//   * prod → JSON-хендлер в stdout (для сбора логов)
//   * dev  → text-хендлер в stderr (для чтения глазами)
//
// Уровень: slog.LevelInfo (dev), slog.LevelInfo (prod).
//
// Вызывается из main() сразу после validateEnv.
// Все остальные файлы используют slog.Info/Warn/Error (не log.Printf).

import (
	"log/slog"
	"os"
)

// setupLogging настраивает глобальный slog-логгер.
//   - prod: JSON в stdout — для ELK/Loki/CloudWatch.
//   - dev:  text в stderr — человек читает.
func setupLogging(env *Env) {
	if env == nil {
		env = &Env{Name: "dev"}
	}

	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}

	var handler slog.Handler
	if env.IsProd {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stderr, opts)
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)
}
