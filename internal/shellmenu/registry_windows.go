//go:build windows

package shellmenu

import (
	"errors"
	"strings"
	"syscall"

	"golang.org/x/sys/windows/registry"

	"github.com/rightconvert/rightconvert/internal/rules"
)

// ClassesRoot is the per-user key Register writes under. Tests change it.
var ClassesRoot = `Software\Classes`

func assocKey(ext string) string {
	return ClassesRoot + `\SystemFileAssociations\.` + ext
}

// Register replaces the context menus RightConvert added with those of the
// rules, and returns how many file types have one.
func Register(set *rules.Set, opts Options) (int, error) {
	if err := Unregister(); err != nil {
		return 0, err
	}
	menus := Plan(set, opts)
	for _, m := range menus {
		if err := writeMenu(m, opts); err != nil {
			return 0, err
		}
	}
	return len(menus), nil
}

func writeMenu(m Menu, opts Options) error {
	root := assocKey(m.Ext) + `\shell\` + Verb
	k, _, err := registry.CreateKey(registry.CURRENT_USER, root, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	for name, v := range map[string]string{
		"MUIVerb":     opts.Title,
		"SubCommands": "",
		"Icon":        quote(opts.Exe) + ",0",
		// Explorer leaves out the static verbs of a selection of more than
		// 15 files unless the verb allows it.
		"MultiSelectModel": "Player",
	} {
		if err := k.SetStringValue(name, v); err != nil {
			return err
		}
	}
	for _, e := range m.Entries {
		ek, _, err := registry.CreateKey(registry.CURRENT_USER, root+`\shell\`+e.Key, registry.SET_VALUE)
		if err != nil {
			return err
		}
		err = errors.Join(ek.SetStringValue("MUIVerb", e.Label), ek.SetStringValue("MultiSelectModel", "Player"))
		ek.Close()
		if err != nil {
			return err
		}
		ck, _, err := registry.CreateKey(registry.CURRENT_USER, root+`\shell\`+e.Key+`\command`, registry.SET_VALUE)
		if err != nil {
			return err
		}
		err = ck.SetStringValue("", e.Command)
		ck.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// Unregister removes every context menu RightConvert added.
func Unregister() error {
	exts, err := Registered()
	if err != nil {
		return err
	}
	for _, ext := range exts {
		if err := deleteTree(assocKey(ext) + `\shell\` + Verb); err != nil {
			return err
		}
	}
	return nil
}

// Registered returns the extensions that have RightConvert's menu.
func Registered() ([]string, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, ClassesRoot+`\SystemFileAssociations`, registry.ENUMERATE_SUB_KEYS)
	if errors.Is(err, registry.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	names, err := k.ReadSubKeyNames(-1)
	k.Close()
	if err != nil {
		return nil, err
	}
	var exts []string
	for _, n := range names {
		if !strings.HasPrefix(n, ".") {
			continue
		}
		vk, err := registry.OpenKey(registry.CURRENT_USER, assocKey(n[1:])+`\shell\`+Verb, registry.QUERY_VALUE)
		if err == nil {
			vk.Close()
			exts = append(exts, n[1:])
		}
	}
	return exts, nil
}

// deleteTree deletes a key with its subkeys; a missing key is no error.
func deleteTree(path string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, path, registry.ENUMERATE_SUB_KEYS)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	subs, err := k.ReadSubKeyNames(-1)
	k.Close()
	if err != nil {
		return err
	}
	for _, s := range subs {
		if err := deleteTree(path + `\` + s); err != nil {
			return err
		}
	}
	if err := registry.DeleteKey(registry.CURRENT_USER, path); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	return nil
}

var procSHChangeNotify = syscall.NewLazyDLL("shell32.dll").NewProc("SHChangeNotify")

// NotifyShell tells Explorer that file associations changed.
func NotifyShell() {
	const shcneAssocChanged, shcnfIDList = 0x08000000, 0
	procSHChangeNotify.Call(shcneAssocChanged, shcnfIDList, 0, 0)
}
