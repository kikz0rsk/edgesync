// Command fuzzsmoke runs every fuzz target of the selected packages for a fixed duration each.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := run(ctx, os.Args[1:], execGo, os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

func execGo(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, "go", args...) //nolint:gosec // fixed binary; arguments are this tool's own flags and go list output
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go %s: %w", strings.Join(args, " "), err)
	}
	return nil
}
