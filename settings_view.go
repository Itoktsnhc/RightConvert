package main

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/rightconvert/rightconvert/internal/config"
	"github.com/rightconvert/rightconvert/internal/rules"
	"github.com/rightconvert/rightconvert/internal/shellmenu"
)

// settingsView shows the rules, ffmpeg and the context menu.
type settingsView struct {
	core *core
	tab  int

	// edited copies of the settings, saved with Save
	ffmpegPath string
	workers    float64
	menuTitle  string

	registered []string // extensions with the menu
	status     string   // the result of the last action
	statusErr  bool
}

func newSettingsView(c *core) *settingsView {
	v := &settingsView{core: c}
	v.load()
	return v
}

func (v *settingsView) load() {
	s := v.core.settings
	v.ffmpegPath, v.workers, v.menuTitle = s.FFmpeg, float64(s.EffectiveWorkers()), s.Title()
	v.registered, _ = shellmenu.Registered()
}

func (v *settingsView) setStatus(err error, ok string) {
	if err != nil {
		v.status, v.statusErr = err.Error(), true
	} else {
		v.status, v.statusErr = ok, false
	}
}

func (v *settingsView) view(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Padding(16).Gap(12).Children(func() {
		ui.Text(c, "RightConvert").FontSize(22).Bold()
		ui.Tabs(c, &v.tab, "规则", "ffmpeg", "右键菜单")
		ui.Scroll(c).Grow(1).Gap(10).Children(func() {
			switch v.tab {
			case 0:
				v.rulesTab(c)
			case 1:
				v.ffmpegTab(c)
			case 2:
				v.menuTab(c)
			}
		})
		if v.status != "" {
			col := t.Success
			if v.statusErr {
				col = t.Danger
			}
			ui.Text(c, v.status).TextColor(col).MaxLines(4).Selectable()
		}
	})
}

func (v *settingsView) rulesTab(c *ui.Context) {
	t := c.Theme()
	core := v.core
	if core.rulesErr != nil {
		ui.Text(c, "rules.toml 有语法错误，已只使用内置规则：\n"+core.rulesErr.Error()).TextColor(t.Danger).Selectable()
	}
	for _, w := range core.rules.Warnings {
		ui.Text(c, "已跳过："+w).TextColor(t.Warning).Selectable()
	}
	ui.Row(c).Gap(8).Children(func() {
		if ui.PrimaryButton(c, "编辑 rules.toml").Clicked() {
			p, err := core.dir.EnsureRules()
			if err == nil {
				err = mygo.Shell.OpenPath(p)
			}
			v.setStatus(err, "已用默认程序打开 "+p+"，保存后点“重新加载”")
		}
		if ui.Button(c, "重新加载").Clicked() {
			core.reloadRules()
			v.setStatus(core.rulesErr, fmt.Sprintf("已加载 %d 条规则；点“应用到右键菜单”让资源管理器生效", len(core.rules.Rules)))
		}
		if ui.Button(c, "应用到右键菜单").Clicked() {
			v.apply()
		}
	})
	for _, r := range core.rules.Rules {
		c.Key("rule-" + r.ID)
		v.ruleRow(c, r)
	}
}

func (v *settingsView) ruleRow(c *ui.Context, r *rules.Rule) {
	t := c.Theme()
	ui.Column(c).Padding(10, 12).Gap(4).Radius(8).Background(t.Surface).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, r.Label).Bold()
			to := "→ ." + r.To
			if r.AutoTo() {
				to = "→ 原音轨格式"
			}
			ui.Text(c, to).TextColor(t.TextMuted)
			switch {
			case !r.IsEnabled():
				ui.Badge(c, "已停用")
			case r.Overridden:
				ui.Badge(c, "已自定义").Background(t.Accent).TextColor(t.AccentText)
			case !r.Builtin:
				ui.Badge(c, "自定义").Background(t.Accent).TextColor(t.AccentText)
			}
			if v.core.ffmpeg != nil {
				if m := v.core.ffmpeg.Missing(r); len(m) > 0 {
					ui.Badge(c, "缺少 "+strings.Join(m, ", ")).Background(t.Warning).TextColor(t.AccentText)
				}
			}
			ui.Spacer(c)
			ui.Text(c, r.ID).FontSize(12).TextColor(t.TextMuted)
		})
		ui.Text(c, "来源："+strings.Join(r.From, " ")).FontSize(12).TextColor(t.TextMuted)
		ui.Text(c, "ffmpeg "+strings.Join(r.Args, " ")).FontSize(12).Font("monospace").TextColor(t.TextMuted).Selectable()
	})
}

