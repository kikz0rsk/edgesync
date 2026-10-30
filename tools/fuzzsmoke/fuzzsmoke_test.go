package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"
)

const (
	alphaPkg = "example.com/edgesync/internal/alpha"
	betaPkg  = "example.com/edgesync/internal/beta"
	listPkgs = "./..."
)

type fakeGo struct {
	outputs map[string]string
	fail    string
	calls   []string
}

func (f *fakeGo) run(_ context.Context, args []string, stdout, _ io.Writer) error {
	key := strings.Join(args, " ")
	f.calls = append(f.calls, key)
	if key == f.fail {
		return errors.New("exit status 1")
	}
	_, err := io.WriteString(stdout, f.outputs[key])
	return err
}

func twoPackageFake(fail string) *fakeGo {
	return &fakeGo{
		outputs: map[string]string{
			"list " + listPkgs:             alphaPkg + "\n" + betaPkg + "\n",
			"test -list ^Fuzz " + alphaPkg: "FuzzParse\nok  \t" + alphaPkg + "\t0.01s\n",
			"test -list ^Fuzz " + betaPkg:  "FuzzEncode\nok  \t" + betaPkg + "\t0.01s\n",
		},
		fail: fail,
	}
}

func fuzzKey(name, pkg string) string {
	return "test -run ^$ -fuzz ^" + name + "$ -fuzztime 30s " + pkg
}

func TestParseListOutput_MultiplePackages(t *testing.T) {
	t.Parallel()
	input := "FuzzParse\nok  \t" + alphaPkg + "\t0.011s\nTestPlain\nFuzzEncode_Roundtrip\nok  \t" + betaPkg + "\t0.012s\n"
	want := []string{"FuzzParse", "FuzzEncode_Roundtrip"}
	if got := parseListOutput(input); !reflect.DeepEqual(got, want) {
		t.Fatalf("parseListOutput() = %q, want %q", got, want)
	}
}

func TestParseListOutput_NoFuzzTargets(t *testing.T) {
	t.Parallel()
	input := "ok  \t" + alphaPkg + "\t0.01s\nTestOnly\nFuzz\nfuzzLower\nFuzzing done\n"
	if got := parseListOutput(input); len(got) != 0 {
		t.Fatalf("parseListOutput() = %q, want no targets", got)
	}
}

func TestCommandsFor_TargetsAndDuration(t *testing.T) {
	t.Parallel()
	targets := []target{{pkg: alphaPkg, name: "FuzzParse"}, {pkg: betaPkg, name: "FuzzEncode"}}
	want := [][]string{
		{"test", "-run", "^$", "-fuzz", "^FuzzParse$", "-fuzztime", "45s", alphaPkg},
		{"test", "-run", "^$", "-fuzz", "^FuzzEncode$", "-fuzztime", "45s", betaPkg},
	}
	if got := commandsFor(targets, 45*time.Second); !reflect.DeepEqual(got, want) {
		t.Fatalf("commandsFor() = %q, want %q", got, want)
	}
	if got := commandsFor(nil, time.Second); len(got) != 0 {
		t.Fatalf("commandsFor(nil) = %q, want none", got)
	}
}

func TestParseArgs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		args      []string
		want      options
		wantErr   bool
		wantErrIs error
	}{
		{name: "defaults", args: []string{"./..."}, want: options{fuzzTime: 30 * time.Second, patterns: []string{"./..."}}},
		{name: "custom duration", args: []string{"-fuzztime", "5s", "./a", "./b"}, want: options{fuzzTime: 5 * time.Second, patterns: []string{"./a", "./b"}}},
		{name: "missing patterns", args: []string{"-fuzztime", "5s"}, wantErr: true, wantErrIs: errNoPatterns},
		{name: "non-positive duration", args: []string{"-fuzztime", "0s", "./..."}, wantErr: true},
		{name: "unknown flag", args: []string{"-bogus", "./..."}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseArgs(tt.args, io.Discard)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseArgs() = %+v, want error", got)
				}
				if tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs) {
					t.Fatalf("parseArgs() error = %v, want %v", err, tt.wantErrIs)
				}
				return
			}
			if err != nil || !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parseArgs() = %+v, %v; want %+v", got, err, tt.want)
			}
		})
	}
}

