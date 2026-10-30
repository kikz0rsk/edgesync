//go:build !windows

package main

import "os"

func main() {
	os.Stderr.WriteString("windows only\n")
	os.Exit(2)
}
