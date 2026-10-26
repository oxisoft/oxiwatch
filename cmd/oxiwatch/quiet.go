package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/oxisoft/oxiwatch/internal/config"
	"github.com/oxisoft/oxiwatch/internal/quiet"
	"github.com/oxisoft/oxiwatch/internal/storage"
)

const quietUsage = `Usage: oxiwatch quiet <list|add|remove|prune|check>

  quiet list
  quiet add <ip|cidr> [--user U] [--label L] [--expires 30d|2w|2026-12-31]
  quiet remove <n|ip|label>
  quiet prune
  quiet check <ip> [--user U]

Logins from a quiet address send no instant alert but are still stored and
counted in reports. The list is the "quiet_ips" key of the config file; the
running daemon picks up changes within seconds, no restart needed.`

func runQuiet(configPath string) {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, quietUsage)
		os.Exit(1)
	}
	args := os.Args[3:]
	switch os.Args[2] {
	case "list", "ls":
		quietList(configPath)
	case "add":
		quietAdd(configPath, args)
	case "remove", "rm", "delete":
		quietRemove(configPath, args)
	case "prune":
		quietPrune(configPath)
	case "check":
		quietCheck(configPath, args)
	case "help", "-h", "--help":
		fmt.Println(quietUsage)
	default:
		fmt.Fprintf(os.Stderr, "Unknown quiet command: %s\n\n%s\n", os.Args[2], quietUsage)
		os.Exit(1)
	}
}

// parseInterspersed lets flags appear before or after positional arguments
// ("quiet add 1.2.3.4 --user alex" and "quiet add --user alex 1.2.3.4").
func parseInterspersed(fset *flag.FlagSet, args []string) []string {
	var positional []string
	for {
		if err := fset.Parse(args); err != nil {
			os.Exit(2)
		}
		args = fset.Args()
		if len(args) == 0 {
			return positional
		}
		positional = append(positional, args[0])
		args = args[1:]
	}
}

func loadQuietCfg(configPath string) (*config.Config, []quiet.Entry) {
	cfg, err := config.Load(configPath)
	if err != nil {
		fatal("failed to load config: %v", err)
	}
	entries, err := config.LoadQuietIPs(configPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			fatal("config file %s not found; the quiet list lives in the config file", configPath)
		}
		fatal("%v", err)
	}
	return cfg, entries
}

func saveQuiet(configPath string, entries []quiet.Entry) {
	if err := config.SaveQuietIPs(configPath, entries); err != nil {
		if errors.Is(err, fs.ErrPermission) {
			fatal("cannot write %s: permission denied (run with sudo)", configPath)
		}
		fatal("%v", err)
	}
	notifyDaemon()
}

// notifyDaemon asks a running systemd service to reload now. The daemon also
// polls the file, so failure here only means a few seconds of delay.
func notifyDaemon() {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return
	}
	if exec.Command("systemctl", "is-active", "--quiet", "oxiwatch").Run() != nil {
		return
	}
	if exec.Command("systemctl", "kill", "--kill-whom=main", "--signal=HUP", "oxiwatch").Run() == nil {
		fmt.Println("Running daemon reloaded the quiet list.")
	}
}

func quietList(configPath string) {
	cfg, entries := loadQuietCfg(configPath)
	if len(entries) == 0 {
		fmt.Println("The quiet list is empty. Add an entry with: oxiwatch quiet add <ip|cidr> --user <name> --label <text>")
		return
	}
	loc := cfg.Location()

	var lastMatch map[string]time.Time
	if store, err := storage.New(cfg.DatabasePath); err == nil {
		lastMatch, _ = store.GetQuietLastMatches()
		store.Close()
	}

	now := time.Now()
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "#\tIP / RANGE\tUSER\tLABEL\tEXPIRES\tLAST MATCH")
	for i, e := range entries {
		user := e.User
		if user == "" {
			user = "(any)"
		}
		exp := e.Expires
		if exp == "" {
			exp = "never"
		} else if quiet.Expired(e, now, loc) {
			exp += " (expired)"
		}
		last := "-"
		if t, ok := lastMatch[e.DisplayLabel()]; ok {
			last = t.In(loc).Format("2006-01-02 15:04")
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\n", i+1, e.IP, user, e.Label, exp, last)
	}
	w.Flush()
	if _, err := quiet.Compile(entries, loc); err != nil {
		fmt.Fprintf(os.Stderr, "\nWarning: the list is invalid, the daemon keeps its previous copy: %v\n", err)
	}
}

