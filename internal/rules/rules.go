// Package rules loads the conversion rules: the built-in ones, merged with
// the user's rules.toml.
package rules

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
)

//go:embed defaults.toml
var defaultsTOML string

// AutoExt as a rule's To names the output after the codec of the source's
// first audio stream, as rules copying it need.
const AutoExt = "auto"

// Rule converts files of the extensions in From to a file of extension To
// by running ffmpeg with Args.
type Rule struct {
	ID    string   `toml:"id"`
	Label string   `toml:"label"`
	From  []string `toml:"from"`
	To    string   `toml:"to"`
	// Suffix is appended to the output file's base name, as "-v0".
	Suffix string `toml:"suffix,omitempty"`
	// Encoders are the ffmpeg encoders the rule needs; a rule whose encoders
	// the installed ffmpeg lacks is shown as unavailable.
	Encoders []string `toml:"encoders,omitempty"`
	Args     []string `toml:"args"`
	// Enabled is nil when unset, which means enabled.
	Enabled *bool `toml:"enabled,omitempty"`

	// Builtin is true for rules that come from defaults.toml, even when
	// rules.toml overrides them.
	Builtin bool `toml:"-"`
	// Overridden is true for built-in rules that rules.toml replaces.
	Overridden bool `toml:"-"`
}

// AutoTo reports whether the output extension depends on the source.
func (r *Rule) AutoTo() bool { return r.To == AutoExt }

// IsEnabled reports whether the rule shows in menus.
func (r *Rule) IsEnabled() bool { return r.Enabled == nil || *r.Enabled }

// Accepts reports whether the rule converts files of extension ext, given
// with or without the dot in any case.
func (r *Rule) Accepts(ext string) bool {
	ext = normExt(ext)
	return ext != "" && ext != r.To && slices.Contains(r.From, ext)
}

type file struct {
	Rule []Rule `toml:"rule"`
}

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,47}$`)

// Validate checks the fields of a complete rule.
func (r *Rule) Validate() error {
	if !idPattern.MatchString(r.ID) {
		return fmt.Errorf("rule %q: id must be lowercase letters, digits, '.', '_' or '-' (max 48)", r.ID)
	}
	if strings.TrimSpace(r.Label) == "" {
		return fmt.Errorf("rule %q: label is empty", r.ID)
	}
	if r.To == "" || strings.ContainsAny(r.To, `\/:*?"<>|. `) {
		return fmt.Errorf("rule %q: invalid target extension %q", r.ID, r.To)
	}
	if len(r.From) == 0 {
		return fmt.Errorf("rule %q: from is empty", r.ID)
	}
	if strings.ContainsAny(r.Suffix, `\/:*?"<>|`) {
		return fmt.Errorf("rule %q: invalid suffix %q", r.ID, r.Suffix)
	}
	in, out := false, false
	for _, a := range r.Args {
		in = in || strings.Contains(a, "{input}")
		out = out || strings.Contains(a, "{output}")
	}
	if !in || !out {
		return fmt.Errorf("rule %q: args must contain {input} and {output}", r.ID)
	}
	return nil
}

func normExt(ext string) string {
	return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(ext), "."))
}

func (r *Rule) normalize() {
	r.ID = strings.TrimSpace(r.ID)
	r.To = normExt(r.To)
	seen := map[string]bool{}
	from := r.From[:0:0]
	for _, e := range r.From {
		if e = normExt(e); e != "" && !seen[e] {
			seen[e] = true
			from = append(from, e)
		}
	}
	r.From = from
}

// Set is the merged list of rules, in menu order.
type Set struct {
	Rules []*Rule
	// Warnings lists problems in the user's file that left rules out.
	Warnings []string
}

// Builtin returns the built-in rules.
func Builtin() []*Rule {
	rs, err := parse(defaultsTOML)
	if err != nil {
		panic("rules: defaults.toml: " + err.Error())
	}
	for _, r := range rs {
		if err := r.Validate(); err != nil {
			panic("rules: defaults.toml: " + err.Error())
		}
		r.Builtin = true
	}
	return rs
}

