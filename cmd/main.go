package main

import (
	"fmt"
	"log/slog"
	"os"

	application "github.com/freitasmatheusrn/lifecycle-monitor/application"
	configs "github.com/freitasmatheusrn/lifecycle-monitor/configs"
	"github.com/freitasmatheusrn/lifecycle-monitor/internal/database/postgres"
	redisdb "github.com/freitasmatheusrn/lifecycle-monitor/internal/database/redis"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func main() {
	config, err := configs.LoadConfig(".")
	if err != nil {
		panic(err)
	}

	// Use DATABASE_URL if available (Dokku), otherwise build from individual params
	var dsn string
	if config.DatabaseURL != "" {
		dsn = config.DatabaseURL
	} else {
		dsn = fmt.Sprintf(
			"%s://%s:%s@%s:%s/%s", config.DBDriver, config.DBUser, config.DBPassword, config.DBHost, config.DBPort, config.DBName)
	}

	db, err := postgres.Init(dsn)
	if err != nil {
		panic("error starting db: " + err.Error())
	}

	// Use REDIS_URL if available (Dokku), otherwise build from individual params
	var redisClient *redisdb.Client
	if config.RedisURL != "" {
		redisClient, err = redisdb.NewClientFromURL(config.RedisURL)
	} else {
		redisClient, err = redisdb.NewClient(redisdb.Config{
			Host:     config.RedisHost,
			Port:     config.RedisPort,
			Password: config.RedisPassword,
			DB:       config.RedisDB,
		})
	}
	if err != nil {
		panic("error starting redis: " + err.Error())
	}
	defer redisClient.Close()

	// Configure encoder
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalColorLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	// Console core: all levels (Info+) to stdout
	consoleEncoder := zapcore.NewConsoleEncoder(encoderConfig)
	consoleCore := zapcore.NewCore(
		consoleEncoder,
		zapcore.AddSync(os.Stdout),
		zap.InfoLevel,
	)

	var logger *zap.Logger

	// If log path is configured, add file core for Warn+ only
	if config.LogPath != "" {
		logFile, err := os.OpenFile(config.LogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			panic("failed to open log file: " + err.Error())
		}

		// File encoder without colors
		fileEncoderConfig := encoderConfig
		fileEncoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder // No colors for file
		fileEncoder := zapcore.NewConsoleEncoder(fileEncoderConfig)

		// File core: only Warn and Error levels
		fileCore := zapcore.NewCore(
			fileEncoder,
			zapcore.AddSync(logFile),
			zap.WarnLevel,
		)

		// Combine both cores
		logger = zap.New(zapcore.NewTee(consoleCore, fileCore))
	} else {
		logger = zap.New(consoleCore)
	}
	defer logger.Sync()

	app := application.Application{
		Config: *config,
		Logger: logger,
		DB:     db,
		Redis:  redisClient,
	}

	if err := app.Run(app.Mount()); err != nil {
		slog.Error("server failed to start", "error", err)
		os.Exit(1)
	}
}
