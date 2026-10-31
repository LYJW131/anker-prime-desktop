package app

import (
	"runtime"

	"github.com/egoist/mygo"

	"github.com/LYJW131/anker-prime-mac/locales"
)

// syncMenu installs the macOS menu bar when the sidebar's item would read
// otherwise: the standard App, Edit and Window menus, and View with Hide
// Sidebar or Show Sidebar under Control-Command-S above Enter Full Screen,
// as the system apps have it.
func (a *App) syncMenu() {
	if runtime.GOOS != "darwin" {
		return
	}
	shown := a.view.SidebarShown()
	if a.menuSet && shown == a.menuSidebar {
		return
	}
	a.menuSet, a.menuSidebar = true, shown
	label := locales.SidebarShow()
	if shown {
		label = locales.SidebarHide()
	}
	toggle := func(*mygo.MenuItem, *mygo.Window) { a.post(a.view.ToggleSidebar) }
	mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu},
		{Role: mygo.RoleEditMenu},
		{Label: locales.MenuView(), Submenu: []*mygo.MenuItem{
			{Label: label, Accelerator: "Ctrl+Cmd+S", Click: toggle},
			mygo.Separator(),
			{Role: mygo.RoleToggleFullScreen},
		}},
		{Role: mygo.RoleWindowMenu},
	}))
}
