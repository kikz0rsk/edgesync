package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
)

const (
	exitPass  = 0
	exitFail  = 1
	exitError = 2

	defaultFuzzTime = 30 * time.Second
)

var (
	errNoPatterns = errors.New("at least one package pattern is required")
	fuzzName      = regexp.MustCompile(`^Fuzz[^a-z]\w*$`)
)

type goRunner func(ctx context.Context, args []string, stdout, stderr io.Writer) error

type options struct {
	fuzzTime time.Duration
	patterns []string
}

type target struct {
	pkg  string
	name string
}

func run(ctx context.Context, args []string, goCmd goRunner, stdout, stderr io.Writer) int {
	opts, err := parseArgs(args, stderr)
	if errors.Is(err, flag.ErrHelp) {
		return exitPass
	}
	if err != nil {
		fmt.Fprintf(stderr, "fuzzsmoke: %v\n", err)
		return exitError
	}
	targets, err := discover(ctx, opts.patterns, goCmd, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "fuzzsmoke: %v\n", err)
		return exitFail
	}
	if len(targets) == 0 {
		fmt.Fprintln(stdout, "no fuzz targets")
		return exitPass
	}
	for _, argv := range commandsFor(targets, opts.fuzzTime) {
		fmt.Fprintf(stdout, "fuzzsmoke: go %s\n", strings.Join(argv, " "))
		if err := goCmd(ctx, argv, stdout, stderr); err != nil {
			fmt.Fprintf(stderr, "fuzzsmoke: %v\n", err)
			return exitFail
		}
	}
	return exitPass
}

func parseArgs(args []string, stderr io.Writer) (options, error) {
	var o options
	fs := flag.NewFlagSet("fuzzsmoke", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.DurationVar(&o.fuzzTime, "fuzztime", defaultFuzzTime, "fuzzing duration for each fuzz target")
	if err := fs.Parse(args); err != nil {
		return options{}, fmt.Errorf("parse flags: %w", err)
	}
	if o.fuzzTime <= 0 {
		return options{}, fmt.Errorf("-fuzztime must be positive, got %s", o.fuzzTime)
	}
	if fs.NArg() == 0 {
		return options{}, errNoPatterns
	}
	o.patterns = fs.Args()
	return o, nil
}

func discover(ctx context.Context, patterns []string, goCmd goRunner, stderr io.Writer) ([]target, error) {
	var listed bytes.Buffer
	if err := goCmd(ctx, append([]string{"list"}, patterns...), &listed, stderr); err != nil {
		return nil, fmt.Errorf("list packages: %w", err)
	}
	var targets []target
	for _, pkg := range strings.Fields(listed.String()) {
		var out bytes.Buffer
		if err := goCmd(ctx, []string{"test", "-list", "^Fuzz", pkg}, &out, stderr); err != nil {
			return nil, fmt.Errorf("list fuzz targets of %s: %w", pkg, err)
		}
		for _, name := range parseListOutput(out.String()) {
			targets = append(targets, target{pkg: pkg, name: name})
		}
	}
	return targets, nil
}

func parseListOutput(out string) []string {
	var names []string
	for _, line := range strings.Split(out, "\n") {
		name := strings.TrimSpace(line)
		if fuzzName.MatchString(name) {
			names = append(names, name)
		}
	}
	return names
}

func commandsFor(targets []target, fuzzTime time.Duration) [][]string {
	cmds := make([][]string, 0, len(targets))
	for _, t := range targets {
		cmds = append(cmds, []string{
			"test", "-run", "^$", "-fuzz", "^" + t.name + "$", "-fuzztime", fuzzTime.String(), t.pkg,
		})
	}
	return cmds
}
