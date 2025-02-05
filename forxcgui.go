package xwebview

import (
	"encoding/json"
	"github.com/twgh/xcgui/common"
	"github.com/twgh/xcgui/wapi"
	"github.com/twgh/xcgui/xc"
	"github.com/twgh/xcgui/xcc"
	"github.com/twgh/xwebview/pkg/edge"
	"log"
	"strconv"
	"syscall"
	"unsafe"
)

// XcWebViewOption 是给 xcgui 定制的 WebViewOption.
type XcWebViewOption struct {
	// WebView2 宿主窗口标题
	Title string
	// WebView2 宿主窗口类名
	ClassName string

	// DataPath 指定 WebView2 运行时用于浏览器实例的数据路径。
	DataPath string

	IconId uint

	// 左边
	Left int32
	// 顶边
	Top int32
	// 宽度
	Width int32
	// 高度
	Height int32

	// 填充父, 如果为true, 则 webView 会填满父窗口或元素, 固定坐标和尺寸会失效.
	FillParent bool

	// Debug 是否可开启开发者工具.
	Debug bool

	// AutoFocus 将在窗口获得焦点时尝试保持 webView 的焦点。
	AutoFocus bool
}

// New 创建 webview 窗口到炫彩窗口或元素, 失败返回nil.
//
// hParent: 炫彩窗口或元素句柄.
//
// opt: 选项.
func New(hParent int, opt XcWebViewOption) *WebView {
	if hParent < 1 {
		return nil
	}
	w := &WebView{}
	w.bindings = map[string]interface{}{}
	w.autofocus = opt.AutoFocus

	chromium := edge.NewChromium()
	chromium.MessageCallback = w.msgcb_xcgui
	chromium.DataPath = opt.DataPath
	chromium.SetPermission(edge.CoreWebView2PermissionKindClipboardRead, edge.CoreWebView2PermissionStateAllow)

	w.browser = chromium
	if !w.createWithOptionsByXcgui(hParent, opt) {
		return nil
	}

	settings, err := chromium.GetSettings()
	if err != nil {
		log.Fatal(err)
	}
	// disable context menu
	err = settings.PutAreDefaultContextMenusEnabled(opt.Debug)
	if err != nil {
		log.Fatal(err)
	}
	// disable developer tools
	err = settings.PutAreDevToolsEnabled(opt.Debug)
	if err != nil {
		log.Fatal(err)
	}

	return w
}

