package log

import (
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
)

type Logger struct {
	stdout *logrus.Logger
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

func New() (*Logger, error) {
	stdout := logrus.New()
	stdout.SetOutput(os.Stdout)
	stdout.SetLevel(logrus.DebugLevel)
	stdout.SetFormatter(&ColorFormatter{})

	return &Logger{
		stdout: stdout,
	}, nil
}

func (l *Logger) Debug(args ...interface{}) {
	l.stdout.Debug(args...)
}

func (l *Logger) Info(args ...interface{}) {
	l.stdout.Info(args...)
}

func (l *Logger) Error(args ...interface{}) {
	l.stdout.Error(args...)
}

func (l *Logger) Fatal(args ...interface{}) {
	l.stdout.Fatal(args...)
}

func (l *Logger) Fatalf(msg string, args ...interface{}) {
	l.stdout.Fatalf(msg, args...)
}

func (l *Logger) Warn(args ...interface{}) {
	l.stdout.Warn(args...)
}

func (l *Logger) Printf(msg string, args ...interface{}) {
	l.stdout.Printf(msg, args...)
}
