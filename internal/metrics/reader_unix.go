//go:build !windows

package metrics

import "os"

func newReader() reader { return procfsReader{root: "/proc", pageSize: int64(os.Getpagesize())} }
