package rules

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuiltinValid(t *testing.T) {
	rs := Builtin()
	if len(rs) < 10 {
		t.Fatalf("only %d built-in rules", len(rs))
	}
	seen := map[string]bool{}
	for _, r := range rs {
		if seen[r.ID] {
			t.Errorf("duplicate id %q", r.ID)
		}
		seen[r.ID] = true
		if r.Accepts(r.To) {
			t.Errorf("%s accepts its own target %q", r.ID, r.To)
		}
	}
}

func writeUser(t *testing.T, src string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "rules.toml")
	if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadMissingFile(t *testing.T) {
	s, err := Load(filepath.Join(t.TempDir(), "nope.toml"))
	if err != nil || len(s.Rules) != len(Builtin()) {
		t.Fatalf("Load = %d rules, %v", len(s.Rules), err)
	}
}

func TestOverrideAndDisable(t *testing.T) {
	s, err := Load(writeUser(t, `
[[rule]]
id = "to-mp3"
label = "MP3 small"
args = ["-i", "{input}", "-b:a", "128k", "{output}"]

[[rule]]
id = "to-ogg"
enabled = false
`))
	if err != nil {
		t.Fatal(err)
	}
	mp3 := s.Get("to-mp3")
	if mp3.Label != "MP3 small" || !mp3.Overridden || !mp3.Builtin {
		t.Errorf("to-mp3 = %+v", mp3)
	}
	if len(mp3.From) == 0 || mp3.To != "mp3" {
		t.Errorf("override lost unset fields: %+v", mp3)
	}
	for _, r := range s.For("flac") {
		if r.ID == "to-ogg" {
			t.Error("disabled to-ogg still offered for flac")
		}
	}
	if len(s.Warnings) != 0 {
		t.Errorf("warnings %v", s.Warnings)
	}
}

func TestAddAndInvalid(t *testing.T) {
	s, err := Load(writeUser(t, `
[[rule]]
id = "to-amr"
label = "AMR"
from = [".WAV", "flac", "wav"]
to = ".AMR"
args = ["-i", "{input}", "{output}"]

[[rule]]
id = "Bad ID"
label = "x"
from = ["wav"]
to = "x"
args = ["{input}", "{output}"]

[[rule]]
id = "no-output"
label = "x"
from = ["wav"]
to = "x"
args = ["-i", "{input}"]
`))
	if err != nil {
		t.Fatal(err)
	}
	r := s.Get("to-amr")
	if r == nil || r.To != "amr" || strings.Join(r.From, ",") != "wav,flac" {
		t.Fatalf("to-amr = %+v", r)
	}
	if !r.Accepts(".Wav") || r.Builtin {
		t.Errorf("to-amr accepts/builtin wrong: %+v", r)
	}
	if len(s.Warnings) != 2 {
		t.Errorf("warnings %q, want 2", s.Warnings)
	}
}

func TestSyntaxErrorKeepsBuiltins(t *testing.T) {
	s, err := Load(writeUser(t, "[[rule]\nid="))
	if err == nil {
		t.Fatal("no error for broken TOML")
	}
	if len(s.Rules) != len(Builtin()) {
		t.Errorf("%d rules after a syntax error", len(s.Rules))
	}
}

func TestTemplateParses(t *testing.T) {
	s, err := Load(writeUser(t, Template))
	if err != nil || len(s.Warnings) != 0 || len(s.Rules) != len(Builtin()) {
		t.Fatalf("template: %v %v %d", err, s.Warnings, len(s.Rules))
	}
}

func TestExtensions(t *testing.T) {
	s := &Set{Rules: Builtin()}
	exts := s.Extensions()
	for _, want := range []string{"flac", "wav", "mp3", "m4a", "mp4", "mov"} {
		found := false
		for _, e := range exts {
			found = found || e == want
		}
		if !found {
			t.Errorf("extensions lack %q: %v", want, exts)
		}
	}
}

func TestAutoTo(t *testing.T) {
	r := (&Set{Rules: Builtin()}).Get("extract-audio")
	if r == nil || !r.AutoTo() || !r.Accepts("mkv") || !r.Accepts("MP4") {
		t.Fatalf("extract-audio = %+v", r)
	}
}