func TestRun_HelpExitsZero(t *testing.T) {
	t.Parallel()
	fake := &fakeGo{}
	if code := run(context.Background(), []string{"-h"}, fake.run, io.Discard, io.Discard); code != exitPass {
		t.Fatalf("run(-h) = %d, want %d", code, exitPass)
	}
	if len(fake.calls) != 0 {
		t.Fatalf("run(-h) invoked go: %q", fake.calls)
	}
}

func TestRun_UsageErrorExitsWithTwo(t *testing.T) {
	t.Parallel()
	var stderr bytes.Buffer
	fake := &fakeGo{}
	if code := run(context.Background(), nil, fake.run, io.Discard, &stderr); code != exitError {
		t.Fatalf("run() = %d, want %d", code, exitError)
	}
	if !strings.Contains(stderr.String(), errNoPatterns.Error()) {
		t.Fatalf("stderr = %q, want usage error", stderr.String())
	}
}

func TestRun_NoFuzzTargetsExitsZero(t *testing.T) {
	t.Parallel()
	fake := &fakeGo{outputs: map[string]string{
		"list " + listPkgs:             alphaPkg + "\n",
		"test -list ^Fuzz " + alphaPkg: "ok  \t" + alphaPkg + "\t0.01s\n",
	}}
	var stdout bytes.Buffer
	if code := run(context.Background(), []string{listPkgs}, fake.run, &stdout, io.Discard); code != exitPass {
		t.Fatalf("run() = %d, want %d", code, exitPass)
	}
	if got := stdout.String(); got != "no fuzz targets\n" {
		t.Fatalf("stdout = %q, want %q", got, "no fuzz targets\n")
	}
}

func TestRun_RunsEachTargetInOrder(t *testing.T) {
	t.Parallel()
	fake := twoPackageFake("")
	var stdout bytes.Buffer
	if code := run(context.Background(), []string{listPkgs}, fake.run, &stdout, io.Discard); code != exitPass {
		t.Fatalf("run() = %d, want %d", code, exitPass)
	}
	want := []string{
		"list " + listPkgs,
		"test -list ^Fuzz " + alphaPkg,
		"test -list ^Fuzz " + betaPkg,
		fuzzKey("FuzzParse", alphaPkg),
		fuzzKey("FuzzEncode", betaPkg),
	}
	if !reflect.DeepEqual(fake.calls, want) {
		t.Fatalf("go calls = %q, want %q", fake.calls, want)
	}
}

func TestRun_StopsAtFirstFuzzFailure(t *testing.T) {
	t.Parallel()
	fake := twoPackageFake(fuzzKey("FuzzParse", alphaPkg))
	if code := run(context.Background(), []string{listPkgs}, fake.run, io.Discard, io.Discard); code != exitFail {
		t.Fatalf("run() = %d, want %d", code, exitFail)
	}
	if last := fake.calls[len(fake.calls)-1]; last != fuzzKey("FuzzParse", alphaPkg) {
		t.Fatalf("last go call = %q, want the failing first target", last)
	}
	for _, call := range fake.calls {
		if strings.Contains(call, "FuzzEncode") {
			t.Fatalf("second target ran after a failure: %q", fake.calls)
		}
	}
}

func TestRun_ReportsDiscoveryFailures(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		fail string
	}{
		{name: "package listing", fail: "list " + listPkgs},
		{name: "fuzz target listing", fail: "test -list ^Fuzz " + betaPkg},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fake := twoPackageFake(tt.fail)
			var stderr bytes.Buffer
			if code := run(context.Background(), []string{listPkgs}, fake.run, io.Discard, &stderr); code != exitFail {
				t.Fatalf("run() = %d, want %d", code, exitFail)
			}
			if !strings.Contains(stderr.String(), "exit status 1") {
				t.Fatalf("stderr = %q, want the failing go command", stderr.String())
			}
			for _, call := range fake.calls {
				if strings.HasPrefix(call, "test -run") {
					t.Fatalf("fuzzing started after discovery failed: %q", fake.calls)
				}
			}
		})
	}
}

func TestExecGo_RunsGoAndReportsFailure(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	if err := execGo(context.Background(), []string{"env", "GOOS"}, &stdout, io.Discard); err != nil {
		t.Fatalf("execGo(env GOOS) error = %v", err)
	}
	if strings.TrimSpace(stdout.String()) == "" {
		t.Fatal("execGo(env GOOS) wrote no output")
	}
	if err := execGo(context.Background(), []string{"no-such-subcommand"}, io.Discard, io.Discard); err == nil {
		t.Fatal("execGo(unknown subcommand) returned nil error")
	}
}
