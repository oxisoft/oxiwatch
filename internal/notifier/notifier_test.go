package notifier

import (
	"errors"
	"strings"
	"testing"
)

func TestFormatLocation(t *testing.T) {
	tests := []struct {
		name    string
		ip      string
		country string
		city    string
		want    string
	}{
		{"ip only when no geo", "1.2.3.4", "", "", "1.2.3.4"},
		{"city and country", "1.2.3.4", "Germany", "Berlin", "Berlin, Germany"},
		{"country only", "1.2.3.4", "Germany", "", "Germany"},
		{"city only", "1.2.3.4", "", "Berlin", "Berlin"},
		{"empty ip no geo", "", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatLocation(tt.ip, tt.country, tt.city); got != tt.want {
				t.Fatalf("formatLocation(%q,%q,%q) = %q, want %q", tt.ip, tt.country, tt.city, got, tt.want)
			}
		})
	}
}

func TestEscapeHTML(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"ampersand", "a&b", "a&amp;b"},
		{"less than", "a<b", "a&lt;b"},
		{"greater than", "a>b", "a&gt;b"},
		{"all three", "<a&b>", "&lt;a&amp;b&gt;"},
		{"ampersand first to avoid double escape", "&lt;", "&amp;lt;"},
		{"no special chars", "plain text", "plain text"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := escapeHTML(tt.in); got != tt.want {
				t.Fatalf("escapeHTML(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestHTMLToText(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"strip tags", "<b>bold</b>", "bold"},
		{"unescape entities", "&lt;a&gt; &amp; &lt;b&gt;", "<a> & <b>"},
		{"newlines preserved", "line1\nline2", "line1\nline2"},
		{"tags and newline", "<b>title</b>\nbody", "title\nbody"},
		{"nested tags", "<html><body>x</body></html>", "x"},
		{"no markup", "plain", "plain"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := htmlToText(tt.in); got != tt.want {
				t.Fatalf("htmlToText(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestMessage(t *testing.T) {
	html := "<b>Title</b>\nbody &amp; more"
	msg := message("My Subject", html)

	if msg.Subject != "My Subject" {
		t.Errorf("Subject = %q, want %q", msg.Subject, "My Subject")
	}
	if msg.HTML != html {
		t.Errorf("HTML = %q, want %q", msg.HTML, html)
	}
	want := htmlToText(html)
	if msg.Text != want {
		t.Errorf("Text = %q, want htmlToText(HTML) = %q", msg.Text, want)
	}
	// Spot check derived plain text.
	if msg.Text != "Title\nbody & more" {
		t.Errorf("Text = %q, want %q", msg.Text, "Title\nbody & more")
	}
}

// fakeChannel is a test double implementing the Channel interface.
type fakeChannel struct {
	name     string
	err      error
	received []Message
}

func (f *fakeChannel) Name() string { return f.name }

func (f *fakeChannel) Send(msg Message) error {
	f.received = append(f.received, msg)
	return f.err
}

func TestManagerAddCountNames(t *testing.T) {
	m := &Manager{}

	if m.Count() != 0 {
		t.Fatalf("initial Count = %d, want 0", m.Count())
	}
	if len(m.Names()) != 0 {
		t.Fatalf("initial Names len = %d, want 0", len(m.Names()))
	}

	m.Add(&fakeChannel{name: "a"})
	m.Add(&fakeChannel{name: "b"})
	// Nil channels are ignored.
	m.Add(nil)

	if m.Count() != 2 {
		t.Fatalf("Count = %d, want 2", m.Count())
	}
	names := m.Names()
	if len(names) != 2 || names[0] != "a" || names[1] != "b" {
		t.Fatalf("Names = %v, want [a b]", names)
	}
}

func TestManagerDispatchFanOut(t *testing.T) {
	ok := &fakeChannel{name: "good"}
	bad := &fakeChannel{name: "broken", err: errors.New("boom")}

	m := &Manager{}
	m.Add(ok)
	m.Add(bad)

	msg := message("Subj", "<b>hi</b>")
	err := m.dispatch(msg)

	// Both channels must have received the message.
	if len(ok.received) != 1 {
		t.Fatalf("good channel received %d messages, want 1", len(ok.received))
	}
	if len(bad.received) != 1 {
		t.Fatalf("broken channel received %d messages, want 1", len(bad.received))
	}
	if ok.received[0] != msg {
		t.Errorf("good channel got %+v, want %+v", ok.received[0], msg)
	}

	// The joined error must be non-nil and name the failing channel.
	if err == nil {
		t.Fatal("dispatch error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "broken") {
		t.Errorf("error %q does not name failing channel 'broken'", err.Error())
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("error %q does not wrap underlying error 'boom'", err.Error())
	}
}

func TestManagerDispatchAllSucceed(t *testing.T) {
	a := &fakeChannel{name: "a"}
	b := &fakeChannel{name: "b"}
	m := &Manager{}
	m.Add(a)
	m.Add(b)

	if err := m.dispatch(message("s", "<i>x</i>")); err != nil {
		t.Fatalf("dispatch error = %v, want nil", err)
	}
	if len(a.received) != 1 || len(b.received) != 1 {
		t.Fatalf("not all channels received message: a=%d b=%d", len(a.received), len(b.received))
	}
}

func TestManagerDispatchNoChannels(t *testing.T) {
	m := &Manager{}
	if err := m.dispatch(message("s", "x")); err != nil {
		t.Fatalf("dispatch with no channels error = %v, want nil", err)
	}
}
