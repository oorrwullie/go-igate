package main

import (
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
)

type Logger struct {
	stdout *logrus.Logger
	file   *logrus.Logger
}

type ColorFormatter struct {
	logrus.Formatter
}

const (
	reset   = "\033[0m"
	red     = "\033[31m"
	green   = "\033[32m"
	yellow  = "\033[33m"
	blue    = "\033[34m"
	magenta = "\033[35m"
	cyan    = "\033[36m"
	white   = "\033[37m"
)

func (f *ColorFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	var levelColor string
	switch entry.Level {
	case logrus.DebugLevel, logrus.TraceLevel:
		levelColor = blue
	case logrus.InfoLevel:
		levelColor = green
	case logrus.WarnLevel:
		levelColor = yellow
	case logrus.ErrorLevel, logrus.FatalLevel, logrus.PanicLevel:
		levelColor = red
	default:
		levelColor = white
	}

	message := fmt.Sprintf("%s[%s] %s%s\n", levelColor, entry.Level, entry.Message, reset)
	return []byte(message), nil
}

func NewLogger() (*Logger, error) {
	stdout := logrus.New()
	stdout.SetOutput(os.Stdout)
	stdout.SetLevel(logrus.DebugLevel)
	stdout.SetFormatter(&ColorFormatter{})

	fileLog := logrus.New()

	file, err := os.OpenFile(
		"go-igate.log",
		os.O_CREATE|os.O_WRONLY|os.O_APPEND,
		0666,
	)
	if err != nil {
		return nil, fmt.Errorf("Failed to initialize log file: %v", err)
	}

	fileLog.SetOutput(file)
	fileLog.SetLevel(logrus.DebugLevel)
	fileLog.SetFormatter(&ColorFormatter{})

	return &Logger{
		stdout: stdout,
		file:   fileLog,
	}, nil
}

func (l *Logger) Debug(args ...interface{}) {
	l.stdout.Debug(args...)
	l.file.Debug(args...)
}

func (l *Logger) Info(args ...interface{}) {
	l.stdout.Info(args...)
	l.file.Info(args...)
}

func (l *Logger) Error(args ...interface{}) {
	l.stdout.Error(args...)
	l.file.Error(args...)
}

func (l *Logger) Fatal(args ...interface{}) {
	l.stdout.Fatal(args...)
	l.file.Fatal(args...)
}

func (l *Logger) Fatalf(msg string, args ...interface{}) {
	l.stdout.Fatalf(msg, args...)
	l.file.Fatalf(msg, args...)
}

func (l *Logger) Warn(args ...interface{}) {
	l.stdout.Warn(args...)
	l.file.Warn(args...)
}

func (l *Logger) Printf(msg string, args ...interface{}) {
	l.stdout.Printf(msg, args...)
	l.file.Printf(msg, args...)
}
