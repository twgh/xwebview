// 在布局元素中创建 WebView
package main

import (
	_ "embed"
	"fmt"
	"github.com/twgh/xcgui/app"
	"github.com/twgh/xcgui/wapi"
	"github.com/twgh/xcgui/widget"
	"github.com/twgh/xcgui/window"
	"github.com/twgh/xcgui/xc"
	"github.com/twgh/xcgui/xcc"
	"github.com/twgh/xwebview"
	"math/rand"
	"strconv"
	"strings"
	"time"
)

//go:embed main.xml
var xmlStr string

func main() {
	a := app.New(true)
	a.EnableAutoDPI(true).EnableDPI(true)

	// 创建窗口
	w := window.NewByLayoutStringW(xmlStr, 0, 0)
	w.SetTransparentAlpha(255)

	// 放置 WebView 的布局元素
	layoutWV := widget.NewLayoutEleByName("布局WV")

	// 创建 WebView
	wv := createWV(layoutWV)

	// 按钮_隐藏
	btnHide := widget.NewButtonByName("按钮_隐藏")
	btnHide.Event_BnClick1(func(hEle int, pbHandled *bool) int {
		btnHide.Enable(false).Redraw(false)
		defer btnHide.Enable(true).Redraw(false)
		if !xc.XC_IsHELE(layoutWV.Handle) {
			xc.XC_Alert("提示", "布局元素不存在")
		}
		isShow := layoutWV.IsShow()
		if isShow {
			btnHide.SetText("显示")
		} else {
			btnHide.SetText("隐藏")
		}
		layoutWV.Show(!isShow)
		layoutWV.Redraw(false)
		return 0
	})

	// 编辑框_地址栏
	editUrl := widget.NewEditByName("编辑框_地址栏")
	// 按钮_跳转
	btnJump := widget.NewButtonByName("按钮_跳转")
	// 按钮_JS测试
	btnJsTest := widget.NewButtonByName("按钮_JS测试")
	// 按钮_销毁
	btnDestroy := widget.NewButtonByName("按钮_销毁")

	// 跳转网页的函数
	jumpFunc := func() {
		btnJump.Enable(false).Redraw(false)
		defer btnJump.Enable(true).Redraw(false)
		addr := strings.TrimSpace(editUrl.GetTextEx())
		if addr != "" {
			wv.Navigate(addr)
		}
	}

	// 按钮_跳转事件
	btnJump.Event_BnClick1(func(hEle int, pbHandled *bool) int {
		jumpFunc()
		return 0
	})

	// 编辑框_地址栏事件
	editUrl.Event_KEYDOWN1(func(hEle int, wParam, lParam uintptr, pbHandled *bool) int {
		if wParam == xcc.VK_Enter { // 判断按下回车键
			jumpFunc()
		}
		return 0
	})

	// 按钮_JS测试事件
	btnJsTest.Event_BnClick1(func(hEle int, pbHandled *bool) int {
		codeWindow := window.New(0, 0, 600, 500, "测试JS代码", w.GetHWND(), xcc.Window_Style_Default)
		codeWindow.EnableLayout(true)
		codeWindow.SetAlignV(xcc.Layout_Align_Top)
		// 代码框
		codeEdit := widget.NewEdit(0, 0, 0, 0, codeWindow.Handle)
		codeEdit.LayoutItem_SetWidth(xcc.Layout_Size_Fill, -1)
		codeEdit.LayoutItem_SetHeight(xcc.Layout_Size_Percent, 60)
		codeEdit.SetText("alert('Hello World')").EnableMultiLine(true).SetDefaultText("请输入代码")
		codeWindow.SetFocusEle(codeEdit.Handle)
		// 选择框
		checkBox := widget.NewButton(0, 0, 0, 0, "获取返回值", codeWindow.Handle)
		checkBox.SetTypeEx(xcc.Button_Type_Check).EnableBkTransparent(true)
		checkBox.LayoutItem_SetWidth(xcc.Layout_Size_Fill, -1)
		checkBox.LayoutItem_SetHeight(xcc.Layout_Size_Percent, 8)
		// 输出框
		resultEdit := widget.NewEdit(0, 0, 0, 0, codeWindow.Handle)
		resultEdit.LayoutItem_SetWidth(xcc.Layout_Size_Fill, -1)
		resultEdit.LayoutItem_SetHeight(xcc.Layout_Size_Percent, 20)
		resultEdit.EnableReadOnly(true).EnableMultiLine(true).SetDefaultText("这里会输出结果").EnableAutoShowScrollBar(true)
		// 日志函数
		log := func(s string) {
			resultEdit.MoveEnd()
			resultEdit.AddText(time.Now().Format("[15:04:05] ") + s + "\n").ScrollBottom()
			resultEdit.Redraw(false)
		}
		// 执行按钮
		btn := widget.NewButton(0, 0, 0, 0, "执行", codeWindow.Handle)
		btn.LayoutItem_SetWidth(xcc.Layout_Size_Fill, -1)
		btn.LayoutItem_SetHeight(xcc.Layout_Size_Percent, 12)
		btn.Event_BnClick1(func(hEle int, pbHandled *bool) int {
			code := strings.TrimSpace(codeEdit.GetTextEx())
			if code != "" {
				if checkBox.IsCheck() { // 获取返回值
					{
						// 同步获取
						ret, err := wv.EvalSync(code)
						if err != nil {
							log("EvalSync, 返回错误: " + err.Error())
							return 0
						}
						log(fmt.Sprintf("EvalSync, js返回结果: %v", ret))
					}

					/*{
						// 异步获取
						if err := wv.EvalAsync(code, func(result interface{}, err error) {
							if err != nil {
								xc.XC_CallUT(func() {
									log("EvalAsync, js返回错误: " + err.Error())
								})
								return
							}
							xc.XC_CallUT(func() {
								log(fmt.Sprintf("EvalAsync, js返回结果: %v", result))
							})
						}); err != nil {
							xc.XC_CallUT(func() {
								log("EvalAsync, 报错: " + err.Error())
							})
						}
					}*/
				} else {
					wv.Eval(code)
				}
			} else {
				codeWindow.SetFocusEle(codeEdit.Handle)
			}
			return 0
		})
		codeWindow.Show(true)
		return 0
	})

	// 按钮_销毁事件
	btnDestroy.Event_BnClick1(func(hEle int, pbHandled *bool) int {
		if btnDestroy.GetText() == "销毁" && wapi.IsWindow(wv.GetHWND()) {
			wv.Destroy()
			btnDestroy.SetText("创建").Redraw(false)
		} else {
			wv = createWV(layoutWV)
			layoutWV.PostEvent(xcc.XE_SIZE, 0, 0)
			btnDestroy.SetText("销毁").Redraw(false)
		}
		return 0
	})

	// 按钮_设置搜索关键词
	btnSetSearch := widget.NewButtonByName("按钮_设置搜索关键词")
	// 按钮_设置搜索关键词事件
	rand.Seed(time.Now().UnixNano())
	words := []string{"腾讯视频", "优酷视频", "爱奇艺视频", "哔哩哔哩"}
	btnSetSearch.Event_BnClick1(func(hEle int, pbHandled *bool) int {
		index := rand.Intn(len(words))
		wv.Eval("document.querySelectorAll('#kw')[0].value = '" + words[index] + "'")
		return 0
	})
	// 按钮_点击搜索
	btnClickSearch := widget.NewButtonByName("按钮_点击搜索")
	// 按钮_点击搜索事件
	btnClickSearch.Event_BnClick1(func(hEle int, pbHandled *bool) int {
		wv.Eval("document.querySelectorAll('#su')[0].click()")
		return 0
	})

	// 按钮_执行Go函数
	btnGoFunc := widget.NewButtonByName("按钮_执行Go函数")
	// 按钮_执行Go函数事件
	btnGoFunc.Event_BnClick1(func(hEle int, pbHandled *bool) int {
		wv.Eval(`
			goAddStr('Hello World', 666).then(function(result) {
				alert(result);
			});
		`)
		return 0
	})

	// 按钮_刷新
	btnRefresh := widget.NewButtonByName("按钮_刷新")
	// 按钮_刷新事件
	btnRefresh.Event_BnClick1(func(hEle int, pbHandled *bool) int {
		wv.Refresh()
		return 0
	})

	// 按钮_打开vea
	btnOpenVea := widget.NewButtonByName("按钮_打开vea")
	// 按钮_打开vea事件
	btnOpenVea.Event_BnClick1(func(hEle int, pbHandled *bool) int {
		wv.Navigate("https://panjiachen.github.io/vue-element-admin/")
		return 0
	})

	// 按钮_打开百度
	btnOpenBaidu := widget.NewButtonByName("按钮_打开百度")
	// 按钮_打开百度事件
	btnOpenBaidu.Event_BnClick1(func(hEle int, pbHandled *bool) int {
		wv.Navigate("https://www.baidu.com")
		return 0
	})

	w.AdjustLayout()
	w.Show(true)
	a.Run()
	a.Exit()
}

func createWV(layoutWV *widget.LayoutEle) *xwebview.WebView {
	wv := xwebview.New(layoutWV.Handle, xwebview.XcWebViewOption{
		DataPath:   "D:\\cache\\wv",
		Debug:      true,
		FillParent: true,
	})

	// 绑定Go函数
	if err := wv.Bind("goAddStr", func(str string, num int) string {
		fmt.Println("执行Go函数: goAddStr")
		return "传进Go函数 goAddStr 的参数: " + str + ", " + strconv.Itoa(num)
	}); err != nil {
		fmt.Println("绑定Go函数 goAddStr 失败:", err.Error())
	}

	// 绑定一个输出函数, 方便在js中调用
	if err := wv.Bind("golog", func(a interface{}) {
		fmt.Printf("js输出: %v\n", a)
	}); err != nil {
		fmt.Println("绑定Go函数 golog 失败:", err.Error())
	}

	// 加载网页
	wv.Navigate("https://www.baidu.com")
	return wv
}