func parse(src string) ([]*Rule, error) {
	var f file
	if _, err := toml.Decode(src, &f); err != nil {
		return nil, err
	}
	rs := make([]*Rule, len(f.Rule))
	for i := range f.Rule {
		r := f.Rule[i]
		r.normalize()
		rs[i] = &r
	}
	return rs, nil
}

// Load merges the user's rules file at path, which may not exist, over the
// built-in rules. A syntax error in the file is returned along with the
// built-in rules; invalid rules in it are skipped with a warning.
func Load(path string) (*Set, error) {
	set := &Set{Rules: Builtin()}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return set, nil
	}
	if err != nil {
		return set, err
	}
	user, err := parse(string(data))
	if err != nil {
		return set, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	set.merge(user)
	return set, nil
}

func (s *Set) merge(user []*Rule) {
	for _, u := range user {
		i := slices.IndexFunc(s.Rules, func(r *Rule) bool { return r.ID == u.ID })
		if i >= 0 && s.Rules[i].Builtin {
			// Fields left out override nothing, so that `enabled = false`
			// alone hides a built-in rule.
			merged := *s.Rules[i]
			if u.Label != "" {
				merged.Label = u.Label
			}
			if len(u.From) > 0 {
				merged.From = u.From
			}
			if u.To != "" {
				merged.To = u.To
			}
			if u.Suffix != "" {
				merged.Suffix = u.Suffix
			}
			if u.Encoders != nil {
				merged.Encoders = u.Encoders
			}
			if len(u.Args) > 0 {
				merged.Args = u.Args
			}
			if u.Enabled != nil {
				merged.Enabled = u.Enabled
			}
			merged.Overridden = true
			if err := merged.Validate(); err != nil {
				s.Warnings = append(s.Warnings, err.Error())
				continue
			}
			s.Rules[i] = &merged
			continue
		}
		if err := u.Validate(); err != nil {
			s.Warnings = append(s.Warnings, err.Error())
			continue
		}
		if i >= 0 {
			s.Warnings = append(s.Warnings, fmt.Sprintf("rule %q is defined twice; the later one wins", u.ID))
			s.Rules[i] = u
			continue
		}
		s.Rules = append(s.Rules, u)
	}
}

// Get returns the rule with the id, or nil.
func (s *Set) Get(id string) *Rule {
	for _, r := range s.Rules {
		if r.ID == id {
			return r
		}
	}
	return nil
}

// For returns the enabled rules that convert files of extension ext.
func (s *Set) For(ext string) []*Rule {
	var out []*Rule
	for _, r := range s.Rules {
		if r.IsEnabled() && r.Accepts(ext) {
			out = append(out, r)
		}
	}
	return out
}

// Extensions returns every source extension of the enabled rules, sorted.
func (s *Set) Extensions() []string {
	var exts []string
	for _, r := range s.Rules {
		if r.IsEnabled() {
			exts = append(exts, r.From...)
		}
	}
	slices.Sort(exts)
	return slices.Compact(exts)
}

// Template is written to a new rules.toml so that users start from an
// example.
const Template = `# RightConvert custom rules.
#
# A rule with the id of a built-in rule overrides the fields it sets, e.g.
#
#   [[rule]]
#   id = "to-mp3"
#   args = ["-i", "{input}", "-map", "0:a:0", "-c:a", "libmp3lame", "-b:a", "192k", "{output}"]
#
# hides a built-in rule with
#
#   [[rule]]
#   id = "to-ogg"
#   enabled = false
#
# and a rule with a new id adds a menu item. Placeholders in args:
# {input} {output} {dir} {name}. to = "auto" names the output after the
# codec of the first audio stream, for rules that copy it with -c:a copy. After editing, choose "Apply to context
# menu" in RightConvert's settings so that Explorer shows the change.
#
# [[rule]]
# id       = "to-mp3-128"
# label    = "MP3 (128k, small)"
# from     = ["flac", "wav", "m4a"]
# to       = "mp3"
# suffix   = "-128k"
# encoders = ["libmp3lame"]
# args     = ["-i", "{input}", "-map", "0:a:0", "-c:a", "libmp3lame", "-b:a", "128k", "{output}"]
`
