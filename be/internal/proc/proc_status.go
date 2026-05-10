// Package proc provides host process probing utilities.
package proc

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

// PidAlive reports whether a process with the given pid is alive on this host.
// Returns false for pid <= 0.
func PidAlive(pid int64) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(int(pid), 0)
	return err == nil || err == syscall.EPERM
}

// PidMetrics returns resource usage for the given pid by shelling out to ps.
// Returns zeros and ok=false on any failure (dead pid, ps not found, parse error).
func PidMetrics(pid int64) (rssKB int64, cpuPct float64, etimeSec int64, ok bool) {
	if pid <= 0 {
		return 0, 0, 0, false
	}
	psPath, err := exec.LookPath("ps")
	if err != nil {
		return 0, 0, 0, false
	}
	out, err := exec.Command(psPath, "-o", "rss=,%cpu=,etime=", "-p", fmt.Sprintf("%d", pid)).Output()
	if err != nil {
		return 0, 0, 0, false
	}
	fields := strings.Fields(strings.TrimSpace(string(out)))
	if len(fields) < 3 {
		return 0, 0, 0, false
	}
	rss, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return 0, 0, 0, false
	}
	cpu, err := strconv.ParseFloat(fields[1], 64)
	if err != nil {
		return 0, 0, 0, false
	}
	etime, err := parseEtime(fields[2])
	if err != nil {
		return 0, 0, 0, false
	}
	return rss, cpu, etime, true
}

// parseEtime parses ps etime field into seconds.
// Formats: MM:SS, HH:MM:SS, DD-HH:MM:SS
func parseEtime(s string) (int64, error) {
	var days, hours, minutes, seconds int64

	// Check for DD-HH:MM:SS
	if idx := strings.Index(s, "-"); idx != -1 {
		d, err := strconv.ParseInt(s[:idx], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid etime days: %s", s)
		}
		days = d
		s = s[idx+1:]
	}

	parts := strings.Split(s, ":")
	switch len(parts) {
	case 2: // MM:SS
		m, err1 := strconv.ParseInt(parts[0], 10, 64)
		sec, err2 := strconv.ParseInt(parts[1], 10, 64)
		if err1 != nil || err2 != nil {
			return 0, fmt.Errorf("invalid etime: %s", s)
		}
		minutes = m
		seconds = sec
	case 3: // HH:MM:SS
		h, err1 := strconv.ParseInt(parts[0], 10, 64)
		m, err2 := strconv.ParseInt(parts[1], 10, 64)
		sec, err3 := strconv.ParseInt(parts[2], 10, 64)
		if err1 != nil || err2 != nil || err3 != nil {
			return 0, fmt.Errorf("invalid etime: %s", s)
		}
		hours = h
		minutes = m
		seconds = sec
	default:
		return 0, fmt.Errorf("invalid etime format: %s", s)
	}

	return days*86400 + hours*3600 + minutes*60 + seconds, nil
}
