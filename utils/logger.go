package utils

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// LogLevel represents the severity level of a log message
type LogLevel int

const (
	// DEBUG level for detailed troubleshooting information
	DEBUG LogLevel = iota
	// INFO level for general operational information
	INFO
	// WARN level for potentially harmful situations
	WARN
	// ERROR level for error events that might still allow the application to continue
	ERROR
	// FATAL level for severe error events that will lead the application to abort
	FATAL
)

var levelNames = map[LogLevel]string{
	DEBUG: "DEBUG",
	INFO:  "INFO",
	WARN:  "WARN",
	ERROR: "ERROR",
	FATAL: "FATAL",
}

// Logger is a custom logger with levels and file output
type Logger struct {
	level      LogLevel
	logger     *log.Logger
	file       *os.File
	mu         sync.Mutex
	timeFormat string
}

var (
	defaultLogger *Logger
	once          sync.Once
)

// InitLogger initializes the default logger
func InitLogger(level LogLevel, logDir string, timeFormat string) error {
	var err error
	once.Do(func() {
		err = initDefaultLogger(level, logDir, timeFormat)
	})
	return err
}

// initDefaultLogger creates and initializes the default logger
func initDefaultLogger(level LogLevel, logDir string, timeFormat string) error {
	// Create log directory if it doesn't exist
	if err := os.MkdirAll(logDir, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	// Create log file with current date
	logFile := filepath.Join(logDir, fmt.Sprintf("app_%s.log", time.Now().Format("20060102")))
	file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	// Create multi-writer to write to both file and stdout
	multiWriter := io.MultiWriter(file, os.Stdout)

	// Create logger
	logger := log.New(multiWriter, "", 0) // No prefix or flags, we'll format manually

	// Set default logger
	defaultLogger = &Logger{
		level:      level,
		logger:     logger,
		file:       file,
		timeFormat: timeFormat,
	}

	// Log initialization
	defaultLogger.Info("Logger initialized with level: %s", levelNames[level])
	return nil
}

// GetLogger returns the default logger
func GetLogger() *Logger {
	if defaultLogger == nil {
		// If logger is not initialized, create a basic console logger
		defaultLogger = &Logger{
			level:      INFO,
			logger:     log.New(os.Stdout, "", 0),
			timeFormat: "2006-01-02 15:04:05.000",
		}
		defaultLogger.Warn("Using default console logger. Call InitLogger() for proper initialization.")
	}
	return defaultLogger
}

// SetLevel sets the logging level
func (l *Logger) SetLevel(level LogLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
	l.Info("Log level set to: %s", levelNames[level])
}

// log logs a message with the specified level
func (l *Logger) log(level LogLevel, format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if level < l.level {
		return
	}

	// Get caller information
	_, file, line, ok := runtime.Caller(2)
	if !ok {
		file = "unknown"
		line = 0
	}
	// Extract just the filename
	file = filepath.Base(file)

	// Format the message
	msg := fmt.Sprintf(format, args...)
	timestamp := time.Now().Format(l.timeFormat)
	logMsg := fmt.Sprintf("[%s] [%s] [%s:%d] %s", timestamp, levelNames[level], file, line, msg)

	// Log the message
	l.logger.Println(logMsg)

	// If fatal, exit the program
	if level == FATAL {
		if l.file != nil {
			l.file.Close()
		}
		os.Exit(1)
	}
}

// Debug logs a debug message
func (l *Logger) Debug(format string, args ...interface{}) {
	l.log(DEBUG, format, args...)
}

// Info logs an info message
func (l *Logger) Info(format string, args ...interface{}) {
	l.log(INFO, format, args...)
}

// Warn logs a warning message
func (l *Logger) Warn(format string, args ...interface{}) {
	l.log(WARN, format, args...)
}

// Error logs an error message
func (l *Logger) Error(format string, args ...interface{}) {
	l.log(ERROR, format, args...)
}

// Fatal logs a fatal message and exits the program
func (l *Logger) Fatal(format string, args ...interface{}) {
	l.log(FATAL, format, args...)
}

// Close closes the log file
func (l *Logger) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		l.file.Close()
		l.file = nil
	}
}

// FormatDuration formats a duration in a human-readable format
func FormatDuration(d time.Duration) string {
	if d < time.Millisecond {
		return fmt.Sprintf("%d µs", d.Microseconds())
	} else if d < time.Second {
		return fmt.Sprintf("%.2f ms", float64(d.Microseconds())/1000)
	} else if d < time.Minute {
		return fmt.Sprintf("%.2f s", d.Seconds())
	} else {
		return fmt.Sprintf("%dm %.2fs", int(d.Minutes()), d.Seconds()-float64(int(d.Minutes())*60))
	}
}

// LogExecutionTime logs the execution time of a function
func LogExecutionTime(functionName string) func() {
	start := time.Now()
	logger := GetLogger()
	logger.Debug("Starting %s", functionName)
	return func() {
		duration := time.Since(start)
		logger.Debug("Completed %s in %s", functionName, FormatDuration(duration))
	}
}

// LogRequest logs an API request
func LogRequest(method, path string, params map[string]string) {
	var paramStr strings.Builder
	for k, v := range params {
		if paramStr.Len() > 0 {
			paramStr.WriteString(", ")
		}
		paramStr.WriteString(fmt.Sprintf("%s: %s", k, v))
	}
	GetLogger().Info("API Request: %s %s [%s]", method, path, paramStr.String())
}

// LogResponse logs an API response
func LogResponse(path string, statusCode int, duration time.Duration) {
	GetLogger().Info("API Response: %s [%d] in %s", path, statusCode, FormatDuration(duration))
}
