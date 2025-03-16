package xwebview

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/twgh/xcgui/wapi"
	"github.com/twgh/xcgui/xc"
	"github.com/twgh/xcgui/xcc"
	"github.com/twgh/xwebview/pkg/edge"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/twgh/xwebview/internal/w32"
	"golang.org/x/sys/windows"
)

var (
	windowContext     = map[uintptr]interface{}{}
	windowContextSync sync.RWMutex
)

func getWindowContext(wnd uintptr) interface{} {
	windowContextSync.RLock()
	defer windowContextSync.RUnlock()
	return windowContext[wnd]
}

func setWindowContext(wnd uintptr, data interface{}) {
	windowContextSync.Lock()
	defer windowContextSync.Unlock()
	windowContext[wnd] = data
}

type WebView struct {
	hwnd      uintptr
	browser   *edge.Chromium
	autofocus bool
	maxsz     w32.Point
	minsz     w32.Point
	m         sync.Mutex
	bindings  map[string]interface{}

	hWindow           int // 炫彩窗口句柄
	hParent           int
	updateWebviewSize func()
	evalCallbackMux   sync.Mutex
	callbackID        int
}

// Hint 用于配置窗口大小和调整大小的行为。
type Hint int

const (
	// HintNone 指定宽度和高度为默认大小
	HintNone Hint = iota

	// HintFixed 指定窗口大小不能被用户改变
	HintFixed

	// HintMin 指定宽度和高度为最小界限
	HintMin

	// HintMax 指定宽度和高度为最大界限
	HintMax
)

type rpcMessage struct {
	ID     int               `json:"id"`
	Method string            `json:"method"`
	Params []json.RawMessage `json:"params"`
}

func jsString(v interface{}) string { b, _ := json.Marshal(v); return string(b) }

func (w *WebView) callbinding(d rpcMessage) (interface{}, error) {
	w.m.Lock()
	f, ok := w.bindings[d.Method]
	w.m.Unlock()
	if !ok {
		return nil, nil
	}

	v := reflect.ValueOf(f)
	isVariadic := v.Type().IsVariadic()
	numIn := v.Type().NumIn()
	if (isVariadic && len(d.Params) < numIn-1) || (!isVariadic && len(d.Params) != numIn) {
		return nil, errors.New("function arguments mismatch")
	}
	args := make([]reflect.Value, 0)
	for i := range d.Params {
		var arg reflect.Value
		if isVariadic && i >= numIn-1 {
			arg = reflect.New(v.Type().In(numIn - 1).Elem())
		} else {
			arg = reflect.New(v.Type().In(i))
		}
		if err := json.Unmarshal(d.Params[i], arg.Interface()); err != nil {
			return nil, err
		}
		args = append(args, arg.Elem())
	}

	errorType := reflect.TypeOf((*error)(nil)).Elem()
	res := v.Call(args)
	switch len(res) {
	case 0:
		// No results from the function, just return nil
		return nil, nil

	case 1:
		// One result may be a value, or an error
		if res[0].Type().Implements(errorType) {
			if res[0].Interface() != nil {
				return nil, res[0].Interface().(error)
			}
			return nil, nil
		}
		return res[0].Interface(), nil

	case 2:
		// Two results: first one is value, second is error
		if !res[1].Type().Implements(errorType) {
			return nil, errors.New("second return value must be an error")
		}
		if res[1].Interface() == nil {
			return res[0].Interface(), nil
		}
		return res[0].Interface(), res[1].Interface().(error)

	default:
		return nil, errors.New("unexpected number of return values")
	}
}

