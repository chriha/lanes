package session

import (
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Proc is one process of a session's process tree in a Sampler sample.
type Proc struct {
	PID, PPID int
	// CPU is the share of one core used since the previous sample, in percent (above 100 for several
	// threads). HasCPU is false on the first sample of a pid, or when the pid was reused in between.
	CPU     float64
	HasCPU  bool
	RSS     int64 // resident memory in bytes
	Command string
}

// Sampler samples the process tree under a pid with ps and turns the accumulated CPU time of two samples into
// a CPU share. ps's own %cpu is no use for spotting a spike: a lifetime average on Linux, a decaying one on
// macOS. A Sampler follows one root: it remembers only the last sampled tree, so use one Sampler per session.
// It is safe for one caller at a time from any goroutine.
type Sampler struct {
	// ps lists every process as "pid ppid time rss command" lines; now is the clock. Tests replace both.
	ps  func() ([]byte, error)
	now func() time.Time

	mu     sync.Mutex
	prev   map[int]procTime
	prevAt time.Time
}

// procTime is what a sample remembers of a process to compute the next one's CPU share and to spot a reused pid.
type procTime struct {
	ppid    int
	cpu     time.Duration
	command string
}

func NewSampler() *Sampler {
	return &Sampler{
		ps: func() ([]byte, error) {
			return exec.Command("ps", "-A", "-o", "pid=,ppid=,time=,rss=,command=").Output()
		},
		now: time.Now,
	}
}

// Sample returns root and all its descendants, ordered by pid. A vanished root, empty ps output or a ps
// error give an empty tree. Sampling another root than last time starts its CPU history afresh.
func (s *Sampler) Sample(root int) []Proc {
	s.mu.Lock()
	defer s.mu.Unlock()
	out, err := s.ps()
	if err != nil {
		return nil
	}
	at := s.now()
	all := map[int]Proc{}
	times := map[int]time.Duration{}
	children := map[int][]int{}
	for _, line := range strings.Split(string(out), "\n") {
		p, cpu, ok := parsePsLine(line)
		if !ok {
			continue
		}
		all[p.PID], times[p.PID] = p, cpu
		children[p.PPID] = append(children[p.PPID], p.PID)
	}
	if _, ok := all[root]; !ok {
		return nil
	}
	var tree []Proc
	next := map[int]procTime{}
	wall := at.Sub(s.prevAt)
	for queue := []int{root}; len(queue) > 0; queue = queue[1:] {
		p := all[queue[0]]
		if _, seen := next[p.PID]; seen {
			continue // a cycle: macOS lists pid 0 as its own parent
		}
		cpu := times[p.PID]
		if old, ok := s.prev[p.PID]; ok && wall > 0 && old.ppid == p.PPID && old.command == p.Command && cpu >= old.cpu {
			p.CPU, p.HasCPU = 100*float64(cpu-old.cpu)/float64(wall), true
		}
		next[p.PID] = procTime{ppid: p.PPID, cpu: cpu, command: p.Command}
		tree = append(tree, p)
		queue = append(queue, children[p.PID]...)
	}
	s.prev, s.prevAt = next, at
	sort.Slice(tree, func(i, j int) bool { return tree[i].PID < tree[j].PID })
	return tree
}

// parsePsLine parses one "pid ppid time rss command" line; the command keeps its inner spacing.
func parsePsLine(line string) (Proc, time.Duration, bool) {
	var f [4]string
	rest := line
	for i := range f {
		rest = strings.TrimLeft(rest, " \t")
		end := strings.IndexAny(rest, " \t")
		if end < 0 {
			return Proc{}, 0, false
		}
		f[i], rest = rest[:end], rest[end:]
	}
	command := strings.TrimSpace(rest)
	pid, ok1 := digits(f[0])
	ppid, ok2 := digits(f[1])
	cpu, ok3 := parseCPUTime(f[2])
	rss, ok4 := digits(f[3])
	if !ok1 || !ok2 || !ok3 || !ok4 || command == "" {
		return Proc{}, 0, false
	}
	return Proc{PID: pid, PPID: ppid, RSS: int64(rss) * 1024, Command: command}, cpu, true
}

// parseCPUTime parses ps's time column: [dd-]hh:mm:ss on Linux, mm:ss.ss (minutes may pass 59) on macOS.
func parseCPUTime(v string) (time.Duration, bool) {
	var d time.Duration
	if days, rest, ok := strings.Cut(v, "-"); ok {
		n, ok := digits(days)
		if !ok {
			return 0, false
		}
		d, v = time.Duration(n)*24*time.Hour, rest
	}
	f := strings.Split(v, ":")
	if len(f) < 2 || len(f) > 3 {
		return 0, false
	}
	secs, frac, _ := strings.Cut(f[len(f)-1], ".")
	s, ok := digits(secs)
	if !ok {
		return 0, false
	}
	d += time.Duration(s) * time.Second
	if frac != "" {
		n, ok := digits(frac)
		if !ok || len(frac) > 9 {
			return 0, false
		}
		d += time.Duration(n) * time.Duration(pow10(9-len(frac)))
	}
	unit := time.Minute
	for i := len(f) - 2; i >= 0; i-- {
		n, ok := digits(f[i])
		if !ok {
			return 0, false
		}
		d += time.Duration(n) * unit
		unit *= 60
	}
	return d, true
}

// digits parses a non-empty run of ASCII digits; signs, spaces and anything else fail.
func digits(s string) (int, bool) {
	if s == "" || strings.Trim(s, "0123456789") != "" {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	return n, err == nil
}

func pow10(n int) int {
	p := 1
	for range n {
		p *= 10
	}
	return p
}
