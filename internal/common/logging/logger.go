// Package logging provides a structured logging implementation for the application
package logging

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
)

// LogLevel represents different levels of logging
type LogLevel int

const (
	// LevelDebug is for detailed debugging information
	LevelDebug LogLevel = iota
	// LevelInfo is for general operational information
	LevelInfo
	// LevelWarn is for warning events that might need attention
	LevelWarn
	// LevelError is for error events that might still allow the application to continue running
	LevelError
	// LevelFatal is for severe error events that will lead the application to abort
	LevelFatal
)

var levelNames = map[LogLevel]string{
	LevelDebug: "DEBUG",
	LevelInfo:  "INFO",
	LevelWarn:  "WARN",
	LevelError: "ERROR",
	LevelFatal: "FATAL",
}

// Logger provides structured logging capabilities
type Logger struct {
	name      string
	stdLogger *log.Logger
	minLevel  LogLevel
	mu        sync.Mutex
}

// New creates a new logger with the given name and minimum log level
func New(name string, minLevel LogLevel) *Logger {
	return &Logger{
		name:      name,
		stdLogger: log.New(os.Stdout, "", log.LstdFlags),
		minLevel:  minLevel,
	}
}

// WithName creates a new logger with a different name but the same configuration
func (l *Logger) WithName(name string) *Logger {
	return &Logger{
		name:      name,
		stdLogger: l.stdLogger,
		minLevel:  l.minLevel,
	}
}

// WithLevel creates a new logger with a different minimum log level
func (l *Logger) WithLevel(level LogLevel) *Logger {
	return &Logger{
		name:      l.name,
		stdLogger: l.stdLogger,
		minLevel:  level,
	}
}

// SetOutput sets the output destination for the logger
func (l *Logger) SetOutput(w io.Writer) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.stdLogger.SetOutput(w)
}

// SetMinLevel sets the minimum log level
func (l *Logger) SetMinLevel(level LogLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.minLevel = level
}

// Debug logs a message at debug level using printf-style formatting
func (l *Logger) Debug(format string, v ...interface{}) {
	l.log(LevelDebug, format, v...)
}

// DebugKV logs a message at debug level with key-value pairs
func (l *Logger) DebugKV(msg string, keyValues ...interface{}) {
	l.logKV(LevelDebug, msg, keyValues...)
}

// Info logs a message at info level using printf-style formatting
func (l *Logger) Info(format string, v ...interface{}) {
	l.log(LevelInfo, format, v...)
}

// InfoKV logs a message at info level with key-value pairs
func (l *Logger) InfoKV(msg string, keyValues ...interface{}) {
	l.logKV(LevelInfo, msg, keyValues...)
}

// Warn logs a message at warning level using printf-style formatting
func (l *Logger) Warn(format string, v ...interface{}) {
	l.log(LevelWarn, format, v...)
}

// WarnKV logs a message at warning level with key-value pairs
func (l *Logger) WarnKV(msg string, keyValues ...interface{}) {
	l.logKV(LevelWarn, msg, keyValues...)
}

// Error logs a message at error level using printf-style formatting
func (l *Logger) Error(format string, v ...interface{}) {
	l.log(LevelError, format, v...)
}

// ErrorKV logs a message at error level with key-value pairs
func (l *Logger) ErrorKV(msg string, keyValues ...interface{}) {
	l.logKV(LevelError, msg, keyValues...)
}

// Fatal logs a message at fatal level and then exits
func (l *Logger) Fatal(format string, v ...interface{}) {
	l.log(LevelFatal, format, v...)
	os.Exit(1)
}

// FatalKV logs a message at fatal level with key-value pairs and then exits
func (l *Logger) FatalKV(msg string, keyValues ...interface{}) {
	l.logKV(LevelFatal, msg, keyValues...)
	os.Exit(1)
}

// Printf is a compatibility method for the standard logger interface
func (l *Logger) Printf(format string, v ...interface{}) {
	l.Info(format, v...)
}

// Println is a compatibility method for the standard logger interface
func (l *Logger) Println(v ...interface{}) {
	l.Info("%s", fmt.Sprint(v...))
}

// log formats and writes a log message at the specified level using printf-style formatting
func (l *Logger) log(level LogLevel, format string, v ...interface{}) {
	if level < l.minLevel {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// Traditional printf-style logging
	msg := fmt.Sprintf(format, v...)
	l.stdLogger.Printf("[%s] %s: %s", levelNames[level], l.name, msg)
}

// logKV writes a log message with key-value pairs at the specified level
func (l *Logger) logKV(level LogLevel, msg string, keyValues ...interface{}) {
	if level < l.minLevel {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// Ensure we have an even number of key-value pairs
	if len(keyValues)%2 != 0 {
		keyValues = append(keyValues, "<missing value>")
	}

	// Format key-value pairs
	kvPairs := make([]string, 0, len(keyValues)/2)
	for i := 0; i < len(keyValues); i += 2 {
		key, ok := keyValues[i].(string)
		if !ok {
			key = fmt.Sprintf("%v", keyValues[i])
		}
		value := keyValues[i+1]
		kvPairs = append(kvPairs, fmt.Sprintf("%s=%v", key, value))
	}

	l.stdLogger.Printf("[%s] %s: %s %s", levelNames[level], l.name, msg, strings.Join(kvPairs, " "))
}

// ParseLevel converts a string level to a LogLevel
func ParseLevel(level string) LogLevel {
	switch strings.ToUpper(level) {
	case "DEBUG":
		return LevelDebug
	case "INFO":
		return LevelInfo
	case "WARN":
		return LevelWarn
	case "ERROR":
		return LevelError
	case "FATAL":
		return LevelFatal
	default:
		return LevelInfo
	}
}

// StdLogger returns a standard log.Logger instance that uses this logger
func (l *Logger) StdLogger() *log.Logger {
	return l.stdLogger
}
