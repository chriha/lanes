package scripts

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// benchOutput is `go test -bench` output with one BenchmarkTimeToFirstList line per ns/op value.
func benchOutput(nsPerOp ...float64) string {
	var b strings.Builder
	b.WriteString("goos: linux\ngoarch: amd64\npkg: github.com/chriha/lanes/internal/ui\n")
	for _, ns := range nsPerOp {
		fmt.Fprintf(&b, "BenchmarkTimeToFirstList-4   \t     270\t   %.0f ns/op\n", ns)
	}
	b.WriteString("PASS\nok  \tgithub.com/chriha/lanes/internal/ui\t4.413s\n")
	return b.String()
}

// benchCheck writes base and head (base "" with noBase = no base file at all) and runs bench-check.sh on them.
func benchCheck(t *testing.T, base, head string, noBase bool) (string, bool) {
	t.Helper()
	dir := t.TempDir()
	basePath, headPath := filepath.Join(dir, "base.txt"), filepath.Join(dir, "head.txt")
	if !noBase {
		if err := os.WriteFile(basePath, []byte(base), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(headPath, []byte(head), 0o644); err != nil {
		t.Fatal(err)
	}
	script, err := filepath.Abs("bench-check.sh")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(script, basePath, headPath)
	cmd.Dir, cmd.Env = dir, []string{"PATH=" + os.Getenv("PATH")}
	out, err := cmd.CombinedOutput()
	return string(out), err == nil
}

func TestBenchCheck(t *testing.T) {
	tests := []struct {
		name   string
		base   string
		noBase bool
		head   string
		ok     bool
		want   []string // all in the output
	}{
		{"1.9x passes", benchOutput(1e6, 1e6, 1e6), false, benchOutput(1.9e6, 1.9e6, 1.9e6), true,
			[]string{"1000000", "1900000", "1.90x"}},
		{"exactly 2x passes", benchOutput(1e6), false, benchOutput(2e6), true, []string{"2.00x"}},
		{"2.1x fails", benchOutput(1e6, 1e6, 1e6), false, benchOutput(2.1e6, 2.1e6, 2.1e6), false,
			[]string{"1000000", "2100000", "2.10x"}},
		{"faster head passes", benchOutput(2e6), false, benchOutput(1e6), true, []string{"0.50x"}},
		{"median of an odd count ignores an outlier", benchOutput(1e6, 1e6, 9e9), false, benchOutput(1.5e6, 1.5e6, 1.5e6), true,
			[]string{"| base | 1000000 | 3 |", "1.50x"}},
		{"median of an even count averages the middle two", benchOutput(4e6, 1e6, 3e6, 2e6), false, benchOutput(9e6, 1e6, 6e6, 4e6), true,
			[]string{"| base | 2500000 | 4 |", "| head | 5000000 | 4 |", "2.00x"}},
		{"head outlier alone does not fail", benchOutput(1e6, 1e6, 1e6, 1e6, 1e6, 1e6), false,
			benchOutput(1e6, 1e6, 1e6, 1e6, 9e6, 9e6), true, []string{"1.00x"}},
		{"base without results reports only", "# github.com/chriha/lanes/internal/ui [build failed]\nFAIL\n", false,
			benchOutput(9e9), true, []string{"no comparison"}},
		{"no base file reports only", "", true, benchOutput(9e9), true, []string{"no comparison"}},
		{"head without results fails", benchOutput(1e6), false, "FAIL\n", false, []string{"no BenchmarkTimeToFirstList result"}},
		{"other benchmarks are ignored", benchOutput(1e6) + "BenchmarkOther-4 \t 10\t 9000000000 ns/op\n", false,
			benchOutput(1e6), true, []string{"| base | 1000000 | 1 |", "1.00x"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, ok := benchCheck(t, tt.base, tt.head, tt.noBase)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v:\n%s", ok, tt.ok, out)
			}
			for _, w := range tt.want {
				if !strings.Contains(out, w) {
					t.Errorf("output does not contain %q:\n%s", w, out)
				}
			}
		})
	}
}
