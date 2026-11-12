package main

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/nico9julio/PGuardian/internal/config"
	"github.com/nico9julio/PGuardian/internal/ipc"
	"github.com/nico9julio/PGuardian/internal/notify"
	"github.com/nico9julio/PGuardian/internal/winintegration"
	"golang.org/x/sys/windows"
)

var (
	user32                     = windows.NewLazySystemDLL("user32.dll")
	shell32                    = windows.NewLazySystemDLL("shell32.dll")
	procRegisterClassExW       = user32.NewProc("RegisterClassExW")
	procCreateWindowExW        = user32.NewProc("CreateWindowExW")
	procDefWindowProcW         = user32.NewProc("DefWindowProcW")
	procGetMessageW            = user32.NewProc("GetMessageW")
	procTranslateMessage       = user32.NewProc("TranslateMessage")
	procDispatchMessageW       = user32.NewProc("DispatchMessageW")
	procPostMessageW           = user32.NewProc("PostMessageW")
	procPostQuitMessage        = user32.NewProc("PostQuitMessage")
	procCreatePopupMenu        = user32.NewProc("CreatePopupMenu")
	procAppendMenuW            = user32.NewProc("AppendMenuW")
	procTrackPopupMenu         = user32.NewProc("TrackPopupMenu")
	procDestroyMenu            = user32.NewProc("DestroyMenu")
	procGetCursorPos           = user32.NewProc("GetCursorPos")
	procSetForegroundWindow    = user32.NewProc("SetForegroundWindow")
	procCreateIconFromResource = user32.NewProc("CreateIconFromResourceEx")
	procLoadImageW             = user32.NewProc("LoadImageW")
	procDestroyIcon            = user32.NewProc("DestroyIcon")
	procShellNotifyIconW       = shell32.NewProc("Shell_NotifyIconW")
)

const (
	wmAppTrayCallback = 0x8001
	wmAppExitClean    = 0x8002
	wmCommand         = 0x0111
	wmDestroy         = 0x0002
	wmLButtonDblClk   = 0x0203
	wmRButtonUp       = 0x0205

	nimAdd    = 0x00000000
	nimModify = 0x00000001
	nimDelete = 0x00000002

	nifMessage = 0x00000001
	nifIcon    = 0x00000002
	nifTip     = 0x00000004
	nifInfo    = 0x00000010

	niifInfo    = 0x00000001
	niifWarning = 0x00000002
	niifError   = 0x00000003

	mfString     = 0x00000000
	mfSeparator  = 0x00000800
	mfGrayed     = 0x00000001
	tpmBottom    = 0x0020
	tpmLeftAlign = 0x0000

	menuIDStatus = 1001
	menuIDOpenUI = 1002
	menuIDExit   = 1003

	imageIcon      = 1
	lrLoadFromFile = 0x00000010
)

type wndClassExW struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   windows.Handle
	Icon       windows.Handle
	Cursor     windows.Handle
	Background windows.Handle
	MenuName   *uint16
	ClassName  *uint16
	IconSm     windows.Handle
}

type point struct {
	X, Y int32
}

type msg struct {
	Hwnd    windows.Handle
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
}

type notifyIconDataW struct {
	CbSize           uint32
	Hwnd             windows.Handle
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            windows.Handle
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UVersion         uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         windows.GUID
	HBalloonIcon     windows.Handle
}

var (
	mainHwnd   windows.Handle
	iconIdle   windows.Handle
	iconRun    windows.Handle
	iconErr    windows.Handle
	nid        notifyIconDataW
	lastStatus = "PGuardian Ready"
	langES     = true
)

