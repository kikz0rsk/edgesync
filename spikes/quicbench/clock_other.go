//go:build !windows

package quicbench

import "time"

var monotonicOrigin = time.Now()

func counterHz() (int64, error) { return int64(time.Second), nil }

func rawCounter() int64 { return int64(time.Since(monotonicOrigin)) }
