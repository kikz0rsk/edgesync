package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"strings"
	"text/tabwriter"
)

var (
	errMalformedProfile = errors.New("malformed cover profile")
	errNoPackages       = errors.New("package list is empty")
)

type block struct {
	file  string
	span  string
	stmts int
	count int
}

type blockKey struct {
	file string
	span string
}

type pkgCoverage struct {
	stmts   int
	covered int
}

func (c pkgCoverage) percent() float64 {
	return 100 * float64(c.covered) / float64(c.stmts)
}

type row struct {
	pkg     string
	cov     pkgCoverage
	hasData bool
	status  string
}

type report struct {
	rows       []row
	violations int
}

type stickyWriter struct {
	w   io.Writer
	err error
}

func (s *stickyWriter) printf(format string, args ...any) {
	if s.err != nil {
		return
	}
	_, s.err = fmt.Fprintf(s.w, format, args...)
}

func evaluate(o options) (report, error) {
	blocks, err := parseFile(o.profile, parseProfile)
	if err != nil {
		return report{}, err
	}
	coverage, err := perPackageCoverage(blocks)
	if err != nil {
		return report{}, err
	}
	var patterns []string
	if o.exclude != "" {
		if patterns, err = parseFile(o.exclude, readList); err != nil {
			return report{}, err
		}
	}
	packages, err := parseFile(o.packages, readList)
	if err != nil {
		return report{}, err
	}
	if len(packages) == 0 {
		return report{}, errNoPackages
	}
	return check(coverage, packages, patterns, o.minPercent), nil
}

func parseFile[T any](file string, parse func(io.Reader) (T, error)) (T, error) {
	var zero T
	f, err := os.Open(file) //nolint:gosec // the operator names the input files on the command line
	if err != nil {
		return zero, fmt.Errorf("open %s: %w", file, err)
	}
	defer f.Close()
	result, err := parse(f)
	if err != nil {
		return zero, fmt.Errorf("%s: %w", file, err)
	}
	return result, nil
}

func parseProfile(r io.Reader) ([]block, error) {
	scanner := bufio.NewScanner(r)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("read profile: %w", err)
		}
		return nil, fmt.Errorf("%w: empty profile", errMalformedProfile)
	}
	if !isModeLine(scanner.Text()) {
		return nil, fmt.Errorf("%w: line 1 must be \"mode: set|count|atomic\", got %q", errMalformedProfile, scanner.Text())
	}
	var blocks []block
	for lineNo := 2; scanner.Scan(); lineNo++ {
		line := scanner.Text()
		if line == "" {
			continue
		}
		b, err := parseBlock(line)
		if err != nil {
			return nil, fmt.Errorf("%w: line %d: %w", errMalformedProfile, lineNo, err)
		}
		blocks = append(blocks, b)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read profile: %w", err)
	}
	return blocks, nil
}

func isModeLine(line string) bool {
	mode, found := strings.CutPrefix(line, "mode: ")
	return found && (mode == "set" || mode == "count" || mode == "atomic")
}

func parseBlock(line string) (block, error) {
	colon := strings.LastIndex(line, ":")
	if colon < 0 {
		return block{}, fmt.Errorf("no file separator in %q", line)
	}
	fields := strings.Fields(line[colon+1:])
	if len(fields) != 3 || !strings.Contains(fields[0], ",") {
		return block{}, fmt.Errorf("want \"file:span numStmts count\", got %q", line)
	}
	stmts, err := strconv.Atoi(fields[1])
	if err != nil {
		return block{}, fmt.Errorf("statement count in %q: %w", line, err)
	}
	count, err := strconv.Atoi(fields[2])
	if err != nil {
		return block{}, fmt.Errorf("hit count in %q: %w", line, err)
	}
	if stmts < 1 || count < 0 {
		return block{}, fmt.Errorf("statement count must be at least 1 and hit count at least 0 in %q", line)
	}
	return block{file: line[:colon], span: fields[0], stmts: stmts, count: count}, nil
}

func perPackageCoverage(blocks []block) (map[string]pkgCoverage, error) {
	merged := make(map[blockKey]block, len(blocks))
	for _, b := range blocks {
		key := blockKey{file: b.file, span: b.span}
		prev, seen := merged[key]
		if !seen {
			merged[key] = b
			continue
		}
		if prev.stmts != b.stmts {
			return nil, fmt.Errorf("%w: %s:%s has statement counts %d and %d", errMalformedProfile, b.file, b.span, prev.stmts, b.stmts)
		}
		prev.count = max(prev.count, b.count)
		merged[key] = prev
	}
	coverage := make(map[string]pkgCoverage)
	for _, b := range merged {
		pkg := path.Dir(b.file)
		cov := coverage[pkg]
		cov.stmts += b.stmts
		if b.count > 0 {
			cov.covered += b.stmts
		}
		coverage[pkg] = cov
	}
	return coverage, nil
}

func readList(r io.Reader) ([]string, error) {
	var items []string
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		item := strings.TrimSpace(scanner.Text())
		if item == "" || strings.HasPrefix(item, "#") {
			continue
		}
		items = append(items, item)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read list: %w", err)
	}
	return items, nil
}

func excluded(importPath string, patterns []string) bool {
	for _, pattern := range patterns {
		if matchesPattern(importPath, pattern) {
			return true
		}
	}
	return false
}

// Module-relative patterns are matched at path-segment boundaries because the module path is not an input.
func matchesPattern(importPath, pattern string) bool {
	if base, tree := strings.CutSuffix(pattern, "/..."); tree {
		return strings.Contains("/"+importPath+"/", "/"+base+"/")
	}
	return importPath == pattern || strings.HasSuffix(importPath, "/"+pattern)
}

func check(coverage map[string]pkgCoverage, packages, patterns []string, minPercent float64) report {
	var rep report
	for _, pkg := range packages {
		if excluded(pkg, patterns) {
			continue
		}
		cov, found := coverage[pkg]
		switch {
		case !found:
			rep.rows = append(rep.rows, row{pkg: pkg, status: "FAIL (no coverage data)"})
			rep.violations++
		case cov.percent() < minPercent:
			rep.rows = append(rep.rows, row{pkg: pkg, cov: cov, hasData: true, status: "FAIL (below minimum)"})
			rep.violations++
		default:
			rep.rows = append(rep.rows, row{pkg: pkg, cov: cov, hasData: true, status: "PASS"})
		}
	}
	return rep
}

func writeReport(w io.Writer, rep report) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	out := &stickyWriter{w: tw}
	out.printf("package\tstmts\tcovered\tpct\tstatus\n")
	for _, r := range rep.rows {
		if !r.hasData {
			out.printf("%s\t-\t-\t-\t%s\n", r.pkg, r.status)
			continue
		}
		out.printf("%s\t%d\t%d\t%.2f\t%s\n", r.pkg, r.cov.stmts, r.cov.covered, r.cov.percent(), r.status)
	}
	if rep.violations == 0 {
		out.printf("PASS\n")
	} else {
		out.printf("FAIL (%d violations)\n", rep.violations)
	}
	if out.err != nil {
		return out.err
	}
	if err := tw.Flush(); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	return nil
}