func wndproc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	if w, ok := getWindowContext(hwnd).(*WebView); ok {
		switch msg {
		case wapi.WM_MOVE, wapi.WM_MOVING:
			_ = w.browser.NotifyParentWindowPositionChanged()
		case wapi.WM_SIZE:
			w.browser.Resize()
		case wapi.WM_ACTIVATE:
			if wp == w32.WAInactive {
				break
			}
			if w.autofocus {
				w.browser.Focus()
			}
		case wapi.WM_CLOSE:
			wapi.DestroyWindow(hwnd)
			// 移除事件
			xc.XWnd_RemoveEventC(w.hWindow, xcc.WM_SIZE, onWndSize)
			xc.XEle_RemoveEventC(w.hParent, xcc.XE_SIZE, onEleSize)
			xc.XEle_RemoveEventC(w.hParent, xcc.XE_SHOW, onEleShow)
			xc.XEle_RemoveEventC(w.hParent, xcc.XE_DESTROY, onEleDestroy)
		case wapi.WM_GETMINMAXINFO:
			lpmmi := (*w32.MinMaxInfo)(unsafe.Pointer(lp))
			if w.maxsz.X > 0 && w.maxsz.Y > 0 {
				lpmmi.PtMaxSize = w.maxsz
				lpmmi.PtMaxTrackSize = w.maxsz
			}
			if w.minsz.X > 0 && w.minsz.Y > 0 {
				lpmmi.PtMinTrackSize = w.minsz
			}
		default:
			r := wapi.DefWindowProc(hwnd, msg, wp, lp)
			return r
		}
		return 0
	}
	r := wapi.DefWindowProc(hwnd, msg, wp, lp)
	return r
}

// Destroy 销毁一个 webview 并关闭原生窗口。
func (w *WebView) Destroy() {
	wapi.PostMessageW(w.hwnd, wapi.WM_CLOSE, 0, 0)
}

// Navigate 导航 webview 到给定的 URL。URL 可能是数据 URI，即
// "data:text/text,<html>...</html>"。通常不进行适当的 url 编码也是可以的，
// webview 会为你重新编码。
func (w *WebView) Navigate(url string) {
	w.browser.Navigate(url)
}

// SetHtml 直接设置 webview 的 HTML。
// 页面的来源是 `about:blank`。
func (w *WebView) SetHtml(html string) {
	w.browser.NavigateToString(html)
}

// SetTitle 更新原生窗口的标题。必须从 UI 线程调用。
func (w *WebView) SetTitle(title string) {
	_title, err := windows.UTF16FromString(title)
	if err != nil {
		_title, _ = windows.UTF16FromString("")
	}
	_, _, _ = w32.User32SetWindowTextW.Call(w.hwnd, uintptr(unsafe.Pointer(&_title[0])))
}

// SetSize 更新原生窗口大小。参见 Hint 常量。
func (w *WebView) SetSize(width int, height int, hints Hint) {
	style := wapi.GetWindowLongPtrW(w.hwnd, wapi.GWL_STYLE)
	if hints == HintFixed {
		style &^= w32.WSThickFrame | w32.WSMaximizeBox
	} else {
		style |= w32.WSThickFrame | w32.WSMaximizeBox
	}
	wapi.SetWindowLongPtrW(w.hwnd, wapi.GWL_STYLE, style)

	if hints == HintMax {
		w.maxsz.X = int32(width)
		w.maxsz.Y = int32(height)
	} else if hints == HintMin {
		w.minsz.X = int32(width)
		w.minsz.Y = int32(height)
	} else {
		r := w32.Rect{}
		r.Left = 0
		r.Top = 0
		r.Right = int32(width)
		r.Bottom = int32(height)
		_, _, _ = w32.User32AdjustWindowRect.Call(uintptr(unsafe.Pointer(&r)), w32.WSOverlappedWindow, 0)
		_, _, _ = w32.User32SetWindowPos.Call(
			w.hwnd, 0, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right-r.Left), uintptr(r.Bottom-r.Top),
			w32.SWPNoZOrder|w32.SWPNoActivate|w32.SWPNoMove|w32.SWPFrameChanged)
		w.browser.Resize()
	}
}

// Init 在新页面初始化时注入 JavaScript 代码。每次
// webview 将打开一个新页面 - 此初始化代码将被执行。保证代码在 window.onload 之前执行。
func (w *WebView) Init(js string) {
	w.browser.Init(js)
}

