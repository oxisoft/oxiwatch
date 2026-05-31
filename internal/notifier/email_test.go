package notifier

import (
	"testing"
)

func TestParseSMTPURL(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		wantHost string
		wantAddr string
		wantErr  bool
	}{
		{"smtp scheme with port", "smtp://host:587", "host", "host:587", false},
		{"smtps scheme with port", "smtps://h:465", "h", "h:465", false},
		{"bare host with port", "host:25", "host", "host:25", false},
		{"missing port defaults to 587", "smtp://host", "host", "host:587", false},
		{"bare host without port defaults to 587", "host", "host", "host:587", false},
		{"empty string is invalid", "", "", "", true},
		{"control char invalid url", "smtp://ho\x7fst:25", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, addr, err := parseSMTPURL(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseSMTPURL(%q) error = nil, want error", tt.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSMTPURL(%q) unexpected error: %v", tt.raw, err)
			}
			if host != tt.wantHost {
				t.Errorf("host = %q, want %q", host, tt.wantHost)
			}
			if addr != tt.wantAddr {
				t.Errorf("addr = %q, want %q", addr, tt.wantAddr)
			}
		})
	}
}

func TestNewEmailValidation(t *testing.T) {
	tests := []struct {
		name    string
		smtpURL string
		from    string
		to      []string
		wantErr bool
	}{
		{"valid", "smtp://host:587", "a@b.com", []string{"c@d.com"}, false},
		{"missing smtp url", "", "a@b.com", []string{"c@d.com"}, true},
		{"missing from", "smtp://host:587", "", []string{"c@d.com"}, true},
		{"no recipients", "smtp://host:587", "a@b.com", nil, true},
		{"empty recipient slice", "smtp://host:587", "a@b.com", []string{}, true},
		{"invalid smtp url", "smtp://host:587", "a@b.com", []string{"c@d.com"}, false},
		{"bad smtp url errors", "smtp://\x7f", "a@b.com", []string{"c@d.com"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, err := NewEmail(tt.smtpURL, tt.from, tt.to, "user", "pass")
			if tt.wantErr {
				if err == nil {
					t.Fatalf("NewEmail() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("NewEmail() unexpected error: %v", err)
			}
			if e == nil {
				t.Fatal("NewEmail() returned nil Email without error")
			}
		})
	}
}

func TestEmailName(t *testing.T) {
	e, err := NewEmail("smtp://host:587", "a@b.com", []string{"c@d.com"}, "u", "p")
	if err != nil {
		t.Fatalf("NewEmail() error: %v", err)
	}
	if e.Name() != "email" {
		t.Errorf("Name() = %q, want %q", e.Name(), "email")
	}
}

func TestNewEmailStoresFields(t *testing.T) {
	e, err := NewEmail("smtp://mail.example.com", "from@x", []string{"to1@x", "to2@x"}, "user", "secret")
	if err != nil {
		t.Fatalf("NewEmail() error: %v", err)
	}
	if e.host != "mail.example.com" {
		t.Errorf("host = %q, want mail.example.com", e.host)
	}
	if e.addr != "mail.example.com:587" {
		t.Errorf("addr = %q, want mail.example.com:587", e.addr)
	}
	if e.from != "from@x" {
		t.Errorf("from = %q, want from@x", e.from)
	}
	if len(e.to) != 2 {
		t.Errorf("to len = %d, want 2", len(e.to))
	}
}
