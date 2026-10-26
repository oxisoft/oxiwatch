package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/oxisoft/oxiwatch/internal/quiet"
)

// LoadQuietIPs reads only the quiet_ips list from the config file, without
// environment overrides. It is what the quiet CLI edits.
func LoadQuietIPs(path string) ([]quiet.Entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	var c struct {
		QuietIPs []quiet.Entry `json:"quiet_ips"`
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}
	return c.QuietIPs, nil
}

// SaveQuietIPs replaces the quiet_ips value in the config file and leaves
// every other byte of the file as it was: key order, layout and options this version does not know about all survive. The
// result is validated before it is written, then swapped in atomically with
// the original file's mode and owner, so the service user can still read it.
func SaveQuietIPs(path string, entries []quiet.Entry) error {
	orig, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}
	updated, err := replaceQuietIPs(orig, entries)
	if err != nil {
		return err
	}

	// Validate: the whole file must still parse and the list must compile.
	cfg := DefaultConfig()
	if err := json.Unmarshal(updated, cfg); err != nil {
		return fmt.Errorf("refusing to write: result is not valid JSON: %w", err)
	}
	if _, err := quiet.Compile(cfg.QuietIPs, cfg.Location()); err != nil {
		return fmt.Errorf("refusing to write: %w", err)
	}

	return writeFileAtomic(path, updated)
}

// replaceQuietIPs returns data with the top-level "quiet_ips" value replaced,
// or the key appended at the end of the object when it is absent.
func replaceQuietIPs(data []byte, entries []quiet.Entry) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, fmt.Errorf("config file is not a JSON object")
	}

	indent := ""
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("failed to parse config file: %w", err)
		}
		key, _ := keyTok.(string)
		keyEnd := int(dec.InputOffset())
		if indent == "" {
			indent = lineIndent(data, keyEnd)
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, fmt.Errorf("failed to parse config file: %w", err)
		}
		valEnd := int(dec.InputOffset())
		if key != "quiet_ips" {
			continue
		}
		// Value starts at the first byte after the colon that is not space.
		i := keyEnd
		for i < valEnd && (data[i] == ':' || isSpace(data[i])) {
			i++
		}
		block := formatEntries(entries, lineIndent(data, keyEnd))
		out := make([]byte, 0, len(data)+len(block))
		out = append(out, data[:i]...)
		out = append(out, block...)
		out = append(out, data[valEnd:]...)
		return out, nil
	}

	// Key absent: insert before the closing brace of the top-level object.
	end := bytes.LastIndexByte(data, '}')
	if end < 0 {
		return nil, fmt.Errorf("config file is not a JSON object")
	}
	j := end - 1
	for j >= 0 && isSpace(data[j]) {
		j--
	}
	if indent == "" {
		indent = "  "
	}
	var ins bytes.Buffer
	if j >= 0 && data[j] != '{' {
		ins.WriteString(",")
	}
	ins.WriteString("\n" + indent + `"quiet_ips": `)
	ins.WriteString(formatEntries(entries, indent))
	ins.WriteString("\n")
	out := make([]byte, 0, len(data)+ins.Len())
	out = append(out, data[:j+1]...)
	out = append(out, ins.Bytes()...)
	out = append(out, data[end:]...)
	return out, nil
}

// formatEntries renders the list one entry per line, which keeps diffs of
// hand-managed config files readable.
func formatEntries(entries []quiet.Entry, indent string) string {
	if len(entries) == 0 {
		return "[]"
	}
	var b strings.Builder
	b.WriteString("[\n")
	for i, e := range entries {
		b.WriteString(indent + "  {")
		fields := [][2]string{{"ip", e.IP}, {"user", e.User}, {"label", e.Label}, {"expires", e.Expires}}
		first := true
		for _, f := range fields {
			if f[1] == "" && f[0] != "ip" {
				continue
			}
			if !first {
				b.WriteString(", ")
			}
			first = false
			v, _ := json.Marshal(f[1])
			b.WriteString(`"` + f[0] + `": ` + string(v))
		}
		b.WriteString("}")
		if i < len(entries)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString(indent + "]")
	return b.String()
}

// lineIndent returns the leading whitespace of the line containing offset.
func lineIndent(data []byte, offset int) string {
	if offset > len(data) {
		offset = len(data)
	}
	start := bytes.LastIndexByte(data[:offset], '\n') + 1
	i := start
	for i < len(data) && (data[i] == ' ' || data[i] == '\t') {
		i++
	}
	return string(data[start:i])
}

func isSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }

// writeFileAtomic writes data next to path and renames it over path, keeping
// the original permissions and (when running as root) owner.
func writeFileAtomic(path string, data []byte) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after a successful rename

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(info.Mode().Perm()); err != nil {
		tmp.Close()
		return err
	}
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		if err := tmp.Chown(int(st.Uid), int(st.Gid)); err != nil && os.Geteuid() == 0 {
			tmp.Close()
			return fmt.Errorf("failed to keep config file owner: %w", err)
		}
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