func quietAdd(configPath string, args []string) {
	fset := flag.NewFlagSet("quiet add", flag.ExitOnError)
	user := fset.String("user", "", "only this user is quiet from the address (recommended)")
	label := fset.String("label", "", "name shown in reports, e.g. \"Alex home\"")
	expires := fset.String("expires", "", "expiry: date 2026-12-31 or 30d / 2w from today")
	pos := parseInterspersed(fset, args)
	if len(pos) != 1 {
		fatal("usage: oxiwatch quiet add <ip|cidr> [--user U] [--label L] [--expires 30d|2026-12-31]")
	}

	cfg, entries := loadQuietCfg(configPath)
	ip, err := quiet.Canonical(pos[0])
	if err != nil {
		fatal("%v", err)
	}
	if ip != strings.TrimSpace(pos[0]) {
		fmt.Printf("Normalised %s to %s\n", pos[0], ip)
	}
	exp, err := quiet.ParseExpiry(*expires, time.Now(), cfg.Location())
	if err != nil {
		fatal("%v", err)
	}
	e := quiet.Entry{IP: ip, User: strings.TrimSpace(*user), Label: strings.TrimSpace(*label), Expires: exp}

	for _, x := range entries {
		if c, err := quiet.Canonical(x.IP); err == nil && c == ip && x.User == e.User {
			fatal("%s (user %s) is already on the quiet list as %q; remove it first to change it", ip, orAny(e.User), x.DisplayLabel())
		}
	}
	entries = append(entries, e)
	saveQuiet(configPath, entries)

	fmt.Printf("Added: %s, user %s, label %q, expires %s\n", e.IP, orAny(e.User), e.DisplayLabel(), orNever(e.Expires))
	if e.User == "" {
		fmt.Println("Note: no --user given, so logins by ANY user (root included) from this address send no alert.")
	}
}

func quietRemove(configPath string, args []string) {
	if len(args) != 1 {
		fatal("usage: oxiwatch quiet remove <n|ip|label>  (n is the number shown by 'oxiwatch quiet list')")
	}
	_, entries := loadQuietCfg(configPath)
	target := strings.TrimSpace(args[0])

	var keep, removed []quiet.Entry
	if n, err := strconv.Atoi(target); err == nil {
		if n < 1 || n > len(entries) {
			fatal("no entry number %d (the list has %d)", n, len(entries))
		}
		removed = []quiet.Entry{entries[n-1]}
		keep = append(append(keep, entries[:n-1]...), entries[n:]...)
	} else {
		canon, _ := quiet.Canonical(target)
		for _, e := range entries {
			c, _ := quiet.Canonical(e.IP)
			if (canon != "" && c == canon) || e.Label == target || e.DisplayLabel() == target {
				removed = append(removed, e)
			} else {
				keep = append(keep, e)
			}
		}
	}
	if len(removed) == 0 {
		fatal("no quiet list entry matches %q", target)
	}
	saveQuiet(configPath, keep)
	for _, e := range removed {
		fmt.Printf("Removed: %s, user %s, label %q\n", e.IP, orAny(e.User), e.DisplayLabel())
	}
}

func quietPrune(configPath string) {
	cfg, entries := loadQuietCfg(configPath)
	now := time.Now()
	var keep, removed []quiet.Entry
	for _, e := range entries {
		if quiet.Expired(e, now, cfg.Location()) {
			removed = append(removed, e)
		} else {
			keep = append(keep, e)
		}
	}
	if len(removed) == 0 {
		fmt.Println("No expired entries.")
		return
	}
	saveQuiet(configPath, keep)
	for _, e := range removed {
		fmt.Printf("Removed expired: %s, user %s, label %q (expired %s)\n", e.IP, orAny(e.User), e.DisplayLabel(), e.Expires)
	}
}

func quietCheck(configPath string, args []string) {
	fset := flag.NewFlagSet("quiet check", flag.ExitOnError)
	user := fset.String("user", "", "username of the login")
	pos := parseInterspersed(fset, args)
	if len(pos) != 1 {
		fatal("usage: oxiwatch quiet check <ip> [--user U]")
	}
	cfg, entries := loadQuietCfg(configPath)
	ql, err := quiet.Compile(entries, cfg.Location())
	if err != nil {
		fatal("quiet list is invalid: %v", err)
	}
	if e, ok := ql.Match(pos[0], *user, time.Now()); ok {
		fmt.Printf("Quiet: matches %q (%s, user %s). No alert; still counted in reports.\n", e.DisplayLabel(), e.IP, orAny(e.User))
		return
	}
	fmt.Println("Not quiet: this login would send an alert.")
}

func orAny(u string) string {
	if u == "" {
		return "(any)"
	}
	return u
}

func orNever(s string) string {
	if s == "" {
		return "never"
	}
	return s
}
