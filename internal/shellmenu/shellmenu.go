// Package shellmenu adds the "Convert to" submenu to Explorer's context menu
// of the files the rules convert. It writes only HKEY_CURRENT_USER, so it
// needs no administrator rights.
package shellmenu

import (
	"fmt"
	"strings"

	"github.com/rightconvert/rightconvert/internal/rules"
)

// Verb is the name of the key of the submenu under each type's shell key.
const Verb = "RightConvert"

// Entry is one item of the submenu.
type Entry struct {
	Key     string // subkey name; Explorer sorts the items by it
	Label   string
	Command string
}

// Menu is what Register writes for one extension.
type Menu struct {
	Ext     string // without the dot
	Entries []Entry
}

// Options say what Register writes.
type Options struct {
	Exe   string // the full path of RightConvert.exe
	Title string // the submenu's label, as "Convert to"
	// Usable reports whether a rule can run with the installed ffmpeg;
	// nil allows every rule.
	Usable func(*rules.Rule) bool
}

// Plan returns the menus for the enabled rules, by extension.
func Plan(set *rules.Set, opts Options) []Menu {
	var menus []Menu
	for _, ext := range set.Extensions() {
		m := Menu{Ext: ext}
		for i, r := range set.For(ext) {
			if opts.Usable != nil && !opts.Usable(r) {
				continue
			}
			m.Entries = append(m.Entries, Entry{
				Key:     fmt.Sprintf("%02d_%s", i, r.ID),
				Label:   r.Label,
				Command: Command(opts.Exe, r.ID),
			})
		}
		if len(m.Entries) > 0 {
			menus = append(menus, m)
		}
	}
	return menus
}

// Command is the command line Explorer runs for a rule, with %1 the file.
func Command(exe, ruleID string) string {
	return fmt.Sprintf(`%s convert --rule %s "%%1"`, quote(exe), ruleID)
}

func quote(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, ``) + `"`
}
