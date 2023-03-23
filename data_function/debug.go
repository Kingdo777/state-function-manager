package data_function

import (
	"fmt"
	"log"
)

// Debugging flag
var Debugging bool

// Debug emits a debug message
func Debug(format string, args ...interface{}) {
	if Debugging {
		log.Printf("%c[1;34m%s%c[0m\n", 0x1B, fmt.Sprintf(format, args...), 0x1B)
	}
}

// Info emits a debug message
func Info(format string, args ...interface{}) {
	if Debugging {
		log.Printf("%c[1;32m%s%c[0m\n", 0x1B, fmt.Sprintf(format, args...), 0x1B)
	}
}

// Warn emits a debug message
func Warn(format string, args ...interface{}) {
	if Debugging {
		log.Printf("%c[1;33m%s%c[0m\n", 0x1B, fmt.Sprintf(format, args...), 0x1B)
	}
}

// Error emits a debug message
func Error(format string, args ...interface{}) {
	if Debugging {
		log.Printf("%c[1;31m%s%c[0m\n", 0x1B, fmt.Sprintf(format, args...), 0x1B)
	}
}

// DebugLimit emits a debug message with a limit in length
func DebugLimit(msg string, in []byte, limit int) {
	if len(in) < limit {
		Debug("%s:%s", msg, in)
	} else {
		Debug("%s:%s...", msg, in[0:limit])
	}
}
