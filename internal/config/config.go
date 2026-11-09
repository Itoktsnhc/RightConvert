// Package config holds the app's settings, in settings.json beside the
// user's rules.toml, under %APPDATA%\RightConvert.
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"

	"github.com/rightconvert/rightconvert/internal/rules"
)

// Settings are what the user sets in the settings window.
type Settings struct {
	// FFmpeg is the path of ffmpeg.exe or of its folder; empty finds it.
	FFmpeg string `json:"ffmpeg,omitempty"`
	// Workers is how many files convert at once; 0 means half the CPUs.
	Workers int `json:"workers,omitempty"`
	// MenuTitle is the label of the context submenu.
	MenuTitle string `json:"menuTitle,omitempty"`
	// CloseWhenDone closes the queue window once every file converted
	// without errors.
	CloseWhenDone bool `json:"closeWhenDone,omitempty"`
}

// DefaultMenuTitle is the submenu's label when none is set.
const DefaultMenuTitle = "转换为"

// Dir is the directory of the settings and rules.
type Dir string

func (d Dir) SettingsPath() string { return filepath.Join(string(d), "settings.json") }
func (d Dir) RulesPath() string    { return filepath.Join(string(d), "rules.toml") }

// Load reads the settings; a missing file gives the defaults.
func (d Dir) Load() (Settings, error) {
	var s Settings
	data, err := os.ReadFile(d.SettingsPath())
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	return s, json.Unmarshal(data, &s)
}

// Save writes the settings.
func (d Dir) Save(s Settings) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(d.SettingsPath(), data)
}

// EnsureRules creates rules.toml from the template when it does not
// exist, and returns its path.
func (d Dir) EnsureRules() (string, error) {
	p := d.RulesPath()
	if _, err := os.Stat(p); err == nil {
		return p, nil
	}
	return p, writeFile(p, []byte(rules.Template))
}

// writeFile writes through a temporary file, so that a crash never leaves
// half a file.
func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// EffectiveWorkers is the number of conversions to run at once.
func (s Settings) EffectiveWorkers() int {
	if s.Workers > 0 {
		return s.Workers
	}
	return max(1, runtime.NumCPU()/2)
}

// Title is the submenu's label.
func (s Settings) Title() string {
	if s.MenuTitle != "" {
		return s.MenuTitle
	}
	return DefaultMenuTitle
}
