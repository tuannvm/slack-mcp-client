package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"sync"
)

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorPurple = "\033[35m"
	colorCyan   = "\033[36m"
	colorGray   = "\033[37m"
)

// LogLevel represents the severity of a log message
type LogLevel int

// Log levels
const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
)

// LoggerOptions holds configuration for the logger
type LoggerOptions struct {
	EnableColors bool
	MinLevel     LogLevel
}

// DefaultOptions returns the default logger options
func DefaultOptions() LoggerOptions {
	return LoggerOptions{
		EnableColors: true, // Enable colors by default
		MinLevel:     LevelInfo,
	}
}

// String returns the string representation of the log level
func (l LogLevel) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	case LevelFatal:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// Color returns the color code for the log level
func (l LogLevel) Color() string {
	switch l {
	case LevelDebug:
		return colorGray
	case LevelInfo:
		return colorGreen
	case LevelWarn:
		return colorYellow
	case LevelError:
		return colorRed
	case LevelFatal:
		return colorPurple
	default:
		return colorReset
	}
}

// Logger is a custom logger implementation that supports filtering by level
type Logger struct {
	mu           sync.Mutex
	stdLogger    *log.Logger
	prefix       string
	flags        int
	writer       io.Writer
	level        LogLevel
	enableColors bool
}

// New creates a new Logger with the specified prefix, flags, and minimum log level
func New(out io.Writer, prefix string, flags int, minLevel LogLevel) *Logger {
	return &Logger{
		stdLogger:    log.New(out, prefix, flags),
		prefix:       prefix,
		flags:        flags,
		writer:       out,
		level:        minLevel,
		enableColors: true, // Enable colors by default
	}
}

// NewWithOptions creates a new Logger with the specified options
func NewWithOptions(out io.Writer, prefix string, flags int, options LoggerOptions) *Logger {
	return &Logger{
		stdLogger:    log.New(out, prefix, flags),
		prefix:       prefix,
		flags:        flags,
		writer:       out,
		level:        options.MinLevel,
		enableColors: options.EnableColors,
	}
}

// NewDefault creates a new Logger with default settings
func NewDefault() *Logger {
	return New(os.Stdout, "", log.LstdFlags, LevelInfo)
}

// SetLevel sets the minimum log level
func (l *Logger) SetLevel(level LogLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// GetLevel returns the current minimum log level
func (l *Logger) GetLevel() LogLevel {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.level
}

// EnableColors enables or disables colorized output
func (l *Logger) EnableColors(enable bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.enableColors = enable
}

// SetOutput sets the output destination
func (l *Logger) SetOutput(w io.Writer) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.writer = w
	l.stdLogger.SetOutput(w)
}

// SetFlags sets the output flags
func (l *Logger) SetFlags(flags int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.flags = flags
	l.stdLogger.SetFlags(flags)
}

// SetPrefix sets the output prefix
func (l *Logger) SetPrefix(prefix string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.prefix = prefix
	l.stdLogger.SetPrefix(prefix)
}

// GetStandardLogger returns the underlying standard logger
func (l *Logger) GetStandardLogger() *log.Logger {
	return l.stdLogger
}

// log logs a message at the specified level if the level is greater than or equal to the logger's level
func (l *Logger) log(level LogLevel, format string, v ...interface{}) {
	if level < l.level {
		return
	}

	// Format the message with level prefix
	var levelPrefix string
	if l.enableColors {
		levelPrefix = fmt.Sprintf("%s[%s]%s ", level.Color(), level.String(), colorReset)
	} else {
		levelPrefix = fmt.Sprintf("[%s] ", level.String())
	}
	
	msg := fmt.Sprintf(format, v...)
	
	l.mu.Lock()
	defer l.mu.Unlock()
	l.stdLogger.Output(2, levelPrefix+msg)
	
	// Fatal messages should terminate the program
	if level == LevelFatal {
		os.Exit(1)
	}
}

// Debug logs a debug message
func (l *Logger) Debug(format string, v ...interface{}) {
	l.log(LevelDebug, format, v...)
}

// Info logs an info message
func (l *Logger) Info(format string, v ...interface{}) {
	l.log(LevelInfo, format, v...)
}

// Warn logs a warning message
func (l *Logger) Warn(format string, v ...interface{}) {
	l.log(LevelWarn, format, v...)
}

// Error logs an error message
func (l *Logger) Error(format string, v ...interface{}) {
	l.log(LevelError, format, v...)
}

// Fatal logs a fatal message and exits the program
func (l *Logger) Fatal(format string, v ...interface{}) {
	l.log(LevelFatal, format, v...)
}

// Printf is provided for compatibility with the standard logger interface
func (l *Logger) Printf(format string, v ...interface{}) {
	l.Info(format, v...)
}

// Println is provided for compatibility with the standard logger interface
func (l *Logger) Println(v ...interface{}) {
	l.Info("%s", fmt.Sprintln(v...))
}

// Print is provided for compatibility with the standard logger interface
func (l *Logger) Print(v ...interface{}) {
	l.Info("%s", fmt.Sprint(v...))
}

// Fatalf is provided for compatibility with the standard logger interface
func (l *Logger) Fatalf(format string, v ...interface{}) {
	l.Fatal(format, v...)
}

// Fatalln is provided for compatibility with the standard logger interface
func (l *Logger) Fatalln(v ...interface{}) {
	l.Fatal("%s", fmt.Sprintln(v...))
}

// WithComponent creates a new logger with a component-specific prefix
func (l *Logger) WithComponent(component string) *Logger {
	newPrefix := fmt.Sprintf("%s[%s] ", l.prefix, component)
	childLogger := &Logger{
		stdLogger:    log.New(l.writer, newPrefix, l.flags),
		prefix:       newPrefix,
		flags:        l.flags,
		writer:       l.writer,
		level:        l.level,
		enableColors: l.enableColors,
	}
	return childLogger
} 