func main() {
	// Pin main OS thread for Win32 GUI message loop
	runtime.LockOSThread()

	// Ensure single tray instance per RDP/Console user session using Local\ namespace
	mutexName, _ := windows.UTF16PtrFromString(`Local\PGuardianTraySingleInstance`)
	hMutex, err := windows.CreateMutex(nil, false, mutexName)
	if err != nil || windows.GetLastError() == windows.ERROR_ALREADY_EXISTS {
		if hMutex != 0 {
			_ = windows.CloseHandle(hMutex)
		}
		return
	}
	defer func() { _ = windows.CloseHandle(hMutex) }()

	iconIdle = loadEmbeddedICO(winintegration.TrayIdleICO, "idle.ico")
	iconRun = loadEmbeddedICO(winintegration.TrayRunningICO, "run.ico")
	iconErr = loadEmbeddedICO(winintegration.TrayErrorICO, "err.ico")
	defer destroyIcons()

	className, _ := windows.UTF16PtrFromString("PGuardianTrayHiddenWindow")
	wc := wndClassExW{
		Size:      uint32(unsafe.Sizeof(wndClassExW{})),
		WndProc:   syscall.NewCallback(wndProc),
		ClassName: className,
	}
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	hwnd, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(className)),
		0,
		0, 0, 0, 0,
		0, 0, 0, 0,
	)
	mainHwnd = windows.Handle(hwnd)

	nid = notifyIconDataW{
		CbSize:           uint32(unsafe.Sizeof(notifyIconDataW{})),
		Hwnd:             mainHwnd,
		UID:              1,
		UFlags:           nifMessage | nifIcon | nifTip,
		UCallbackMessage: wmAppTrayCallback,
		HIcon:            iconIdle,
	}
	copyUTF16(nid.SzTip[:], "PGuardian — PostgreSQL Backup Agent")
	procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&nid)))
	defer func() {
		procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&nid)))
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go listenServiceStream(ctx)

	var m msg
	for {
		ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(ret) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func listenServiceStream(ctx context.Context) {
	cli := ipc.NewHTTPClient(0) // Streaming connection without read timeout
	healthCli := ipc.NewHTTPClient(4 * time.Second)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Sync language setting
		if hResp, hErr := healthCli.Get("http://pipe/api/v1/health"); hErr == nil {
			var h struct {
				Language string `json:"language"`
			}
			_ = json.NewDecoder(hResp.Body).Decode(&h)
			_ = hResp.Body.Close()
			langES = h.Language != "en"
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://pipe/api/v1/tray/stream", nil)
		if err != nil {
			time.Sleep(3 * time.Second)
			continue
		}
		resp, err := cli.Do(req)
		if err != nil {
			time.Sleep(4 * time.Second)
			continue
		}

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || line == "{}" {
				continue
			}
			var ev notify.Event
			if err := json.Unmarshal([]byte(line), &ev); err != nil || ev.Type == "" {
				continue
			}
			handleIncomingEvent(ev)
		}
		_ = resp.Body.Close()
		time.Sleep(3 * time.Second)
	}
}

func handleIncomingEvent(ev notify.Event) {
	switch ev.Type {
	case notify.EventSystemUpdating, notify.EventShutdownTray:
		procPostMessageW.Call(uintptr(mainHwnd), wmAppExitClean, 0, 0)
		return
	case notify.EventBackupStarted, notify.EventBackupProgress:
		lastStatus = ev.Title + " (" + ev.Message + ")"
		updateTrayIcon(iconRun, "PGuardian — "+lastStatus)
	case notify.EventBackupCompleted:
		lastStatus = ev.Title
		updateTrayIcon(iconIdle, "PGuardian — "+ev.Title)
	case notify.EventBackupFailed:
		lastStatus = ev.Title
		updateTrayIcon(iconErr, "PGuardian — "+ev.Title)
	}

	if ev.ShowToast && ev.Title != "" {
		showWindowsToast(ev.Title, ev.Message, ev.Severity)
	}
}

func updateTrayIcon(hIcon windows.Handle, tooltip string) {
	if hIcon != 0 {
		nid.HIcon = hIcon
	}
	nid.UFlags = nifIcon | nifTip
	copyUTF16(nid.SzTip[:], tooltip)
	procShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&nid)))
}