func (v *settingsView) ffmpegTab(c *ui.Context) {
	t := c.Theme()
	core := v.core
	switch {
	case core.locating:
		ui.Text(c, "正在查找 ffmpeg…")
	case core.ffmpeg != nil:
		ui.Text(c, "已找到："+core.ffmpeg.Path).Selectable()
		ui.Text(c, "版本："+core.ffmpeg.Version).TextColor(t.TextMuted).Selectable()
		if core.ffmpeg.Probe == "" {
			ui.Text(c, "未找到 ffprobe：转换可用，但不显示进度百分比").TextColor(t.Warning)
		}
	default:
		ui.Text(c, fmt.Sprint(core.ffmpegErr)).TextColor(t.Danger)
		ui.Link(c, "下载 ffmpeg（Windows 版本）", "https://www.gyan.dev/ffmpeg/builds/")
	}
	ui.Form(c, func() {
		ui.Field(c, "ffmpeg 路径", func() {
			ui.Row(c).Gap(8).Children(func() {
				ui.TextInput(c, &v.ffmpegPath).Placeholder("留空自动查找：程序目录 → PATH").Grow(1)
				if ui.Button(c, "浏览…").Clicked() {
					paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
						Parent: core.settingsWin, Title: "选择 ffmpeg.exe",
						Filters: []mygo.FileFilter{{Name: "ffmpeg", Extensions: []string{"exe"}}},
					})
					if err == nil && len(paths) == 1 {
						v.ffmpegPath = paths[0]
					}
				}
			})
		}).Description("可填 ffmpeg.exe 或它所在的文件夹")
		ui.Field(c, "同时转换", func() {
			ui.NumberInput(c, &v.workers, 1, 32, 1).Width(120)
		}).Description("同时运行的 ffmpeg 数量")
	})
	ui.Row(c).Gap(8).Children(func() {
		if ui.PrimaryButton(c, "保存并重新检测").Clicked() {
			core.settings.FFmpeg = strings.Trim(strings.TrimSpace(v.ffmpegPath), `"`)
			core.settings.Workers = int(v.workers)
			err := core.dir.Save(core.settings)
			v.setStatus(err, "已保存")
			core.locateFFmpeg(func() { core.settingsWin.Invalidate() })
		}
	})
}

func (v *settingsView) menuTab(c *ui.Context) {
	t := c.Theme()
	if len(v.registered) == 0 {
		ui.Text(c, "右键菜单尚未安装。")
	} else {
		ui.Text(c, fmt.Sprintf("已为 %d 种文件类型添加右键菜单：", len(v.registered)))
		ui.Text(c, strings.Join(v.registered, " ")).TextColor(t.TextMuted)
	}
	ui.Text(c, "菜单只写入当前用户的注册表（HKEY_CURRENT_USER），不需要管理员权限。Windows 11 中位于“显示更多选项”里。").
		FontSize(12).TextColor(t.TextMuted)
	ui.Form(c, func() {
		ui.Field(c, "菜单标题", func() {
			ui.TextInput(c, &v.menuTitle).Placeholder(config.DefaultMenuTitle).Width(240)
		})
	})
	ui.Row(c).Gap(8).Children(func() {
		if ui.PrimaryButton(c, "应用到右键菜单").Clicked() {
			v.apply()
		}
		if len(v.registered) > 0 && ui.Button(c, "移除右键菜单").Clicked() {
			err := shellmenu.Unregister()
			shellmenu.NotifyShell()
			v.registered, _ = shellmenu.Registered()
			v.setStatus(err, "已移除右键菜单")
		}
	})
}

// apply saves the menu title and writes the context menu.
func (v *settingsView) apply() {
	core := v.core
	title := strings.TrimSpace(v.menuTitle)
	if title == config.DefaultMenuTitle {
		title = ""
	}
	core.settings.MenuTitle = title
	if err := core.dir.Save(core.settings); err != nil {
		v.setStatus(err, "")
		return
	}
	n, err := core.applyMenu()
	v.registered, _ = shellmenu.Registered()
	v.setStatus(err, fmt.Sprintf("已为 %d 种文件类型更新右键菜单", n))
}
