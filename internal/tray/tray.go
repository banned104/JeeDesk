package tray

import (
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/energye/systray"
)

// Callbacks defines the actions triggered by system tray interactions and menu items.
type Callbacks struct {
	OnToggle     func()
	OnSettings   func()
	OnOpenConfig func()
	OnReload     func()
	OnQuit       func()
}

var (
	startOnce sync.Once
	stopOnce  sync.Once
	started   atomic.Bool
)

// Start initializes and runs the Windows system tray icon and context menu on a
// dedicated OS-locked thread. Subsequent calls are no-ops.
func Start(cb Callbacks) {
	startOnce.Do(func() {
		started.Store(true)
		go func() {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()

			systray.Run(func() {
				onReady(cb)
			}, func() {})
		}()
	})
}

// Stop shuts down the system tray icon cleanly. Safe to call multiple times or
// when Start has not been called.
func Stop() {
	if !started.Load() {
		return
	}
	stopOnce.Do(func() {
		systray.Quit()
	})
}

func onReady(cb Callbacks) {
	systray.SetIcon(IconBytes())
	systray.SetTitle("JeeDesk")
	systray.SetTooltip("JeeDesk - 极简快速启动面板")

	systray.SetOnClick(func(_ systray.IMenu) {
		invokeAsync(cb.OnToggle)
	})
	systray.SetOnRClick(func(menu systray.IMenu) {
		if menu != nil {
			_ = menu.ShowMenu()
		}
	})

	bindMenuItem("显示 / 隐藏 JeeDesk", "切换显示或隐藏 JeeDesk 主面板", cb.OnToggle)
	bindMenuItem("设置", "打开 JeeDesk 设置", cb.OnSettings)
	systray.AddSeparator()
	bindMenuItem("打开 config.json", "在默认编辑器中打开配置文件", cb.OnOpenConfig)
	bindMenuItem("手动重载配置", "立即重新读取 config.json", cb.OnReload)
	systray.AddSeparator()
	bindMenuItem("退出程序", "退出 JeeDesk", cb.OnQuit)
}

func bindMenuItem(title, tooltip string, fn func()) *systray.MenuItem {
	item := systray.AddMenuItem(title, tooltip)
	item.Click(func() {
		invokeAsync(fn)
	})
	return item
}

func invokeAsync(fn func()) {
	if fn == nil {
		return
	}
	go fn()
}