func showWindowsToast(title, body, severity string) {
	nid.UFlags = nifInfo
	copyUTF16(nid.SzInfoTitle[:], title)
	copyUTF16(nid.SzInfo[:], body)
	switch severity {
	case "error":
		nid.DwInfoFlags = niifError
	case "warning":
		nid.DwInfoFlags = niifWarning
	default:
		nid.DwInfoFlags = niifInfo
	}
	procShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&nid)))
}

func wndProc(hwnd windows.Handle, message uint32, wParam, lParam uintptr) uintptr {
	switch message {
	case wmAppTrayCallback:
		switch uint32(lParam) {
		case wmLButtonDblClk:
			openMainUI()
			return 0
		case wmRButtonUp:
			showContextMenu(hwnd)
			return 0
		}
	case wmCommand:
		switch uint16(wParam & 0xFFFF) {
		case menuIDOpenUI:
			openMainUI()
		case menuIDExit:
			procPostQuitMessage.Call(0)
		}
		return 0
	case wmAppExitClean, wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	ret, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
	return ret
}

func showContextMenu(hwnd windows.Handle) {
	hMenu, _, _ := procCreatePopupMenu.Call()
	if hMenu == 0 {
		return
	}
	defer procDestroyMenu.Call(hMenu)

	openLabel := "Abrir Panel PGuardian"
	exitLabel := "Cerrar icono de bandeja"
	if !langES {
		openLabel = "Open PGuardian Console"
		exitLabel = "Close tray icon"
	}

	stPtr, _ := windows.UTF16PtrFromString(lastStatus)
	openPtr, _ := windows.UTF16PtrFromString(openLabel)
	exitPtr, _ := windows.UTF16PtrFromString(exitLabel)

	procAppendMenuW.Call(hMenu, mfString|mfGrayed, menuIDStatus, uintptr(unsafe.Pointer(stPtr)))
	procAppendMenuW.Call(hMenu, mfSeparator, 0, 0)
	procAppendMenuW.Call(hMenu, mfString, menuIDOpenUI, uintptr(unsafe.Pointer(openPtr)))
	procAppendMenuW.Call(hMenu, mfSeparator, 0, 0)
	procAppendMenuW.Call(hMenu, mfString, menuIDExit, uintptr(unsafe.Pointer(exitPtr)))

	var pt point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	procSetForegroundWindow.Call(uintptr(hwnd))
	procTrackPopupMenu.Call(hMenu, tpmBottom|tpmLeftAlign, uintptr(pt.X), uintptr(pt.Y), 0, uintptr(hwnd), 0)
}

func openMainUI() {
	uiExe := filepath.Join(config.InstallDir(), "PGuardian.exe")
	cmd := exec.Command(uiExe)
	_ = cmd.Start()
	if cmd.Process != nil {
		_ = cmd.Process.Release()
	}
}

func loadEmbeddedICO(data []byte, tempSuffix string) windows.Handle {
	tmpDir := filepath.Join(os.TempDir(), "PGuardianTrayIcons")
	_ = os.MkdirAll(tmpDir, 0700)
	icoPath := filepath.Join(tmpDir, tempSuffix)
	_ = os.WriteFile(icoPath, data, 0600)
	p, err := windows.UTF16PtrFromString(icoPath)
	if err != nil {
		return 0
	}
	h, _, _ := procLoadImageW.Call(0, uintptr(unsafe.Pointer(p)), imageIcon, 32, 32, lrLoadFromFile)
	return windows.Handle(h)
}

func destroyIcons() {
	for _, h := range []windows.Handle{iconIdle, iconRun, iconErr} {
		if h != 0 {
			procDestroyIcon.Call(uintptr(h))
		}
	}
	_ = procCreateIconFromResource
}

func copyUTF16(dst []uint16, s string) {
	for i := range dst {
		dst[i] = 0
	}
	u := syscall.StringToUTF16(s)
	limit := len(dst) - 1
	if len(u) < limit {
		limit = len(u)
	}
	copy(dst[:limit], u[:limit])
}
