//go:build windows
// +build windows

package edge

import (
	"unsafe"

	"github.com/twgh/xwebview/internal/w32"
)

func (e *Chromium) Resize() {
	if e.controller == nil {
		return
	}

	var bounds w32.Rect
	w32.User32GetClientRect.Call(e.hwnd, uintptr(unsafe.Pointer(&bounds)))

	words := (*[2]uintptr)(unsafe.Pointer(&bounds))
	e.controller.vtbl.PutBounds.Call(
		uintptr(unsafe.Pointer(e.controller)),
		words[0],
		words[1],
	)
}
