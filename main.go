// RightConvert converts audio and video files from Explorer's context menu
// with ffmpeg.
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/rightconvert/rightconvert/internal/config"
	"github.com/rightconvert/rightconvert/internal/shellmenu"
)

func main() {
	wd, _ := os.Getwd()
	cmd, err := parseArgs(os.Args[1:], wd)

	// --register and --unregister run without a window, from the installer.
	if err == nil && (cmd.Kind == "register" || cmd.Kind == "unregister") {
		os.Exit(headless(cmd))
	}

	// Explorer starts one process per selected file: the first one keeps
	// running and the others hand it their files.
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}

	appData, perr := mygo.App.Path(mygo.PathAppData)
	if perr != nil {
		log.Fatal(perr)
	}
	c := newCore(config.Dir(appData + string(os.PathSeparator) + "RightConvert"))

	mygo.App.OnSecondInstance(func(args []string, wd string) {
		// Explorer may start the others before this one is ready.
		mygo.App.WhenReady(func() {
			next, err := parseArgs(args, wd)
			if err != nil {
				c.addErrors = append(c.addErrors, err.Error())
				c.openQueue()
				return
			}
			c.dispatch(next)
		})
	})

	mygo.App.WhenReady(func() {
		c.locateFFmpeg(func() {
			pending := c.pending
			c.pending = nil
			for _, p := range pending {
				c.enqueue(p)
			}
			c.updateQueue()
		})
		if err != nil {
			c.addErrors = append(c.addErrors, err.Error())
			c.openQueue()
			return
		}
		c.dispatch(cmd)
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// headless registers or removes the context menu, for the installer.
func headless(cmd command) int {
	var err error
	if cmd.Kind == "unregister" {
		err = shellmenu.Unregister()
	} else {
		appData, perr := os.UserConfigDir()
		if perr != nil {
			fmt.Fprintln(os.Stderr, perr)
			return 1
		}
		c := newCore(config.Dir(appData + string(os.PathSeparator) + "RightConvert"))
		_, err = c.applyMenu()
	}
	shellmenu.NotifyShell()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

// dispatch acts on a command, on the main thread.
func (c *core) dispatch(cmd command) {
	switch cmd.Kind {
	case "settings":
		c.openSettings()
	case "convert":
		c.enqueue(cmd)
		c.openQueue()
	}
}

func (c *core) openSettings() {
	if c.settingsWin != nil {
		c.settingsWin.Restore()
		c.settingsWin.Focus()
		return
	}
	c.settingsUI = newSettingsView(c)
	c.settingsWin = mygo.NewWindow(mygo.WindowOptions{
		Title: "RightConvert 设置", Width: 720, Height: 620, MinWidth: 520, MinHeight: 420,
		StateKey: "settings",
		Content:  ui.View(c.settingsUI.view),
	})
	c.settingsWin.OnClosed(func() { c.settingsWin, c.settingsUI = nil, nil })
}

func (c *core) openQueue() {
	if c.queueWin != nil {
		c.updateQueue()
		c.queueWin.Restore()
		c.queueWin.Focus()
		return
	}
	c.queueView = &queueView{core: c, closeWhenDone: c.settings.CloseWhenDone}
	c.queueView.refresh()
	win := mygo.NewWindow(mygo.WindowOptions{
		Title: "RightConvert", Width: 640, Height: 460, MinWidth: 420, MinHeight: 300,
		StateKey: "queue",
		Content:  ui.View(c.queueView.view),
	})
	c.queueWin = win
	c.redrawMu.Lock()
	// Update does not wait for the main thread, so conversions never block
	// on the window.
	c.redraw = func() { win.Update(c.updateQueue) }
	c.redrawMu.Unlock()
	win.OnClose(func(e *mygo.CloseEvent) {
		if c.queue == nil || !c.queue.Active() {
			return
		}
		res, err := mygo.Dialog.Message(mygo.MessageOptions{
			Parent: win, Type: mygo.MessageQuestion, Title: "RightConvert",
			Message: "还有文件正在转换，关闭窗口会取消它们。", Buttons: []string{"取消转换并关闭", "继续转换"},
			CancelButton: 1,
		})
		if err != nil || res.Button != 0 {
			e.PreventDefault()
			return
		}
		c.queue.CancelAll()
	})
	win.OnClosed(func() {
		c.redrawMu.Lock()
		c.redraw = nil
		c.redrawMu.Unlock()
		c.queueWin, c.queueView = nil, nil
		c.addErrors = nil
		if c.queue != nil {
			c.queue.ClearFinished()
		}
	})
}

// updateQueue refreshes the queue window from the queue, on the main
// thread, and closes it when every file converted and the user asked so.
func (c *core) updateQueue() {
	if c.queueWin == nil {
		return
	}
	v := c.queueView
	v.refresh()
	done, failed, active := counts(v.jobs)
	if v.closeWhenDone && active == 0 && failed == 0 && done > 0 && len(c.addErrors) == 0 {
		c.queueWin.Close()
		return
	}
	c.queueWin.Invalidate()
}
