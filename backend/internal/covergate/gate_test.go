// Package covergate locks the total-statement coverage floor. It is a
// separate package (no integration TestMain) so the gate runs fast and only
// needs the coverprofile produced by a full test run.
//
// Activation: GO_COVERAGE_PROFILE=<coverprofile> go test ./internal/covergate
// Plain `go test ./...` skips the gate — there is nothing to measure until a
// profile exists. The Makefile `gate`/`cover` targets wire this up.
package covergate

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
)

// CoverageGatePct is the locked floor: the G2 verification gate
// (evidence/verification.json) requires at least 80% total coverage.
const CoverageGatePct = 80.0

// coverTotals sums a "go test -coverprofile" file (mode: set).
func coverTotals(t *testing.T, path string) (total, covered int64) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open coverprofile %q: %v (run the full suite with -coverprofile first)", path, err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	line := 0
	for sc.Scan() {
		line++
		text := sc.Text()
		if line == 1 {
			if !strings.HasPrefix(text, "mode:") {
				t.Fatalf("%q is not a coverprofile (missing mode line)", path)
			}
			continue
		}
		// name.go:line.col,line.col stmts count
		fields := strings.Fields(text)
		if len(fields) != 3 {
			t.Fatalf("%q line %d: malformed entry %q", path, line, text)
		}
		stmts, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			t.Fatalf("%q line %d: bad stmt count: %v", path, line, err)
		}
		count, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil {
			t.Fatalf("%q line %d: bad count: %v", path, line, err)
		}
		total += stmts
		if count > 0 {
			covered += stmts
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("read coverprofile: %v", err)
	}
	if total == 0 {
		t.Fatalf("%q contains no statements", path)
	}
	return total, covered
}

// resolveProfile finds the coverprofile named by env. The test binary runs in
// the package dir, so a repo-root-relative path only resolves after walking
// up (backend/internal/covergate -> repo root is three levels).
func resolveProfile(name string) string {
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(name); err == nil {
			return name
		}
		name = "../" + name
	}
	return os.Getenv("GO_COVERAGE_PROFILE")
}

func TestCoverageGate(t *testing.T) {
	profile := os.Getenv("GO_COVERAGE_PROFILE")
	if profile == "" {
		t.Skip("GO_COVERAGE_PROFILE not set; activate via `make gate` after a full cover run")
	}
	profile = resolveProfile(profile)
	total, covered := coverTotals(t, profile)
	pct := float64(covered) * 100 / float64(total)
	t.Logf("total statement coverage: %.1f%% (%d/%d statements)", pct, covered, total)
	if pct < CoverageGatePct {
		t.Fatalf(fmt.Sprintf("coverage gate FAILED: %.1f%% < %.1f%% — add tests before merging", pct, CoverageGatePct))
	}
}
