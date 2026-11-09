package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/egoist/mygo"

	"github.com/rightconvert/rightconvert/internal/config"
	"github.com/rightconvert/rightconvert/internal/convert"
	"github.com/rightconvert/rightconvert/internal/rules"
	"github.com/rightconvert/rightconvert/internal/shellmenu"
)

// core is the app's state outside the windows. Its fields are read and
// written on the main thread, except where noted.
type core struct {
	dir      config.Dir
	settings config.Settings
	rules    *rules.Set
	rulesErr error // a syntax error in rules.toml

	ffmpeg    *convert.FFmpeg // nil until located, or when not found
	ffmpegErr error
	locating  bool

	queue *convert.Queue // safe from any goroutine
	// errors of files that could not be queued, as a missing rule
	addErrors []string
	// commands that arrived before ffmpeg was located
	pending []command

	queueWin    *mygo.Window
	settingsWin *mygo.Window
	queueView   *queueView
	settingsUI  *settingsView

	// redraw is called from any goroutine when the queue changes.
	redrawMu sync.Mutex
	redraw   func()
}

func newCore(dir config.Dir) *core {
	c := &core{dir: dir}
	c.settings, _ = dir.Load()
	c.reloadRules()
	return c
}

func (c *core) reloadRules() {
	c.rules, c.rulesErr = rules.Load(c.dir.RulesPath())
}

// exe is the path written into the context menu.
func exe() string {
	p, err := os.Executable()
	if err != nil {
		return "RightConvert.exe"
	}
	if r, err := filepath.EvalSymlinks(p); err == nil {
		p = r
	}
	return p
}

// locateFFmpeg finds ffmpeg in the background, then runs done on the main
// thread.
func (c *core) locateFFmpeg(done func()) {
	c.locating = true
	configured := c.settings.FFmpeg
	go func() {
		f, err := convert.Locate(context.Background(), configured, filepath.Dir(exe()))
		mygo.RunOnMain(func() {
			c.ffmpeg, c.ffmpegErr, c.locating = f, err, false
			c.queue = nil // a new ffmpeg, a new runner
			if done != nil {
				done()
			}
		})
	}()
}

// usable reports whether the located ffmpeg runs the rule.
func (c *core) usable(r *rules.Rule) bool {
	return c.ffmpeg == nil || len(c.ffmpeg.Missing(r)) == 0
}

func (c *core) ensureQueue() *convert.Queue {
	if c.queue == nil {
		f := c.ffmpeg
		c.queue = convert.NewQueue(f.Run, c.settings.EffectiveWorkers(), func() {
			c.redrawMu.Lock()
			fn := c.redraw
			c.redrawMu.Unlock()
			if fn != nil {
				fn()
			}
		})
		c.queue.ResolveExt = f.AudioExt
	}
	return c.queue
}

// enqueue adds a convert command's files to the queue, on the main thread.
func (c *core) enqueue(cmd command) {
	if c.ffmpeg == nil {
		if c.locating {
			c.pending = append(c.pending, cmd)
			return
		}
		c.addErrors = append(c.addErrors, fmt.Sprint(c.ffmpegErr))
		return
	}
	r := c.rules.Get(cmd.Rule)
	switch {
	case r == nil:
		c.addErrors = append(c.addErrors, fmt.Sprintf("规则 %q 不存在（可能已被删除），请在设置中重新应用右键菜单", cmd.Rule))
		return
	case !r.IsEnabled():
		c.addErrors = append(c.addErrors, fmt.Sprintf("规则 %q 已停用", r.Label))
		return
	}
	if m := c.ffmpeg.Missing(r); len(m) > 0 {
		c.addErrors = append(c.addErrors, fmt.Sprintf("当前 ffmpeg 缺少编码器 %s，无法执行 %q", strings.Join(m, ", "), r.Label))
		return
	}
	q := c.ensureQueue()
	for _, f := range cmd.Files {
		if _, err := q.Add(r, f); err != nil {
			c.addErrors = append(c.addErrors, fmt.Sprintf("%s: %v", filepath.Base(f), err))
		}
	}
}

// applyMenu writes the context menu for the current rules.
func (c *core) applyMenu() (int, error) {
	n, err := shellmenu.Register(c.rules, shellmenu.Options{Exe: exe(), Title: c.settings.Title(), Usable: c.usable})
	shellmenu.NotifyShell()
	return n, err
}
