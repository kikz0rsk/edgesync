package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/iotest"
)

const (
	alphaPkg = "github.com/kikz0r/edgesync/internal/alpha"
	betaPkg  = "github.com/kikz0r/edgesync/internal/beta"
)

func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "input.txt")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func loadFixture(t *testing.T, name string) map[string]pkgCoverage {
	t.Helper()
	blocks, err := parseFile(filepath.Join("testdata", name), parseProfile)
	if err != nil {
		t.Fatalf("parse fixture %s: %v", name, err)
	}
	coverage, err := perPackageCoverage(blocks)
	if err != nil {
		t.Fatalf("coverage for fixture %s: %v", name, err)
	}
	return coverage
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("sink closed")
}

func TestParseProfile_SetAndAtomicModes(t *testing.T) {
	t.Parallel()
	const file = alphaPkg + "/alpha.go"
	tests := []struct {
		name  string
		input string
		want  []block
	}{
		{
			name:  "set",
			input: "mode: set\n" + file + ":3.2,5.3 2 1\n" + file + ":7.2,9.3 2 0\n",
			want: []block{
				{file: file, span: "3.2,5.3", stmts: 2, count: 1},
				{file: file, span: "7.2,9.3", stmts: 2, count: 0},
			},
		},
		{
			name:  "atomic",
			input: "mode: atomic\n" + file + ":3.2,5.3 4 7\n",
			want:  []block{{file: file, span: "3.2,5.3", stmts: 4, count: 7}},
		},
		{
			name:  "count",
			input: "mode: count\n" + file + ":3.2,5.3 1 3\n",
			want:  []block{{file: file, span: "3.2,5.3", stmts: 1, count: 3}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseProfile(strings.NewReader(tc.input))
			if err != nil {
				t.Fatalf("parseProfile() error = %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("parseProfile() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestParseProfile_RejectsMalformedInput(t *testing.T) {
	t.Parallel()
	const file = alphaPkg + "/alpha.go"
	tests := []struct {
		name  string
		input string
	}{
		{name: "empty", input: ""},
		{name: "unknown mode", input: "mode: bogus\n"},
		{name: "missing mode", input: file + ":3.2,5.3 2 1\n"},
		{name: "no file separator", input: "mode: set\nnocolon 2 1\n"},
		{name: "too few fields", input: "mode: set\n" + file + ":3.2,5.3 2\n"},
		{name: "span without comma", input: "mode: set\n" + file + ":32 2 1\n"},
		{name: "non-numeric statements", input: "mode: set\n" + file + ":3.2,5.3 x 1\n"},
		{name: "non-numeric count", input: "mode: set\n" + file + ":3.2,5.3 2 y\n"},
		{name: "zero statements", input: "mode: set\n" + file + ":3.2,5.3 0 1\n"},
		{name: "negative count", input: "mode: set\n" + file + ":3.2,5.3 2 -1\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := parseProfile(strings.NewReader(tc.input))
			if !errors.Is(err, errMalformedProfile) {
				t.Errorf("parseProfile() error = %v, want %v", err, errMalformedProfile)
			}
		})
	}
}

func TestParseProfile_ReportsReadFailure(t *testing.T) {
	t.Parallel()
	_, err := parseProfile(iotest.ErrReader(errors.New("disk gone")))
	if err == nil || errors.Is(err, errMalformedProfile) {
		t.Errorf("parseProfile() error = %v, want a read error", err)
	}
}

func TestPerPackageCoverage_MergesDuplicateBlocks(t *testing.T) {
	t.Parallel()
	const pkg = "example.com/m/p"
	blocks := []block{
		{file: pkg + "/a.go", span: "1.1,2.2", stmts: 3, count: 0},
		{file: pkg + "/a.go", span: "1.1,2.2", stmts: 3, count: 2},
		{file: pkg + "/a.go", span: "4.1,5.2", stmts: 2, count: 0},
		{file: "example.com/m/q/b.go", span: "1.1,2.2", stmts: 1, count: 0},
	}
	got, err := perPackageCoverage(blocks)
	if err != nil {
		t.Fatalf("perPackageCoverage() error = %v", err)
	}
	want := map[string]pkgCoverage{
		pkg:               {stmts: 5, covered: 3},
		"example.com/m/q": {stmts: 1, covered: 0},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("perPackageCoverage() = %+v, want %+v", got, want)
	}
}

func TestPerPackageCoverage_RejectsConflictingStatementCounts(t *testing.T) {
	t.Parallel()
	blocks := []block{
		{file: "example.com/m/p/a.go", span: "1.1,2.2", stmts: 3, count: 1},
		{file: "example.com/m/p/a.go", span: "1.1,2.2", stmts: 4, count: 1},
	}
	_, err := perPackageCoverage(blocks)
	if !errors.Is(err, errMalformedProfile) {
		t.Errorf("perPackageCoverage() error = %v, want %v", err, errMalformedProfile)
	}
}

func TestCheck_FailsBelowMinimum(t *testing.T) {
	t.Parallel()
	coverage := loadFixture(t, "low.out")
	rep := check(coverage, []string{alphaPkg, betaPkg}, nil, 85)
	if rep.violations != 1 {
		t.Fatalf("check() violations = %d, want 1", rep.violations)
	}
	last := rep.rows[len(rep.rows)-1]
	if last.pkg != betaPkg || last.status != "FAIL (below minimum)" {
		t.Errorf("check() beta row = %+v, want below-minimum failure", last)
	}
}

func TestCheck_FailsWhenIncludedPackageHasNoProfile(t *testing.T) {
	t.Parallel()
	coverage := loadFixture(t, "notests.out")
	rep := check(coverage, []string{alphaPkg, betaPkg}, nil, 85)
	if rep.violations != 1 {
		t.Fatalf("check() violations = %d, want 1", rep.violations)
	}
	last := rep.rows[len(rep.rows)-1]
	if last.pkg != betaPkg || last.hasData || last.status != "FAIL (no coverage data)" {
		t.Errorf("check() beta row = %+v, want no-coverage-data failure", last)
	}
}

func TestCheck_ExcludedPatternsIgnored(t *testing.T) {
	t.Parallel()
	const windowsPkg = "github.com/kikz0r/edgesync/internal/platform/windows"
	coverage := loadFixture(t, "ok.out")
	packages := []string{alphaPkg, betaPkg, windowsPkg}

	if rep := check(coverage, packages, nil, 85); rep.violations != 1 {
		t.Fatalf("check() without exclusions: violations = %d, want 1", rep.violations)
	}
	rep := check(coverage, packages, []string{"internal/platform/windows/..."}, 85)
	if rep.violations != 0 {
		t.Errorf("check() with exclusion: violations = %d, want 0", rep.violations)
	}
	if len(rep.rows) != 2 {
		t.Errorf("check() with exclusion: rows = %d, want 2", len(rep.rows))
	}
}

func TestMatchesPattern(t *testing.T) {
	t.Parallel()
	const windows = "github.com/kikz0r/edgesync/internal/platform/windows"
	tests := []struct {
		name       string
		importPath string
		pattern    string
		want       bool
	}{
		{name: "tree matches package itself", importPath: windows, pattern: "internal/platform/windows/...", want: true},
		{name: "tree matches subpackage", importPath: windows + "/sub", pattern: "internal/platform/windows/...", want: true},
		{name: "tree rejects longer sibling", importPath: windows + "x", pattern: "internal/platform/windows/...", want: false},
		{name: "exact matches package", importPath: windows, pattern: "internal/platform/windows", want: true},
		{name: "exact rejects subpackage", importPath: windows + "/sub", pattern: "internal/platform/windows", want: false},
		{name: "exact matches full path", importPath: "windows", pattern: "windows", want: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := matchesPattern(tc.importPath, tc.pattern); got != tc.want {
				t.Errorf("matchesPattern(%q, %q) = %v, want %v", tc.importPath, tc.pattern, got, tc.want)
			}
		})
	}
}

func TestReadList_SkipsCommentsAndBlankLines(t *testing.T) {
	t.Parallel()
	got, err := readList(strings.NewReader("# header\n\n  a/b  \nc\n# trailing\n"))
	if err != nil {
		t.Fatalf("readList() error = %v", err)
	}
	if want := []string{"a/b", "c"}; !reflect.DeepEqual(got, want) {
		t.Errorf("readList() = %q, want %q", got, want)
	}
}

func TestReadList_ReportsReadFailure(t *testing.T) {
	t.Parallel()
	if _, err := readList(iotest.ErrReader(errors.New("disk gone"))); err == nil {
		t.Error("readList() error = nil, want a read error")
	}
}

func TestWriteReport_PrintsTableAndVerdict(t *testing.T) {
	t.Parallel()
	rep := report{
		rows: []row{
			{pkg: alphaPkg, cov: pkgCoverage{stmts: 4, covered: 4}, hasData: true, status: "PASS"},
			{pkg: betaPkg, status: "FAIL (no coverage data)"},
		},
		violations: 1,
	}
	var buf bytes.Buffer
	if err := writeReport(&buf, rep); err != nil {
		t.Fatalf("writeReport() error = %v", err)
	}
	out := buf.String()
	for _, want := range []string{"package", alphaPkg, "100.00", "PASS", betaPkg, "FAIL (no coverage data)", "FAIL (1 violations)"} {
		if !strings.Contains(out, want) {
			t.Errorf("writeReport() output lacks %q:\n%s", want, out)
		}
	}
}

func TestWriteReport_ReturnsWriterError(t *testing.T) {
	t.Parallel()
	if err := writeReport(failingWriter{}, report{}); err == nil {
		t.Error("writeReport() error = nil, want the writer error")
	}
}

func TestCLI_ExitCodes(t *testing.T) {
	t.Parallel()
	packages := writeTempFile(t, alphaPkg+"\n"+betaPkg+"\n")
	emptyPackages := writeTempFile(t, "# no packages\n")
	badExclude := filepath.Join(t.TempDir(), "absent-exclude.txt")
	okProfile := filepath.Join("testdata", "ok.out")
	lowProfile := filepath.Join("testdata", "low.out")
	noTestsProfile := filepath.Join("testdata", "notests.out")
	absentProfile := filepath.Join(t.TempDir(), "absent.out")

	tests := []struct {
		name       string
		args       []string
		want       int
		wantStdout string
	}{
		{name: "help", args: []string{"-h"}, want: exitPass},
		{name: "required flags missing", args: nil, want: exitError},
		{name: "unknown flag", args: []string{"-bogus"}, want: exitError},
		{name: "positional argument", args: []string{"-profile", okProfile, "-packages", packages, "extra"}, want: exitError},
		{name: "min above 100", args: []string{"-profile", okProfile, "-packages", packages, "-min", "101"}, want: exitError},
		{name: "min is NaN", args: []string{"-profile", okProfile, "-packages", packages, "-min", "NaN"}, want: exitError},
		{name: "profile file absent", args: []string{"-profile", absentProfile, "-packages", packages}, want: exitError},
		{name: "exclude file absent", args: []string{"-profile", okProfile, "-packages", packages, "-exclude", badExclude}, want: exitError},
		{name: "package list empty", args: []string{"-profile", okProfile, "-packages", emptyPackages}, want: exitError},
		{name: "all packages pass", args: []string{"-profile", okProfile, "-packages", packages}, want: exitPass, wantStdout: "PASS"},
		{name: "below minimum", args: []string{"-profile", lowProfile, "-packages", packages}, want: exitFail, wantStdout: "FAIL (1 violations)"},
		{name: "package without profile", args: []string{"-profile", noTestsProfile, "-packages", packages}, want: exitFail, wantStdout: "FAIL (1 violations)"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			got := run(tc.args, &stdout, &stderr)
			if got != tc.want {
				t.Errorf("run(%q) = %d, want %d; stderr: %s", tc.args, got, tc.want, stderr.String())
			}
			if tc.wantStdout != "" && !strings.Contains(stdout.String(), tc.wantStdout) {
				t.Errorf("run(%q) stdout lacks %q:\n%s", tc.args, tc.wantStdout, stdout.String())
			}
		})
	}
}
