// Package collector enumerates the current user's established TCP connections
// via lsof — the no-entitlement, no-sudo capture tier the spike validated.
// This is a straight port of spike/attribution_spike.py's parsing.
package collector

import (
	"os/exec"
	"strings"
)

type Conn struct {
	Command    string
	PID        string
	RemoteHost string
	RemotePort string
}

// Collect runs lsof and returns established TCP connections owned by the user.
func Collect() ([]Conn, error) {
	cmd := exec.Command("/usr/sbin/lsof",
		"+c", "0", // do not truncate command names ("Google Chrome Helper")
		"-nP",                 // numeric hosts + ports (fast, no DNS)
		"-iTCP",               // TCP only
		"-sTCP:ESTABLISHED",   // established connections only
		"-F", "pcn",           // machine-readable: pid, command, name
	)
	out, err := cmd.Output()
	// lsof exits non-zero when some fds are unreadable but still prints usable
	// output. Only treat as fatal when we got nothing back.
	if err != nil && len(out) == 0 {
		return nil, err
	}
	return parse(string(out)), nil
}

func parse(text string) []Conn {
	var conns []Conn
	var command, pid string
	for _, line := range strings.Split(text, "\n") {
		if line == "" {
			continue
		}
		tag, val := line[0], line[1:]
		switch tag {
		case 'p':
			pid = val
		case 'c':
			command = val
		case 'n':
			if i := strings.Index(val, "->"); i >= 0 {
				host, port := splitHostPort(strings.TrimSpace(val[i+2:]))
				conns = append(conns, Conn{Command: command, PID: pid, RemoteHost: host, RemotePort: port})
			}
		}
	}
	return conns
}

func splitHostPort(ep string) (host, port string) {
	if strings.HasPrefix(ep, "[") { // IPv6: [2607:6bc0::10]:443
		if i := strings.Index(ep, "]:"); i >= 0 {
			return ep[1:i], ep[i+2:]
		}
		return ep, ""
	}
	if i := strings.LastIndex(ep, ":"); i >= 0 { // IPv4: 1.2.3.4:443
		return ep[:i], ep[i+1:]
	}
	return ep, ""
}
