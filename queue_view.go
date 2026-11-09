package main

import (
	"fmt"
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/rightconvert/rightconvert/internal/convert"
)

// queueView shows the files being converted.
type queueView struct {
	core *core
	jobs []convert.Job // a snapshot, refreshed on the main thread
	// closeWhenDone records that the window should close once the queue
	// finishes; set when it was opened from the context menu.
	closeWhenDone bool
}

func (v *queueView) refresh() {
	if v.core.queue != nil {
		v.jobs = v.core.queue.Snapshot()
	} else {
		v.jobs = nil
	}
}

// counts returns how many jobs are done, failed and still to go.
func counts(jobs []convert.Job) (done, failed, active int) {
	for _, j := range jobs {
		switch j.State {
		case convert.Done:
			done++
		case convert.Failed:
			failed++
		case convert.Queued, convert.Running:
			active++
		}
	}
	return
}

func (v *queueView) view(c *ui.Context) {
	t := c.Theme()
	done, failed, active := counts(v.jobs)
	ui.Column(c).Fill().Padding(16).Gap(12).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			title := "转换完成"
			switch {
			case v.core.locating && len(v.jobs) == 0:
				title = "正在查找 ffmpeg…"
			case active > 0:
				title = fmt.Sprintf("正在转换 %d 个文件", active)
			case len(v.jobs) == 0:
				title = "没有任务"
			}
			ui.Text(c, title).FontSize(18).Bold().Grow(1)
			if done > 0 {
				ui.Badge(c, fmt.Sprintf("成功 %d", done)).Background(t.Success).TextColor(t.AccentText)
			}
			if failed > 0 {
				ui.Badge(c, fmt.Sprintf("失败 %d", failed)).Background(t.Danger).TextColor(t.AccentText)
			}
		})
		for i, msg := range v.core.addErrors {
			c.Key(fmt.Sprintf("adderr-%d", i))
			ui.Text(c, msg).TextColor(t.Danger).MaxLines(3)
		}
		ui.List(c, nil, len(v.jobs), func(i int) { v.row(c, v.jobs[i]) }).
			ItemKey(func(i int) any { return v.jobs[i].ID }).Gap(6).Grow(1)
		ui.Row(c).Gap(8).Children(func() {
			if ui.Button(c, "设置…").Clicked() {
				v.core.openSettings()
			}
			ui.Spacer(c)
			if done+failed > 0 && ui.Button(c, "清除已完成").Clicked() {
				v.core.queue.ClearFinished()
				v.core.addErrors = nil
			}
			if active > 0 {
				if ui.Button(c, "全部取消").Clicked() {
					v.core.queue.CancelAll()
				}
			} else if ui.PrimaryButton(c, "关闭").Clicked() {
				v.core.queueWin.Close()
			}
		})
	})
}

func (v *queueView) row(c *ui.Context, j convert.Job) {
	t := c.Theme()
	ui.Column(c).Padding(10, 12).Gap(6).Radius(8).Background(t.Surface).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).Shrink(1).Gap(2).Children(func() {
				out := "（检测音轨格式…）"
				if j.Output != "" {
					out = filepath.Base(j.Output)
				}
				ui.Text(c, filepath.Base(j.Input)+"  →  "+out).MaxLines(1).Ellipsis("…")
				ui.Text(c, j.Rule.Label+" · "+filepath.Dir(j.Input)).FontSize(12).TextColor(t.TextMuted).MaxLines(1).Ellipsis("…")
			})
			switch j.State {
			case convert.Queued, convert.Running:
				if ui.Button(c, "取消").Clicked() {
					v.core.queue.Cancel(j.ID)
				}
			case convert.Done:
				if ui.Button(c, "打开位置").Clicked() {
					mygo.Shell.ShowItemInFolder(j.Output)
				}
			case convert.Failed:
				ui.Text(c, "失败").TextColor(t.Danger)
			case convert.Canceled:
				ui.Text(c, "已取消").TextColor(t.TextMuted)
			}
		})
		switch j.State {
		case convert.Queued:
			ui.Text(c, "排队中").FontSize(12).TextColor(t.TextMuted)
		case convert.Running:
			ui.Progress(c, j.Progress).Label(filepath.Base(j.Input))
		case convert.Failed:
			ui.Text(c, j.Err.Error()).FontSize(12).TextColor(t.Danger).MaxLines(4).Selectable()
		}
	})
}
