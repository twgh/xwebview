// 计算文件MD5.
// 不使用炫彩元素, 直接使用html文件作为窗口内容.
package main

import (
	"crypto/md5"
	_ "embed"
	"encoding/hex"
	"github.com/twgh/xcgui/app"
	"github.com/twgh/xcgui/wapi"
	"github.com/twgh/xcgui/wapi/wutil"
	"github.com/twgh/xcgui/window"
	"github.com/twgh/xcgui/xc"
	"github.com/twgh/xcgui/xcc"
	"github.com/twgh/xwebview"
	"os"
	"path/filepath"
)

var a *app.App

type MainWindow struct {
	w  *window.Window
	wv *xwebview.WebView
}

func NewMainWindow() *MainWindow {
	m := &MainWindow{}
	m.main()
	return m
}

func (m *MainWindow) main() {
	// 创建窗口
	m.w = window.New(0, 0, 550, 500, "计算文件MD5", 0, xcc.Window_Style_Center|xcc.Window_Style_Drag_Border|xcc.Window_Style_Allow_MaxWindow)
	// 设置窗口透明类型
	m.w.SetTransparentType(xcc.Window_Transparent_Shadow).SetTransparentAlpha(255)
	// 设置窗口阴影
	m.w.SetShadowInfo(8, 255, 0, false, 0)

	// 创建 webview
	m.wv = xwebview.New(m.w.Handle, xwebview.XcWebViewOption{
		DataPath:   os.TempDir(),
		FillParent: true,
		Debug:      true,
	})
	// 加载网页
	url, _ := filepath.Abs("./example/CalcMD5/index.html")
	url = "file:///" + url
	m.wv.Navigate(url)

	// 绑定函数
	m.bindBasicFuncs()
	m.bindFuncs()
	// 显示窗口
	m.w.Show(true)
}

// bindBasicFuncs 绑定基本函数.
func (m *MainWindow) bindBasicFuncs() {
	// 绑定 moveWindow
	m.wv.Bind("moveWindow", func(x int32, y int32) {
		// 减去阴影大小8
		wapi.SetWindowPos(m.w.GetHWND(), 0, m.w.DpiConv(x-8), m.w.DpiConv(y-8), 0, 0, wapi.SWP_NOSIZE|wapi.SWP_NOZORDER)
	})

	// 绑定 最小化窗口函数
	m.wv.Bind("minimizeWindow", func() {
		m.w.ShowWindow(xcc.SW_MINIMIZE)
	})

	// 绑定 切换最大化窗口函数
	m.wv.Bind("toggleMaximize", func() {
		m.w.MaxWindow(!m.w.IsMaxWindow())
	})

	// 绑定 关闭窗口函数
	m.wv.Bind("closeWindow", func() {
		m.w.CloseWindow()
	})
}

// bindFuncs 绑定函数.
func (m *MainWindow) bindFuncs() {
	// 绑定 goOpenFile
	m.wv.Bind("goOpenFile", func() string {
		return wutil.OpenFile(m.w.Handle, []string{"All Files(*.*)", "*.*"}, "")
	})

	// 绑定 calculateMD5
	m.wv.Bind("calculateMD5", func(filePath string) string {
		// 判断文件是否存在
		if !xc.PathExists2(filePath) {
			return "错误: 文件不存在"
		}
		// 读取文件内容
		data, err := os.ReadFile(filePath)
		if err != nil {
			return "错误: " + err.Error()
		}

		// 计算MD5
		hash := md5.Sum(data)
		md5Str := hex.EncodeToString(hash[:])

		// 返回结果（包含文件名和MD5）
		return "文件: " + filePath + "\nMD5: " + md5Str
	})
}

func main() {
	a = app.New(true)
	a.EnableAutoDPI(true).EnableDPI(true)

	NewMainWindow()

	a.Run()
	a.Exit()
}
