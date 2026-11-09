//go:build windows

package shellmenu

import (
	"slices"
	"testing"

	"golang.org/x/sys/windows/registry"

	"github.com/rightconvert/rightconvert/internal/rules"
)

func TestRegisterUnregister(t *testing.T) {
	// A key of its own under HKCU, so that the test leaves Explorer alone.
	ClassesRoot = `Software\RightConvertTest\Classes`
	t.Cleanup(func() {
		deleteTree(`Software\RightConvertTest`)
		ClassesRoot = `Software\Classes`
	})
	set := &rules.Set{Rules: rules.Builtin()}
	exe := `C:\Users\Test User\AppData\Local\Programs\RightConvert\RightConvert.exe`
	n, err := Register(set, Options{Exe: exe, Title: "转换为", Usable: func(r *rules.Rule) bool { return r.ID != "to-opus" }})
	if err != nil {
		t.Fatal(err)
	}
	exts, err := Registered()
	if err != nil || len(exts) != n || !slices.Contains(exts, "flac") || !slices.Contains(exts, "mov") {
		t.Fatalf("Registered = %v, %v; Register = %d", exts, err, n)
	}

	root := assocKey("flac") + `\shell\` + Verb
	k, err := registry.OpenKey(registry.CURRENT_USER, root, registry.READ)
	if err != nil {
		t.Fatal(err)
	}
	title, _, _ := k.GetStringValue("MUIVerb")
	k.Close()
	if title != "转换为" {
		t.Errorf("MUIVerb = %q", title)
	}
	k, _ = registry.OpenKey(registry.CURRENT_USER, root+`\shell`, registry.READ)
	items, _ := k.ReadSubKeyNames(-1)
	k.Close()
	if slices.ContainsFunc(items, func(s string) bool { return len(s) > 3 && s[3:] == "to-flac" }) {
		t.Errorf("flac offers to-flac: %v", items)
	}
	if slices.ContainsFunc(items, func(s string) bool { return len(s) > 3 && s[3:] == "to-opus" }) {
		t.Errorf("unusable rule registered: %v", items)
	}
	ck, err := registry.OpenKey(registry.CURRENT_USER, root+`\shell\00_to-mp3\command`, registry.READ)
	if err != nil {
		t.Fatalf("no to-mp3 command among %v: %v", items, err)
	}
	cmd, _, _ := ck.GetStringValue("")
	ck.Close()
	if want := `"` + exe + `" convert --rule to-mp3 "%1"`; cmd != want {
		t.Errorf("command %q, want %q", cmd, want)
	}

	// Registering again replaces the menus: a disabled rule disappears.
	f := false
	set.Get("to-mp3").Enabled = &f
	if _, err := Register(set, Options{Exe: exe, Title: "转换为"}); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.OpenKey(registry.CURRENT_USER, root+`\shell\00_to-mp3`, registry.READ); err == nil {
		t.Error("disabled rule still registered")
	}

	if err := Unregister(); err != nil {
		t.Fatal(err)
	}
	if exts, _ := Registered(); len(exts) != 0 {
		t.Errorf("left %v after Unregister", exts)
	}
}