// createWithOptionsByXcgui 创建webview宿主窗口.
func (w *WebView) createWithOptionsByXcgui(hParent int, opt XcWebViewOption) bool {
	w.hParent = hParent
	// 获取父窗口或元素的HWND
	var hWnd uintptr
	var isWindow bool
	if w.hParent > 0 {
		if xc.XC_IsHWINDOW(w.hParent) {
			isWindow = true
			hWnd = xc.XWnd_GetHWND(w.hParent)
		} else if xc.XC_IsHELE(w.hParent) {
			hWnd = xc.XWidget_GetHWND(w.hParent)
			w.hWindow = xc.XWidget_GetHWINDOW(w.hParent)
		}
	}

	dpi := int32(96)
	if w.hWindow > 0 {
		dpi = xc.XWnd_GetDPI(w.hWindow)
		// 启用自动焦点
		xc.XWnd_EnableAutoFocus(w.hWindow, true)
	}

	hInstance := wapi.GetModuleHandleEx(0, "")

	var icon uintptr
	if opt.IconId == 0 {
		// load default icon
		icow := wapi.GetSystemMetrics(wapi.SM_CXICON)
		icoh := wapi.GetSystemMetrics(wapi.SM_CYICON)
		icon = wapi.LoadImageW(hInstance, uintptr(32512), wapi.IMAGE_ICON, icow, icoh, wapi.LR_DEFAULTCOLOR)
	} else {
		// load icon from resource
		icon = wapi.LoadImageW(hInstance, uintptr(opt.IconId), wapi.IMAGE_ICON, 0, 0, wapi.LR_DEFAULTSIZE|wapi.LR_SHARED)
	}

	// 注册窗口类名
	if opt.ClassName == "" {
		opt.ClassName = "XWebview"
	}
	wc := wapi.WNDCLASSEX{
		Style:         wapi.CS_HREDRAW | wapi.CS_VREDRAW,
		CbSize:        uint32(unsafe.Sizeof(wapi.WNDCLASSEX{})),
		HInstance:     hInstance,
		LpszClassName: common.StrPtr(opt.ClassName),
		HIcon:         icon,
		HIconSm:       icon,
		LpfnWndProc:   syscall.NewCallback(wndproc),
	}
	wapi.RegisterClassEx(&wc)

	// 窗口坐标和宽高
	var left, top, width, height = xc.DpiConv(dpi, opt.Left), xc.DpiConv(dpi, opt.Top), xc.DpiConv(dpi, opt.Width), xc.DpiConv(dpi, opt.Height)

	// 创建宿主窗口
	w.hwnd = wapi.CreateWindowEx(0, opt.ClassName, opt.Title, xcc.WS_MINIMIZE, left, top, width, height, hWnd, 0, hInstance, 0)

	setWindowContext(w.hwnd, w)
	setWindowContext(hWnd, w)

	// 显示窗口, 更新窗口, 设置焦点
	wapi.ShowWindow(w.hwnd, xcc.SW_SHOW)
	wapi.UpdateWindow(w.hwnd)
	wapi.SetFocus(w.hwnd)

	if !w.browser.Embed(w.hwnd) {
		return false
	}
	w.browser.Resize()

	// 设置 WebView2 宿主窗口为炫彩父窗口或元素的子窗口
	wapi.SetParent(w.hwnd, hWnd)
	// 设置 WebView2 宿主窗口样式
	wapi.SetWindowLongPtrW(w.hwnd, wapi.GWL_STYLE, int(xcc.WS_CHILD|xcc.WS_VISIBLE))

	// 更新 WebView2 宿主窗口大小和尺寸
	w.updateWebviewSize = func() {
		if !wapi.IsWindow(w.hwnd) {
			return
		}
		var rc xc.RECT
		if isWindow {
			xc.XWnd_GetClientRect(w.hParent, &rc)
		} else {
			xc.XEle_GetWndClientRect(w.hParent, &rc)
		}
		// 填充父
		if opt.FillParent {
			left = xc.DpiConv(dpi, rc.Left)
			top = xc.DpiConv(dpi, rc.Top)
			width = xc.DpiConv(dpi, rc.Right-rc.Left)
			height = xc.DpiConv(dpi, rc.Bottom-rc.Top)
		}

		w.SetSize(int(width), int(height), HintFixed)
		wapi.MoveWindow(w.hwnd, left, top, width, height, false)

		if isWindow {
			xc.XWnd_AdjustLayout(w.hParent)
		} else {
			xc.XEle_AdjustLayout(w.hParent, 0)
		}
	}

	// 窗口 调整位置和大小
	xc.XWnd_RemoveEventC(w.hWindow, xcc.WM_SIZE, onWndSize)
	xc.XWnd_RegEventC1(w.hWindow, xcc.WM_SIZE, onWndSize)

	// 元素事件
	if !isWindow {
		// 调整位置和大小
		xc.XEle_RemoveEventC(w.hParent, xcc.XE_SIZE, onEleSize)
		xc.XEle_RegEventC1(w.hParent, xcc.XE_SIZE, onEleSize)

		// 跟随父销毁
		xc.XEle_RemoveEventC(w.hParent, xcc.XE_DESTROY, onEleDestroy)
		xc.XEle_RegEventC1(w.hParent, xcc.XE_DESTROY, onEleDestroy)

		// 跟随父显示或隐藏
		xc.XEle_RemoveEventC(w.hParent, xcc.XE_SHOW, onEleShow)
		xc.XEle_RegEventC1(w.hParent, xcc.XE_SHOW, onEleShow)
	}
	return true
}

func onEleDestroy(hEle int, pbHandled *bool) int {
	if w, ok := getWindowContext(xc.XWidget_GetHWND(hEle)).(*WebView); ok {
		if wapi.IsWindow(w.hwnd) {
			w.Destroy()
		}
	}
	return 0
}

func onEleSize(hEle int, nFlags xcc.AdjustLayout_, nAdjustNo uint32, pbHandled *bool) int {
	if w, ok := getWindowContext(xc.XWidget_GetHWND(hEle)).(*WebView); ok {
		w.updateWebviewSize()
	}
	return 0
}

func onWndSize(hWindow int, nFlags uint, pPt *xc.SIZE, pbHandled *bool) int {
	if w, ok := getWindowContext(xc.XWnd_GetHWND(hWindow)).(*WebView); ok {
		w.updateWebviewSize()
	}
	return 0
}

func onEleShow(hEle int, bShow bool, pbHandled *bool) int {
	if w, ok := getWindowContext(xc.XWidget_GetHWND(hEle)).(*WebView); ok {
		if !wapi.IsWindow(w.hwnd) {
			return 0
		}
		nCmdShow := xcc.SW_SHOW
		if !bShow {
			nCmdShow = xcc.SW_HIDE
		}
		wapi.ShowWindow(w.hwnd, nCmdShow)
	}
	return 0
}

func (w *WebView) msgcb_xcgui(msg string) {
	d := rpcMessage{}
	if err := json.Unmarshal([]byte(msg), &d); err != nil {
		log.Printf("invalid RPC message: %v", err)
		return
	}

	id := strconv.Itoa(d.ID)
	if res, err := w.callbinding(d); err != nil {
		xc.XC_CallUT(func() {
			w.Eval("window._rpc[" + id + "].reject(" + jsString(err.Error()) + "); window._rpc[" + id + "] = undefined")
		})
	} else if b, err := json.Marshal(res); err != nil {
		xc.XC_CallUT(func() {
			w.Eval("window._rpc[" + id + "].reject(" + jsString(err.Error()) + "); window._rpc[" + id + "] = undefined")
		})
	} else {
		xc.XC_CallUT(func() {
			w.Eval("window._rpc[" + id + "].resolve(" + string(b) + "); window._rpc[" + id + "] = undefined")
		})
	}
}
