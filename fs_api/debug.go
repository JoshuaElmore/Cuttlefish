//go:build debug

package main

import "log"

func debugLog(format string, args ...any) {
	log.Printf(format, args...)
}
