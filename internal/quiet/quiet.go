// Package quiet implements the quiet list: known IP addresses or ranges whose
// successful SSH logins are recorded and reported as usual but do not trigger
// an instant login alert.
package quiet

import (
	"fmt"
	"net/netip"
	"strings"
	"time"
)

// DateLayout is the format of the "expires" field.
const DateLayout = "2006-01-02"

// Entry is one quiet_ips item as stored in config.json.
type Entry struct {
	IP      string `json:"ip"`
	User    string `json:"user,omitempty"`
	Label   string `json:"label,omitempty"`
	Expires string `json:"expires,omitempty"`
}

// DisplayLabel is the label shown in reports and logs. It falls back to the
// address (plus user) when no label is set, so every match is identifiable.
func (e Entry) DisplayLabel() string {
	if e.Label != "" {
		return e.Label
	}
	if e.User != "" {
		return e.IP + " " + e.User
	}
	return e.IP
}

// ParsePrefix accepts a single address ("203.0.113.7", "2001:db8::1") or a
// CIDR range ("198.51.100.0/24"). IPv4-mapped IPv6 addresses are unmapped.
func ParsePrefix(s string) (netip.Prefix, error) {
	s = strings.TrimSpace(s)
	if strings.Contains(s, "/") {
		p, err := netip.ParsePrefix(s)
		if err != nil {
			return netip.Prefix{}, fmt.Errorf("invalid IP range %q", s)
		}
		addr := p.Addr()
		if addr.Is4In6() {
			bits := p.Bits() - 96
			if bits < 0 {
				return netip.Prefix{}, fmt.Errorf("invalid IP range %q", s)
			}
			p = netip.PrefixFrom(addr.Unmap(), bits)
		}
		return p, nil
	}
	addr, err := netip.ParseAddr(s)
	if err != nil || addr.Zone() != "" {
		return netip.Prefix{}, fmt.Errorf("invalid IP address %q", s)
	}
	addr = addr.Unmap()
	return netip.PrefixFrom(addr, addr.BitLen()), nil
}

// Canonical returns the normalised form of an address or range: a single
// address without "/32" or "/128", a range with its host bits cleared.
func Canonical(s string) (string, error) {
	p, err := ParsePrefix(s)
	if err != nil {
		return "", err
	}
	if p.IsSingleIP() {
		return p.Addr().String(), nil
	}
	return p.Masked().String(), nil
}

type rule struct {
	entry  Entry
	prefix netip.Prefix
	until  time.Time // zero: never expires; otherwise first instant it no longer applies
}

// List is a compiled, ready-to-match quiet list. The zero value and nil both
// match nothing.
type List struct {
	rules []rule
}

// Compile validates entries and prepares them for matching. An entry's
// expiry date is inclusive: it stops matching at midnight after that date in
// loc (the configured timezone).
func Compile(entries []Entry, loc *time.Location) (*List, error) {
	if loc == nil {
		loc = time.UTC
	}
	l := &List{}
	labels := make(map[string]int)
	for i, e := range entries {
		n := i + 1
		if strings.TrimSpace(e.IP) == "" {
			return nil, fmt.Errorf("quiet_ips[%d]: ip is required", n)
		}
		p, err := ParsePrefix(e.IP)
		if err != nil {
			return nil, fmt.Errorf("quiet_ips[%d]: %w", n, err)
		}
		r := rule{entry: e, prefix: p.Masked()}
		if e.Expires != "" {
			d, err := time.ParseInLocation(DateLayout, e.Expires, loc)
			if err != nil {
				return nil, fmt.Errorf("quiet_ips[%d]: expires must be a date like 2026-12-31, got %q", n, e.Expires)
			}
			r.until = d.AddDate(0, 0, 1)
		}
		label := e.DisplayLabel()
		if prev, ok := labels[label]; ok {
			return nil, fmt.Errorf("quiet_ips[%d]: label %q is already used by entry %d", n, label, prev)
		}
		labels[label] = n
		l.rules = append(l.rules, r)
	}
	return l, nil
}

// Len is the number of entries, expired ones included.
func (l *List) Len() int {
	if l == nil {
		return 0
	}
	return len(l.rules)
}

// Active is the number of entries that have not expired at now.
func (l *List) Active(now time.Time) int {
	n := 0
	if l == nil {
		return 0
	}
	for _, r := range l.rules {
		if !r.expired(now) {
			n++
		}
	}
	return n
}

func (r rule) expired(now time.Time) bool {
	return !r.until.IsZero() && !now.Before(r.until)
}

// Match returns the first entry that covers this login: the address is inside
// the entry's range, the entry is not expired, and the entry either has no
// user restriction or names exactly this user.
func (l *List) Match(ip, user string, now time.Time) (Entry, bool) {
	if l == nil {
		return Entry{}, false
	}
	addr, err := netip.ParseAddr(strings.TrimSpace(ip))
	if err != nil {
		return Entry{}, false
	}
	addr = addr.Unmap().WithZone("")
	for _, r := range l.rules {
		if r.expired(now) {
			continue
		}
		if r.entry.User != "" && r.entry.User != user {
			continue
		}
		if r.prefix.Contains(addr) {
			return r.entry, true
		}
	}
	return Entry{}, false
}

// Expired reports whether e has expired at now (loc as in Compile). Invalid
// dates are treated as not expired; Compile reports them.
func Expired(e Entry, now time.Time, loc *time.Location) bool {
	if e.Expires == "" {
		return false
	}
	if loc == nil {
		loc = time.UTC
	}
	d, err := time.ParseInLocation(DateLayout, e.Expires, loc)
	if err != nil {
		return false
	}
	return !now.Before(d.AddDate(0, 0, 1))
}

// ParseExpiry turns a CLI value into an expiry date: either a date
// ("2026-12-31") or a duration in days or weeks from today ("30d", "2w").
func ParseExpiry(s string, now time.Time, loc *time.Location) (string, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return "", nil
	}
	if loc == nil {
		loc = time.UTC
	}
	if _, err := time.ParseInLocation(DateLayout, s, loc); err == nil {
		return s, nil
	}
	unit := s[len(s)-1]
	var n int
	if _, err := fmt.Sscanf(s[:len(s)-1], "%d", &n); err != nil || n < 1 || fmt.Sprint(n) != s[:len(s)-1] {
		return "", fmt.Errorf("invalid expiry %q: use a date (2026-12-31) or days/weeks (30d, 2w)", s)
	}
	days := n
	switch unit {
	case 'd':
	case 'w':
		days = n * 7
	default:
		return "", fmt.Errorf("invalid expiry %q: use a date (2026-12-31) or days/weeks (30d, 2w)", s)
	}
	return now.In(loc).AddDate(0, 0, days).Format(DateLayout), nil
}
