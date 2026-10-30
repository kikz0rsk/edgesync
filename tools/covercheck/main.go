// Command covercheck enforces the per-package statement coverage gate over a Go cover profile.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

const (
	exitPass  = 0
	exitFail  = 1
	exitError = 2
)

type options struct {
	profile    string
	exclude    string
	packages   string
	minPercent float64
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	opts, err := parseArgs(args, stderr)
	if errors.Is(err, flag.ErrHelp) {
		return exitPass
	}
	if err != nil {
		fmt.Fprintf(stderr, "covercheck: %v\n", err)
		return exitError
	}
	rep, err := evaluate(opts)
	if err != nil {
		fmt.Fprintf(stderr, "covercheck: %v\n", err)
		return exitError
	}
	if err = writeReport(stdout, rep); err != nil {
		fmt.Fprintf(stderr, "covercheck: %v\n", err)
		return exitError
	}
	if rep.violations > 0 {
		return exitFail
	}
	return exitPass
}

func parseArgs(args []string, stderr io.Writer) (options, error) {
	var o options
	fs := flag.NewFlagSet("covercheck", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&o.profile, "profile", "", "Go cover profile (go test -coverprofile output)")
	fs.Float64Var(&o.minPercent, "min", 85, "minimum statement coverage in percent for every included package")
	fs.StringVar(&o.exclude, "exclude", "", "optional file of module-relative package patterns to skip")
	fs.StringVar(&o.packages, "packages", "", "file listing the import path of every package (go list ./... output)")
	if err := fs.Parse(args); err != nil {
		return options{}, fmt.Errorf("parse flags: %w", err)
	}
	if fs.NArg() > 0 {
		return options{}, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	if o.profile == "" || o.packages == "" {
		return options{}, errors.New("-profile and -packages are required")
	}
	if !(o.minPercent >= 0 && o.minPercent <= 100) {
		return options{}, fmt.Errorf("-min must be between 0 and 100, got %v", o.minPercent)
	}
	return o, nil
}
