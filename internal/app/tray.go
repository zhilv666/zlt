//go:build windows

package app

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/getlantern/systray"
	rootassets "zhulingtai"
	"zhulingtai/internal/buildinfo"
	"zhulingtai/internal/process"
	"zhulingtai/internal/task"
)

type trayTaskItem struct {
	taskID      string
	controlItem *systray.MenuItem
}

type trayController struct {
	rt        *Runtime
	mu        sync.Mutex
	taskRoot  *systray.MenuItem
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
	if len(rootassets.TrayIcon) > 0 {
		systray.SetIcon(rootassets.TrayIcon)
	}
	systray.SetTitle("驻令台")
	systray.SetTooltip("驻令台")

	openItem := systray.AddMenuItem("打开控制面板", "Open Dashboard")
	systray.AddSeparator()
	c.taskRoot = systray.AddMenuItem("任务", "Task controls")
	c.syncTaskMenus()
	systray.AddSeparator()
	c.initVersionMenu()
	systray.AddSeparator()
	quitItem := systray.AddMenuItem("退出", "Quit the app")

	go func() {
		if err := c.rt.StartAutoStartTasks(); err != nil {
			log.Printf("zlt autostart failed: %v", err)
		}
		c.syncTaskMenus()
	}()

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
				quitItem.Disable()
				quitItem.SetTitle("退出中...")
				log.Printf("zlt quit: stopping tasks before exit")
				_ = c.rt.Shutdown(context.Background())
				log.Printf("zlt quit: shutdown complete, quitting systray")
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
			controlItem := c.taskRoot.AddSubMenuItem("", "")
			entry = &trayTaskItem{
				taskID:      cfg.ID,
				controlItem: controlItem,
			}
			c.taskItems[cfg.ID] = entry

			go c.listenTaskControl(entry)
		}
		c.refreshTaskItem(cfg, entry)
		entry.controlItem.Show()
	}

	for id, entry := range c.taskItems {
		if _, ok := seen[id]; !ok {
			entry.controlItem.Hide()
		}
	}
}

func (c *trayController) listenTaskControl(entry *trayTaskItem) {
	go func() {
		for range entry.controlItem.ClickedCh {
			state, ok := c.rt.Manager.State(entry.taskID)
			if ok && isTaskRunning(state.Status) {
				_ = c.rt.Manager.Stop(entry.taskID)
			} else {
				_ = c.rt.Manager.Start(entry.taskID)
			}
			c.mu.Lock()
			cfg, ok := c.lookupTask(entry.taskID)
			if ok {
				c.refreshTaskItem(cfg, entry)
			}
			c.mu.Unlock()
		}
	}()
}

func (c *trayController) refreshTaskItem(cfg task.Config, entry *trayTaskItem) {
	state, ok := c.rt.Manager.State(cfg.ID)
	if !ok || !isTaskRunning(state.Status) {
		entry.controlItem.SetTitle("▶ 启动 " + cfg.Name)
		entry.controlItem.SetTooltip("Start " + cfg.Name)
		entry.controlItem.Enable()
	} else if state.Status == process.StatusStarting || state.Status == process.StatusStopping {
		entry.controlItem.SetTitle("⏳ 处理中 " + cfg.Name)
		entry.controlItem.SetTooltip("Busy " + cfg.Name)
		entry.controlItem.Disable()
	} else {
		entry.controlItem.SetTitle("■ 停止 " + cfg.Name)
		entry.controlItem.SetTooltip("Stop " + cfg.Name)
		entry.controlItem.Enable()
	}
}

func (c *trayController) lookupTask(taskID string) (task.Config, bool) {
	tasks := c.rt.ListTasks()
	for _, cfg := range tasks {
		if cfg.ID == taskID {
			return cfg, true
		}
	}
	return task.Config{}, false
}

func isTaskRunning(status string) bool {
	return status == process.StatusRunning || status == process.StatusStarting || status == process.StatusStopping
}

func (c *trayController) initVersionMenu() {
	info := buildinfo.Current()
	_ = systray.AddMenuItem("Version: "+buildinfo.DisplayVersion(info.Version), "Version")
}