// Eval 执行 JS 代码(异步). 必须在UI线程执行.
func (w *WebView) Eval(js string) {
	w.browser.Eval(js)
}

// Bind 绑定一个Go函数，使其以给定的名称
// 作为全局 JavaScript 函数出现。内部使用 webview_init()。必须在UI线程执行.
//
// f 必须是一个函数:
//   - 函数参数没什么限制
//   - 函数返回值可以是一个值或一个error
//   - 函数返回值可以是一个值和一个error
func (w *WebView) Bind(name string, f interface{}) error {
	v := reflect.ValueOf(f)
	if v.Kind() != reflect.Func {
		return errors.New("only functions can be bound")
	}
	if n := v.Type().NumOut(); n > 2 {
		return errors.New("function may only return a value or a value+error")
	}
	w.m.Lock()
	w.bindings[name] = f
	w.m.Unlock()

	initCode := "(function() { var name = " + jsString(name) + ";" + `
		var RPC = window._rpc = (window._rpc || {nextSeq: 1});
		window[name] = function() {
		  var seq = RPC.nextSeq++;
		  var promise = new Promise(function(resolve, reject) {
			RPC[seq] = {
			  resolve: resolve,
			  reject: reject,
			};
		  });
		  window.external.invoke(JSON.stringify({
			id: seq,
			method: name,
			params: Array.prototype.slice.call(arguments),
		  }));
		  return promise;
		}
	})()`

	w.browser.Eval(initCode)
	w.Init(initCode)
	return nil
}

// GetHWND 返回 webview 所在的原生窗口句柄.
func (w *WebView) GetHWND() uintptr {
	return w.hwnd
}

func (w *WebView) GetBrowser() *edge.Chromium {
	return w.browser
}

// EvalAsync 执行 js 代码, 可在回调函数中异步获取结果. 必须在UI线程执行.
//
// js: js 代码.
//
// f: 可在回调函数中获取js代码执行结果以及错误, 为 nil 时, 等同于执行了 Eval. 注意这个回调函数是在协程中执行的, 不是在UI线程.
//
// timeout: 超时时间, 为空默认10秒.
func (w *WebView) EvalAsync(js string, f func(result interface{}, err error), timeout ...time.Duration) error {
	if f == nil {
		w.browser.Eval(js)
		return nil
	}

	resultChan := make(chan interface{}, 1)
	w.evalCallbackMux.Lock()
	w.callbackID++
	callbackName := "__go_eval2_cb_" + strconv.Itoa(w.callbackID)
	w.evalCallbackMux.Unlock()

	// 绑定临时回调
	if err := w.Bind(callbackName, func(r interface{}) {
		resultChan <- r
		// 删除绑定的js函数
		w.browser.Eval(fmt.Sprintf("delete window.%s;", callbackName))
	}); err != nil {
		return err
	}

	// 执行脚本
	wrappedJS := fmt.Sprintf(`
	    (function() {
	        try {
	            const result = (%s);
	            if (result instanceof Promise) {
	                result.then(
	                    res => window.%s(res),
	                    err => window.%s({ error: err.message })
	                );
	            } else {
	                window.%s(result);
	            }
	        } catch (e) {
	            window.%s({ error: e.message });
	        }
	    })();
	`, strings.TrimRight(js, ";"), callbackName, callbackName, callbackName, callbackName)

	w.browser.Eval(wrappedJS)

	// 超时时间默认10秒
	t := 10 * time.Second
	if len(timeout) > 0 {
		t = timeout[0]
	}

	// 异步处理结果
	go func() {
		select {
		case ret := <-resultChan:
			close(resultChan)
			if errMap, ok := ret.(map[string]interface{}); ok && errMap["error"] != nil {
				f(nil, fmt.Errorf("JS错误: %v", errMap["error"]))
			} else {
				f(ret, nil)
			}
		case <-time.After(t):
			close(resultChan)
			f(nil, ErrEvalTimeout)
		}
	}()
	return nil
}

var (
	// ErrEvalTimeout 是执行 js 代码超时.
	ErrEvalTimeout = errors.New("执行超时")
)

