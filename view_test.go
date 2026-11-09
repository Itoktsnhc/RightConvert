package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/rightconvert/rightconvert/internal/config"
	"github.com/rightconvert/rightconvert/internal/convert"
	"github.com/rightconvert/rightconvert/internal/rules"
)

func TestQueueViewShowsJobs(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "song.flac")
	if err := os.WriteFile(in, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	c := newCore(config.Dir(dir))
	release := make(chan struct{})
	c.queue = convert.NewQueue(func(ctx context.Context, r *rules.Rule, input, output string, p func(float64)) error {
		p(0.5)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}, 1, nil)
	if _, err := c.queue.Add(c.rules.Get("to-mp3"), in); err != nil {
		t.Fatal(err)
	}
	c.addErrors = []string{"规则 \"gone\" 不存在"}
	v := &queueView{core: c}
	v.refresh()
	tt := ui.NewTester(v.view, 640, 460)
	for _, want := range []string{"正在转换 1 个文件", "song.flac  →  song.mp3", "全部取消", "规则 \"gone\" 不存在"} {
		if !tt.HasText(want) {
			t.Errorf("no %q in %q", want, tt.Texts())
		}
	}
	close(release)
	c.queue.Wait()
	v.refresh()
	tt.Frame()
	for _, want := range []string{"转换完成", "成功 1", "打开位置", "清除已完成", "关闭"} {
		if !tt.HasText(want) {
			t.Errorf("after finishing, no %q in %q", want, tt.Texts())
		}
	}
	if err := tt.Click("清除已完成"); err != nil {
		t.Fatal(err)
	}
	v.refresh()
	tt.Frame()
	if !tt.HasText("没有任务") {
		t.Errorf("after clearing: %q", tt.Texts())
	}
}

func TestSettingsViewTabs(t *testing.T) {
	c := newCore(config.Dir(t.TempDir()))
	v := &settingsView{core: c}
	v.load()
	tt := ui.NewTester(v.view, 720, 620)
	if !tt.HasText("MP3 (320k)") || !tt.HasText("编辑 rules.toml") {
		t.Errorf("rules tab: %q", tt.Texts())
	}
	if err := tt.Click("ffmpeg"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("保存并重新检测") {
		t.Errorf("ffmpeg tab: %q", tt.Texts())
	}
	if err := tt.Click("右键菜单"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("应用到右键菜单") {
		t.Errorf("menu tab: %q", tt.Texts())
	}
}
