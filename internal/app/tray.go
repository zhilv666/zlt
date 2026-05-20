package app

import (
	"context"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"tray/internal/buildinfo"
	"tray/internal/process"
	"github.com/getlantern/systray"
)

type trayTaskItem struct {
	taskID string
	item   *systray.MenuItem
}

type trayController struct {
	rt        *Runtime
	mu        sync.Mutex
	taskItems map[string]*trayTaskItem
}

func runTray(rt *Runtime) error {
	controller := &trayController{
		rt:        rt,
		taskItems: map[string]*trayTaskItem{},
	}

	systray.Run(func() {
		controller.onReady()
	}, func() {
		_ = rt.Shutdown(context.Background())
	})
	return nil
}

func (c *trayController) onReady() {
	systray.SetTitle("Tray Cmd")
	systray.SetTooltip("Tray Command Manager")

	openItem := systray.AddMenuItem("打开控制面板", "Open Dashboard")
	systray.AddSeparator()
	c.syncTaskMenus()
	systray.AddSeparator()
	aboutRoot := systray.AddMenuItem("About / Version", "Build information")
	c.initAboutMenu(aboutRoot)
	systray.AddSeparator()
	quitItem := systray.AddMenuItem("退出", "Quit the tray app")

	go func() {
		ticker := time.NewTicker(1500 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-openItem.ClickedCh:
				openBrowser(c.rt.Address())
			case <-ticker.C:
				c.syncTaskMenus()
			case <-quitItem.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()
}

func (c *trayController) syncTaskMenus() {
	c.mu.Lock()
	defer c.mu.Unlock()

	tasks := c.rt.ListTasks()
	seen := make(map[string]struct{}, len(tasks))

	for _, cfg := range tasks {
		seen[cfg.ID] = struct{}{}
		entry, ok := c.taskItems[cfg.ID]
		if !ok {
			item := systray.AddMenuItem("", "")
			entry = &trayTaskItem{
				taskID: cfg.ID,
				item:   item,
			}
			c.taskItems[cfg.ID] = entry

			go c.listenTaskItem(entry)
		}
		c.refreshTaskItem(cfg.ID, cfg.Name, entry)
		entry.item.Show()
	}

	for id, entry := range c.taskItems {
		if _, ok := seen[id]; !ok {
			entry.item.Hide()
		}
	}
}

func (c *trayController) listenTaskItem(entry *trayTaskItem) {
	go func() {
		for range entry.item.ClickedCh {
			state, ok := c.rt.Manager.State(entry.taskID)
			if ok && isTaskRunning(state.Status) {
				_ = c.rt.Manager.Stop(entry.taskID)
			} else {
				_ = c.rt.Manager.Start(entry.taskID)
			}
			c.mu.Lock()
			name := c.lookupTaskName(entry.taskID)
			c.refreshTaskItem(entry.taskID, name, entry)
			c.mu.Unlock()
		}
	}()
}

func (c *trayController) refreshTaskItem(taskID string, taskName string, entry *trayTaskItem) {
	state, ok := c.rt.Manager.State(taskID)
	if !ok || !isTaskRunning(state.Status) {
		entry.item.SetTitle("▶ 启动 " + taskName)
		entry.item.SetTooltip("Start " + taskName)
		entry.item.Enable()
		return
	}

	if state.Status == process.StatusStarting || state.Status == process.StatusStopping {
		entry.item.SetTitle("⏳ 处理中 " + taskName)
		entry.item.SetTooltip("Busy " + taskName)
		entry.item.Disable()
		return
	}

	entry.item.SetTitle("■ 停止 " + taskName)
	entry.item.SetTooltip("Stop " + taskName)
	entry.item.Enable()
}

func (c *trayController) lookupTaskName(taskID string) string {
	tasks := c.rt.ListTasks()
	for _, cfg := range tasks {
		if cfg.ID == taskID {
			return cfg.Name
		}
	}
	return taskID
}

func isTaskRunning(status string) bool {
	return status == process.StatusRunning || status == process.StatusStarting || status == process.StatusStopping
}

func (c *trayController) initAboutMenu(root *systray.MenuItem) {
	info := buildinfo.Current()

	titleItem := root.AddSubMenuItem("Tray Command Manager", "Application")
	titleItem.Disable()

	versionItem := root.AddSubMenuItem("Version: "+info.Version, "Version")
	versionItem.Disable()

	commitItem := root.AddSubMenuItem("Commit: "+info.Commit, "Commit")
	commitItem.Disable()

	platformItem := root.AddSubMenuItem("Platform: "+info.Platform, "Platform")
	platformItem.Disable()

	buildTimeItem := root.AddSubMenuItem("Build: "+info.BuildTime, "Build time")
	buildTimeItem.Disable()
}

func openBrowser(url string) {
	switch runtime.GOOS {
	case "windows":
		_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		_ = exec.Command("open", url).Start()
	default:
		_ = exec.Command("xdg-open", url).Start()
	}
}
