package main

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestFieldParsesKeygenOutput(t *testing.T) {
	out := "Master Private Key: master\nMaster Public Key: public\nAvailable Private Key: client\n"
	if got := field(out, "Available Private Key:"); got != "client" {
		t.Fatalf("field() = %q", got)
	}
}

func TestShortLinkIsDecodable(t *testing.T) {
	p := map[string]any{"server": "203.0.113.1", "port": 50001, "key": "private", "aead": "chacha20-poly1305", "table_type": "prefer_entropy", "custom_table": "", "pure_downlink": false, "http_mask": false, "http_mode": "auto", "multiplex": "off"}
	link := shortLink(p)
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(link, "sudoku://"))
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["h"] != p["server"] || decoded["k"] != p["key"] {
		t.Fatalf("unexpected payload: %#v", decoded)
	}
	if decoded["a"] != "entropy" || decoded["hx"] != "off" {
		t.Fatalf("link is not compatible with the official codec: %#v", decoded)
	}
}

func TestClashYAMLContainsSudokuNode(t *testing.T) {
	c := Connection{Port: 50001, AEAD: "chacha20-poly1305", TableType: "up_ascii_down_entropy", PaddingMin: 2, PaddingMax: 7, Multiplex: "off", HTTPMode: "auto"}
	k := AccessKey{Name: "iPhone", PrivateKey: "client-key"}
	yaml := clashYAML(c, k, "203.0.113.1")
	for _, want := range []string{"type: sudoku", "server: \"203.0.113.1\"", "key: \"client-key\"", "MATCH,PROXY"} {
		if !strings.Contains(yaml, want) {
			t.Fatalf("missing %q in YAML:\n%s", want, yaml)
		}
	}
}

func TestPasswordVerification(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	cfg := AppConfig{PassHash: string(hash)}
	if !verifyPassword("secret", cfg) || verifyPassword("wrong", cfg) {
		t.Fatal("bcrypt verification failed")
	}
}

func TestValidationHelpers(t *testing.T) {
	if !validChoice("auto", "off", "auto", "on") || validChoice("bad", "off", "auto", "on") {
		t.Fatal("validChoice returned an invalid result")
	}
	if !sameVersion("v0.2.0", "0.2.0") {
		t.Fatal("sameVersion must ignore a leading v")
	}
}
