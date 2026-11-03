package session

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestParseCPUTime(t *testing.T) {
	tests := []struct {
		in   string
		want time.Duration
		ok   bool
	}{
		{"00:00:00", 0, true},
		{"01:02:03", time.Hour + 2*time.Minute + 3*time.Second, true},
		{"2-03:04:05", 51*time.Hour + 4*time.Minute + 5*time.Second, true},
		{"0:00.03", 30 * time.Millisecond, true},
		{"12:34.56", 12*time.Minute + 34560*time.Millisecond, true},
		{"125:00.50", 125*time.Minute + 500*time.Millisecond, true},
		{"", 0, false},
		{"abc", 0, false},
		{"1:x:00", 0, false},
		{"x-01:00:00", 0, false},
		{"1:2:3:4", 0, false},
		{"-1:00", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, ok := parseCPUTime(tt.in)
			if got != tt.want || ok != tt.ok {
				t.Errorf("parseCPUTime(%q) = %v, %v; want %v, %v", tt.in, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestSamplerTree(t *testing.T) {
	// 100 is the pane; 200 and 300 its children, 301 a grandchild with spaces in its command; 400 is unrelated.
	const linux = "    1     0 00:00:09  1000 /sbin/init\n" +
		"  100     1 00:00:01  2048 claude\n" +
		"  200   100 00:01:00   512 go test ./...\n" +
		"  300   100 1-00:00:00 256 npx  mcp-server\n" +
		"  301   300 00:00:00   128 node   server.js --port 1\n" +
		"  400     1 00:00:05    64 vim\n"
	const mac = "  100     1   0:01.00  2048 claude\n" +
		"  200   100   1:00.00   512 go test ./...\n" +
		"  garbage line\n" +
		"  400     1   0:05.00    64 vim\n"
	tree := []Proc{
		{PID: 100, PPID: 1, RSS: 2048 * 1024, Command: "claude"},
		{PID: 200, PPID: 100, RSS: 512 * 1024, Command: "go test ./..."},
		{PID: 300, PPID: 100, RSS: 256 * 1024, Command: "npx  mcp-server"},
		{PID: 301, PPID: 300, RSS: 128 * 1024, Command: "node   server.js --port 1"},
	}
	tests := []struct {
		name string
		out  string
		err  error
		root int
		want []Proc
	}{
		{"linux time format", linux, nil, 100, tree},
		{"macOS time format, bad lines skipped", mac, nil, 100, tree[:2]},
		{"subtree of a child", linux, nil, 300, tree[2:]},
		{"leaf only", linux, nil, 400, []Proc{{PID: 400, PPID: 1, RSS: 64 * 1024, Command: "vim"}}},
		{"own parent is no cycle", "0 0 0:00.00 0 kernel_task\n", nil, 0, []Proc{{PID: 0, PPID: 0, Command: "kernel_task"}}},
		{"vanished root", linux, nil, 999, nil},
		{"empty output", "", nil, 100, nil},
		{"ps error", linux, errors.New("ps failed"), 100, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Sampler{ps: func() ([]byte, error) { return []byte(tt.out), tt.err }, now: time.Now}
			if got := s.Sample(tt.root); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Sample(%d) = %+v\nwant %+v", tt.root, got, tt.want)
			}
		})
	}
}

func TestSamplerCPU(t *testing.T) {
	type cpu struct {
		pct float64
		has bool
	}
	tests := []struct {
		name   string
		first  string
		second string
		wall   time.Duration // between the two samples
		want   map[int]cpu   // second sample, by pid
	}{
		{"delta of CPU time over wall time",
			"100 1 00:00:01 1 claude\n200 100 00:00:10 1 go test\n",
			"100 1 00:00:01 1 claude\n200 100 00:00:13 1 go test\n",
			2 * time.Second, map[int]cpu{100: {0, true}, 200: {150, true}}},
		{"macOS format, fractions",
			"100 1 0:01.00 1 claude\n",
			"100 1 0:01.75 1 claude\n",
			3 * time.Second, map[int]cpu{100: {25, true}}},
		{"new process has no CPU yet",
			"100 1 00:00:01 1 claude\n",
			"100 1 00:00:01 1 claude\n200 100 00:00:05 1 go test\n",
			time.Second, map[int]cpu{100: {0, true}, 200: {0, false}}},
		{"reused pid with another command",
			"100 1 00:00:01 1 claude\n200 100 00:00:01 1 go test\n",
			"100 1 00:00:01 1 claude\n200 100 00:00:03 1 make\n",
			time.Second, map[int]cpu{100: {0, true}, 200: {0, false}}},
		{"reused pid with less CPU time",
			"100 1 00:00:01 1 claude\n200 100 00:00:09 1 go test\n",
			"100 1 00:00:01 1 claude\n200 100 00:00:02 1 go test\n",
			time.Second, map[int]cpu{100: {0, true}, 200: {0, false}}},
		{"reused pid with another parent",
			"100 1 00:00:01 1 claude\n200 100 00:00:01 1 sh\n300 100 00:00:01 1 x\n",
			"100 1 00:00:01 1 claude\n200 300 00:00:02 1 sh\n300 100 00:00:01 1 x\n",
			time.Second, map[int]cpu{100: {0, true}, 200: {0, false}, 300: {0, true}}},
		{"no wall time passed",
			"100 1 00:00:01 1 claude\n",
			"100 1 00:00:02 1 claude\n",
			0, map[int]cpu{100: {0, false}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
			out := tt.first
			s := &Sampler{ps: func() ([]byte, error) { return []byte(out), nil }, now: func() time.Time { return clock }}
			for _, p := range s.Sample(100) {
				if p.HasCPU || p.CPU != 0 {
					t.Errorf("first sample: pid %d has CPU %v (%v), want none", p.PID, p.CPU, p.HasCPU)
				}
			}
			out, clock = tt.second, clock.Add(tt.wall)
			got := map[int]cpu{}
			for _, p := range s.Sample(100) {
				got[p.PID] = cpu{p.CPU, p.HasCPU}
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("second sample CPU = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSamplerKeepsHistoryOverAFailedSample(t *testing.T) {
	clock := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	out, fail := "100 1 00:00:00 1 claude\n", error(nil)
	s := &Sampler{ps: func() ([]byte, error) { return []byte(out), fail }, now: func() time.Time { return clock }}
	s.Sample(100)
	clock, fail = clock.Add(time.Second), errors.New("ps failed")
	if got := s.Sample(100); got != nil {
		t.Fatalf("failed sample = %+v, want empty", got)
	}
	clock, fail, out = clock.Add(time.Second), nil, "100 1 00:00:01 1 claude\n"
	got := s.Sample(100)
	if len(got) != 1 || !got[0].HasCPU || got[0].CPU != 50 {
		t.Errorf("sample after a failed one = %+v, want 50%% over the 2s since the last good sample", got)
	}
}
