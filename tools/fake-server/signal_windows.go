//go:build windows

package main

import "os"

// Windows delivers console Ctrl+C and Ctrl+Break as os.Interrupt; there is no
// separate terminate signal to listen for.
var terminateSignal = os.Interrupt
