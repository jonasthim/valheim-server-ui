//go:build !windows

package main

import "syscall"

var terminateSignal = syscall.SIGTERM
