// 在窗口中创建 WebView
package main

import (
	_ "embed"
	"github.com/twgh/xcgui/app"
	"github.com/twgh/xcgui/window"
	"github.com/twgh/xcgui/xcc"
	"github.com/twgh/xwebview"
)

func main() {
	a := app.New(true)
	a.EnableAutoDPI(true).EnableDPI(true)

	w := window.New(0, 0, 1400, 900, "创建到窗口", 0, xcc.Window_Style_Default)
	w.SetBorderSize(1, 30, 1, 1)

	wv := xwebview.New(w.Handle, xwebview.XcWebViewOption{
		DataPath:   "D:\\cache\\wv",
		FillParent: true,
		Debug:      true,
	})
	wv.Navigate("https://panjiachen.github.io/vue-element-admin/#/login?redirect=%2Fexample%2Fcreate")

	w.Show(true)
	a.Run()
	a.Exit()
}
