//go:build windows

package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"unsafe"
)

var (
	shell32          = syscall.NewLazyDLL("shell32.dll")
	user32           = syscall.NewLazyDLL("user32.dll")
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procShellNotifyIcon = shell32.NewProc("Shell_NotifyIconW")
	procDefWindowProc   = user32.NewProc("DefWindowProcW")
	procRegisterClass   = user32.NewProc("RegisterClassExW")
	procCreateWindowEx  = user32.NewProc("CreateWindowExW")
	procDestroyWindow   = user32.NewProc("DestroyWindow")
	procGetMessage      = user32.NewProc("GetMessageW")
	procTranslateMsg    = user32.NewProc("TranslateMessage")
	procDispatchMsg     = user32.NewProc("DispatchMessageW")
	procPostQuitMessage = user32.NewProc("PostQuitMessage")
	procLoadIcon        = user32.NewProc("LoadIconW")
)

const (
	NIM_ADD    = 0x00000000
	NIM_MODIFY = 0x00000001
	NIM_DELETE = 0x00000002

	NIF_MESSAGE = 0x00000001
	NIF_ICON    = 0x00000002
	NIF_TIP     = 0x00000004

	WM_USER     = 0x0400
	WM_TRAYICON = WM_USER + 1
	WM_LBUTTONDBLCLK = 0x0203
	WM_RBUTTONUP     = 0x0205
	WM_DESTROY       = 0x0002

	IDI_APPLICATION = 32512
)

type NOTIFYICONDATA struct {
	CbSize           uint32
	HWnd             uintptr
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            uintptr
	SzTip            [128]uint16
}

type WNDCLASSEX struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

type MSG struct {
	HWnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
}

var currentServerURL string

func wndProc(hWnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case WM_TRAYICON:
		if lParam == WM_LBUTTONDBLCLK || lParam == WM_RBUTTONUP {
			if currentServerURL != "" {
				_ = openBrowser(currentServerURL)
			}
		}
		return 0
	case WM_DESTROY:
		procPostQuitMessage.Call(0)
		return 0
	default:
		r, _, _ := procDefWindowProc.Call(hWnd, uintptr(msg), wParam, lParam)
		return r
	}
}

func runTray(ctx context.Context, serverURL string) {
	currentServerURL = serverURL

	defer func() {
		if r := recover(); r != nil {
			log.Printf("[Tray] Warning: Win32 Tray panic caught: %v", r)
		}
	}()

	className, _ := syscall.UTF16PtrFromString("OpenLocalCRMControlTray")
	iconHandle, _, _ := procLoadIcon.Call(0, uintptr(IDI_APPLICATION))

	wc := WNDCLASSEX{
		CbSize:      uint32(unsafe.Sizeof(WNDCLASSEX{})),
		LpfnWndProc: syscall.NewCallback(wndProc),
		LpszClassName: className,
		HIcon:       iconHandle,
	}

	procRegisterClass.Call(uintptr(unsafe.Pointer(&wc)))

	title, _ := syscall.UTF16PtrFromString("OpenLocalCRM Tray Window")
	hwnd, _, _ := procCreateWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(title)),
		0, 0, 0, 0, 0, 0, 0, 0, 0,
	)

	if hwnd == 0 {
		log.Println("[Tray] Failed creating hidden window, falling back to signal wait.")
		fallbackWait(ctx)
		return
	}

	var nid NOTIFYICONDATA
	nid.CbSize = uint32(unsafe.Sizeof(nid))
	nid.HWnd = hwnd
	nid.UID = 1001
	nid.UFlags = NIF_MESSAGE | NIF_ICON | NIF_TIP
	nid.UCallbackMessage = WM_TRAYICON
	nid.HIcon = iconHandle

	tipBytes, _ := syscall.UTF16FromString("OpenLocalCRM - Control Center")
	copy(nid.SzTip[:], tipBytes)

	procShellNotifyIcon.Call(NIM_ADD, uintptr(unsafe.Pointer(&nid)))
	defer func() {
		procShellNotifyIcon.Call(NIM_DELETE, uintptr(unsafe.Pointer(&nid)))
		procDestroyWindow.Call(hwnd)
	}()

	log.Printf("[Tray] Win32 System-Tray icon initialized.")

	// Standard Win32 Message Pump
	var msg MSG
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		r, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		procTranslateMsg.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMsg.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func fallbackWait(ctx context.Context) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	select {
	case <-ctx.Done():
	case <-sigChan:
	}
}