// EvalSync 执行 js 代码, 同步取回返回值. 必须在UI线程执行.
//
// js: js 代码.
//
// timeout: 超时时间, 为空默认10秒.
func (w *WebView) EvalSync(js string, timeout ...time.Duration) (interface{}, error) {
	resultChan := make(chan interface{}, 1)
	w.evalCallbackMux.Lock()
	w.callbackID++
	callbackName := "__go_EvalSync_cb_" + strconv.Itoa(w.callbackID)
	w.evalCallbackMux.Unlock()

	var err error
	// 绑定临时回调
	if err = w.Bind(callbackName, func(r interface{}) {
		resultChan <- r
		// 删除绑定的js函数
		w.browser.Eval(fmt.Sprintf("delete window.%s;", callbackName))
	}); err != nil {
		return nil, err
	}

	// 执行脚本
	wrappedJS := fmt.Sprintf(`
	    (function() {
	        try {
	            const result = (%s);
	            if (result instanceof Promise) {
	                result.then(
	                    res => window.%s(res),
	                    err => window.%s({ error: err.message })
	                );
	            } else {
	                window.%s(result);
	            }
	        } catch (e) {
	            window.%s({ error: e.message });
	        }
	    })();
	`, strings.TrimRight(js, ";"), callbackName, callbackName, callbackName, callbackName)

	w.browser.Eval(wrappedJS)

	// 超时时间默认10秒
	t := 10 * time.Second
	if len(timeout) > 0 {
		t = timeout[0]
	}

	// 等待结果
	var isDone bool
	var result interface{}
	go func() {
		select {
		case ret := <-resultChan:
			if errMap, ok := ret.(map[string]interface{}); ok && errMap["error"] != nil {
				err = fmt.Errorf("JS错误: %v", errMap["error"])
			} else {
				result = ret
				err = nil
			}
		case <-time.After(t):
			err = ErrEvalTimeout
		}
		close(resultChan)
		isDone = true
	}()

	// 启动消息循环处理
	var msg wapi.MSG
	for !isDone {
		hasMessage := wapi.PeekMessage(&msg, 0, 0, 0, wapi.PM_REMOVE)
		if hasMessage {
			wapi.TranslateMessage(&msg)
			wapi.DispatchMessage(&msg)
		} else {
			wapi.Sleep(1) // 避免 CPU 空转
		}
	}
	return result, err
}

// Refresh 网页_刷新.
//
// forceReload: 是否强制刷新, 默认为false. 为 true 时，浏览器会强制重新加载页面，忽略缓存。这意味着无论页面是否已经在本地缓存中，都会从服务器重新获取资源。
func (w *WebView) Refresh(forceReload ...bool) *WebView {
	b := ""
	if len(forceReload) > 0 && forceReload[0] {
		b = "true"
	}
	w.browser.Eval("location.reload(" + b + ");")
	return w
}

// GoBack 网页_后退.
func (w *WebView) GoBack() *WebView {
	w.browser.Eval("history.back();")
	return w
}

// GoForward 网页_前进.
func (w *WebView) GoForward() *WebView {
	w.browser.Eval("history.forward();")
	return w
}

// Stop 网页_停止加载.
func (w *WebView) Stop() *WebView {
	w.browser.Eval("location.href = location.href;")
	return w
}

// Reload 网页_重新加载.
func (w *WebView) Reload() *WebView {
	w.browser.Eval("location.reload();")
	return w
}

// BindLog 绑定一个日志输出函数, 参数不限个数, 在js代码中调用, 会在go控制台中输出.
//
// funcName: 自定义函数名, 为空默认为glog.
func (w *WebView) BindLog(funcName ...string) error {
	name := "glog"
	if len(funcName) > 0 {
		name = funcName[0]
		// 名字中不能有空格
		if strings.Contains(name, " ") {
			return errors.New("funcName 中不能有空格")
		}
	}
	return w.Bind(name, func(msg ...interface{}) {
		fmt.Printf("%v\n", msg...)
	})
}
