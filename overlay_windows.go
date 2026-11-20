//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The overlay is a plain Win32 window: no CGO, no WebView, nothing to install.

const (
	overlayClass   = "WjTokenOverlay"
	overlayRefresh = 5 * time.Second

	wmDestroy       = 0x0002
	wmClose         = 0x0010
	wmLButtonDown   = 0x0201
	wmLButtonDblClk = 0x0203
	menuToggle      = 2
	wmRButtonUp     = 0x0205
	wmEnterSizeMove = 0x0231
	wmExitSizeMove  = 0x0232
	wmDpiChanged    = 0x02E0
	monitorNearest  = 2
	wmSnapshot      = 0x8001
	wmSetOpacity    = 0x8002 // wParam: background opacity in percent
	wmReloadLimits  = 0x8003 // read the limits from overlay.json again
	wmSysCommand    = 0x0112
	scDragMove      = 0xF012
	csDblClks       = 0x0008
	wsPopup         = 0x80000000
	wsExTopmost     = 0x00000008
	wsExToolWindow  = 0x00000080
	wsExLayered     = 0x00080000
	wsExNoActivate  = 0x08000000
	ulwAlpha        = 2
	acSrcAlpha      = 1
	swShowNoActive  = 4
	swpNoActivate   = 0x0010
	swpNoZOrder     = 0x0004
	swpNoMove       = 0x0002
	tpmReturnCmd    = 0x0100
	menuClose       = 1
	menuSince       = 10 // + index in overlayWindows
	menuOpacity     = 20 // + index in overlayOpacities
	menuText        = 30 // + index in overlayTextSizes
	mfChecked       = 0x0008
	mfPopup         = 0x0010
	mfGrayed        = 0x0001
	logPixelsY      = 90
	defaultOpacity  = 50  // default background opacity in percent; text is always fully opaque
	haloAlpha       = 190 // the background right around text is at least this opaque, so text stays readable
	borderAlpha     = 255
	textColor       = 0xFFFFFF
	mutedColor      = 0xD8D2CC
	activeColor     = 0x80DE4A
	warnColor       = 0x3CC8FF
	overColor       = 0x6060FF
	detachedProcess = 0x00000008
)

var bgPixel = [3]byte{0x1D, 0x18, 0x16}         // blue, green, red
var borderPixel = [3]byte{0x6A, 0x62, 0x5C}     // blue, green, red
var pastColumnPixel = [3]byte{0x9C, 0x92, 0x8A} // the chart's earlier periods
var sharePixel = [3]byte{0xD0, 0x9C, 0x5C}      // a project's share bar, blue

// barPixels is the usage bar's fill by warn level: green, yellow from 80%, red when over.
var barPixels = [3][3]byte{{0x80, 0xDE, 0x4A}, {0x3C, 0xC8, 0xFF}, {0x60, 0x60, 0xFF}}

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	gdi32    = windows.NewLazySystemDLL("gdi32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	getModuleHandle     = kernel32.NewProc("GetModuleHandleW")
	setProcessDPIAware  = user32.NewProc("SetProcessDPIAware")
	setDPIContext       = user32.NewProc("SetProcessDpiAwarenessContext") // Windows 10 1703+
	getDpiForWindow     = user32.NewProc("GetDpiForWindow")               // Windows 10 1607+
	registerClassEx     = user32.NewProc("RegisterClassExW")
	createWindowEx      = user32.NewProc("CreateWindowExW")
	defWindowProc       = user32.NewProc("DefWindowProcW")
	showWindow          = user32.NewProc("ShowWindow")
	updateLayeredWindow = user32.NewProc("UpdateLayeredWindow")
	setWindowPos        = user32.NewProc("SetWindowPos")
	getWindowRect       = user32.NewProc("GetWindowRect")
	getMessage          = user32.NewProc("GetMessageW")
	translateMessage    = user32.NewProc("TranslateMessage")
	dispatchMessage     = user32.NewProc("DispatchMessageW")
	postMessage         = user32.NewProc("PostMessageW")
	postQuitMessage     = user32.NewProc("PostQuitMessage")
	findWindow          = user32.NewProc("FindWindowW")
	destroyWindow       = user32.NewProc("DestroyWindow")
	getDC               = user32.NewProc("GetDC")
	releaseDC           = user32.NewProc("ReleaseDC")
	loadCursor          = user32.NewProc("LoadCursorW")
	getSystemMetrics    = user32.NewProc("GetSystemMetrics")
	getCursorPos        = user32.NewProc("GetCursorPos")
	monitorFromRect     = user32.NewProc("MonitorFromRect")
	getMonitorInfo      = user32.NewProc("GetMonitorInfoW")
	createPopupMenu     = user32.NewProc("CreatePopupMenu")
	appendMenu          = user32.NewProc("AppendMenuW")
	trackPopupMenu      = user32.NewProc("TrackPopupMenu")
	destroyMenu         = user32.NewProc("DestroyMenu")
	setForegroundWindow = user32.NewProc("SetForegroundWindow")

	createFont         = gdi32.NewProc("CreateFontW")
	selectObject       = gdi32.NewProc("SelectObject")
	deleteObject       = gdi32.NewProc("DeleteObject")
	deleteDC           = gdi32.NewProc("DeleteDC")
	createCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	createDIBSection   = gdi32.NewProc("CreateDIBSection")
	gdiFlush           = gdi32.NewProc("GdiFlush")
	setBkMode          = gdi32.NewProc("SetBkMode")
	setTextColor       = gdi32.NewProc("SetTextColor")
	textOut            = gdi32.NewProc("TextOutW")
	getTextExtent      = gdi32.NewProc("GetTextExtentPoint32W")
	getDeviceCaps      = gdi32.NewProc("GetDeviceCaps")
)

type winRect struct{ Left, Top, Right, Bottom int32 }
type winSize struct{ CX, CY int32 }
type winPoint struct{ X, Y int32 }

type winMsg struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      winPoint
	Private uint32
}

type winClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}

type winMonitorInfo struct {
	Size          uint32
	Monitor, Work winRect
	Flags         uint32
}

type winBitmapInfo struct {
	Size          uint32
	Width, Height int32
	Planes, Bits  uint16
	Compression   uint32
	SizeImage     uint32
	XPels, YPels  int32
	Used, Needed  uint32
	Colors        [1]uint32
}

type overlayLine struct {
	Project, Tokens string
	Active          bool
	Detail          []string // extra columns, between Project and Tokens, when expanded
	Cost            string   // last column, only when expanded
	Total           int64    // the project's token, for its share bar
}

// Columns from overlayFirstNumber on are right aligned when expanded.
var overlayDetailHeading = []string{"Project", "App", "Lúc", "Model", "Vào", "Ra", "Cache", "Tổng", "≈USD"}

const overlayFirstNumber = 4

func (line overlayLine) cells(expanded bool) []string {
	if !expanded {
		if line.Cost == "" {
			return []string{line.Project, line.Tokens}
		}
		return []string{line.Project, line.Tokens, line.Cost}
	}
	cells := []string{line.Project}
	cells = append(cells, line.Detail...)
	return append(cells, line.Tokens, line.Cost)
}

// overlayFrame is everything one render needs, copied under the lock.
type overlayFrame struct {
	lines                       []overlayLine
	title, total, footer, whole string
	expanded                    bool
	warnLevel                   int
	used, limit                 int64 // the window's total and the -warn limit; limit 0 means no bar
	alpha                       byte
	series                      []int64 // token per day, week or month, the current one last
	limits                      []overlayLimit
}

type overlayView struct {
	used       int64
	mu         sync.Mutex
	footer     string
	title      string
	total      string
	whole      string
	lines      []overlayLine
	detail     []overlayLine
	series     []int64
	limits     []overlayLimit
	limit5h    string // the user's 5-hour limit, as -limit-5h takes it
	limitWeek  string // the user's 7-day limit
	expanded   bool
	warn       int64 // the user's limit for the window's total; 0 means none
	warnLevel  int
	hwnd       uintptr
	font, bold uintptr
	big        uintptr // bold, larger: the total
	scale      float64
	statePath  string
	moving     bool   // inside a drag: don't pull the window back into the work area yet
	window     string // the period, as wj usage names it: today, week or month
	alpha      byte   // background alpha; text stays opaque
	textPct    int    // text size in percent of the default
	refresh    chan struct{}
}

// overlaySettings is overlay.json. Old files hold only X and Y: zero values mean the default.
type overlaySettings struct {
	X, Y    int32
	Since   string `json:",omitempty"`
	Opacity int    `json:",omitempty"` // percent, 20-100
	Text    int    `json:",omitempty"` // percent, 80-160
	// Token limits for the limit row, as -limit-5h and -limit-week take them.
	Limit5h   string `json:",omitempty"`
	LimitWeek string `json:",omitempty"`
}

// The overlay counts the same periods as wj usage (calendar day, week from Monday,
// month), so its numbers match the usage report.
var overlayWindows = []string{"today", "week", "month"}
var overlayWindowNames = []string{"Hôm nay", "Tuần này", "Tháng này"}

func overlayPeriodTag(spec string) string {
	return map[string]string{"today": "hôm nay", "week": "tuần này", "month": "tháng này"}[spec]
}

// overlayPeriodStart is where the period that holds now begins.
func overlayPeriodStart(spec string, now time.Time) time.Time {
	if p, err := resolvePeriod(spec, now); err == nil {
		return p.Start
	}
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
}

var overlayOpacities = []int{20, 35, 50, 65, 80, 100}

// overlayOpacityFlag is -opacity, 0 when not given; it wins over the saved one.
var overlayOpacityFlag int

func opacityAlpha(pct int) byte { return byte(pct * 255 / 100) }

var overlayTextSizes = []int{85, 100, 125, 150}

func overlayLoadSettings() (s overlaySettings, ok bool) {
	data, err := os.ReadFile(overlay.statePath)
	if err != nil || json.Unmarshal(data, &s) != nil {
		return overlaySettings{}, false
	}
	return s, true
}

var overlay overlayView

func runOverlay(args []string, out io.Writer, home string) error {
	flags := flag.NewFlagSet("overlay", flag.ContinueOnError)
	flags.SetOutput(out)
	window := flags.String("period", "today", "kỳ đếm token như wj usage: today, week, month")
	foreground := flags.Bool("run", false, "chạy cửa sổ ngay trong tiến trình này")
	warn := flags.String("warn", "", "đổi màu dòng tổng khi gần tới ngưỡng này, ví dụ 5M, 500k")
	opacity := flags.Int("opacity", 0, "độ đục của nền, 20-100 (%), được lưu lại; mặc định 50")
	limit5h := flags.String("limit-5h", "", "hạn mức token của một block 5 giờ, vd 20M, hoặc max (block lớn nhất từng dùng), off để tắt; được lưu lại")
	limitWeek := flags.String("limit-week", "", "hạn mức token của 7 ngày gần nhất, vd 500M, off để tắt; được lưu lại")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	sinceSet := false
	flags.Visit(func(f *flag.Flag) { sinceSet = sinceSet || f.Name == "period" })
	if !containsName(overlayWindows, *window) {
		return errors.New("-period chỉ nhận: today, week, month")
	}
	if *opacity != 0 && (*opacity < 20 || *opacity > 100) {
		return errors.New("-opacity nhận từ 20 đến 100 (phần trăm)")
	}
	overlayOpacityFlag = *opacity
	limitsSet := false
	flags.Visit(func(f *flag.Flag) { limitsSet = limitsSet || f.Name == "limit-5h" || f.Name == "limit-week" })
	if err := checkOverlayLimit(*limit5h, true); err != nil {
		return fmt.Errorf("-limit-5h: %w", err)
	}
	if err := checkOverlayLimit(*limitWeek, false); err != nil {
		return fmt.Errorf("-limit-week: %w", err)
	}
	if limitsSet {
		if err := overlaySaveLimits(filepath.Join(stateDir(home), "overlay.json"), flags, *limit5h, *limitWeek); err != nil {
			return err
		}
	}
	if *warn != "" {
		limit, err := parseTokenCount(*warn)
		if err != nil {
			return err
		}
		overlay.warn = limit
	}
	class, _ := windows.UTF16PtrFromString(overlayClass)
	existing, _, _ := findWindow.Call(uintptr(unsafe.Pointer(class)), 0)
	switch {
	case flags.NArg() == 1 && flags.Arg(0) == "stop":
		if existing == 0 {
			fmt.Fprintln(out, "Overlay chưa mở.")
			return nil
		}
		postMessage.Call(existing, wmClose, 0, 0)
		fmt.Fprintln(out, "Đã đóng overlay.")
		return nil
	case flags.NArg() != 0:
		return errors.New("cách dùng: wj overlay [-period today|week|month] [-warn 5M] [-opacity 50] | wj overlay stop")
	}
	if existing != 0 {
		// The open overlay takes new settings at once.
		if *opacity != 0 {
			postMessage.Call(existing, wmSetOpacity, uintptr(*opacity), 0)
			fmt.Fprintf(out, "Đã đặt độ đục nền overlay %d%%.\n", *opacity)
		}
		if limitsSet {
			postMessage.Call(existing, wmReloadLimits, 0, 0)
			fmt.Fprintln(out, "Đã đặt hạn mức trên overlay.")
		}
		if *opacity == 0 && !limitsSet {
			fmt.Fprintln(out, "Overlay đang mở rồi. Đóng: wj overlay stop")
		}
		return nil
	}
	if *foreground {
		return overlayLoop(*window, sinceSet, home)
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	childArgs := []string{"overlay", "-run", "-warn", *warn, "-opacity", fmt.Sprint(*opacity)}
	if sinceSet {
		childArgs = append(childArgs, "-period", *window)
	}
	cmd := exec.Command(exe, childArgs...)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: detachedProcess | windows.CREATE_NEW_PROCESS_GROUP}
	if err = cmd.Start(); err != nil {
		return err
	}
	_ = cmd.Process.Release()
	fmt.Fprintln(out, "Đã mở overlay token. Kéo để đổi chỗ, chuột phải để đóng (hoặc: wj overlay stop).")
	return nil
}

func overlayLoop(window string, sinceSet bool, home string) error {
	runtime.LockOSThread()
	overlay.statePath = filepath.Join(stateDir(home), "overlay.json")
	overlay.alpha, overlay.textPct = opacityAlpha(defaultOpacity), 100
	if saved, ok := overlayLoadSettings(); ok {
		if containsName(overlayWindows, saved.Since) && !sinceSet {
			window = saved.Since
		}
		// 92 was the default before 0.2.14 and is no longer a choice: it moves to the new default.
		if saved.Opacity >= 20 && saved.Opacity <= 100 && saved.Opacity != 92 {
			overlay.alpha = opacityAlpha(saved.Opacity)
		}
		if saved.Text >= 80 && saved.Text <= 160 {
			overlay.textPct = saved.Text
		}
		overlay.limit5h, overlay.limitWeek = saved.Limit5h, saved.LimitWeek
	}
	overlay.window = window
	overlay.refresh = make(chan struct{}, 1)
	overlay.title = overlayPeriodTag(window)
	overlayDPIAware()
	instance, _, _ := getModuleHandle.Call(0)
	class, _ := windows.UTF16PtrFromString(overlayClass)
	cursor, _, _ := loadCursor.Call(0, 32512)
	wc := winClassEx{Style: csDblClks, WndProc: windows.NewCallback(overlayProc), Instance: instance, Cursor: cursor, ClassName: class}
	wc.Size = uint32(unsafe.Sizeof(wc))
	if atom, _, err := registerClassEx.Call(uintptr(unsafe.Pointer(&wc))); atom == 0 {
		return fmt.Errorf("không tạo được cửa sổ: %w", err)
	}
	hwnd, _, err := createWindowEx.Call(wsExTopmost|wsExToolWindow|wsExLayered|wsExNoActivate,
		uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(class)), wsPopup, 0, 0, 10, 10, 0, 0, instance, 0)
	if hwnd == 0 {
		return fmt.Errorf("không tạo được cửa sổ: %w", err)
	}
	overlay.hwnd = hwnd
	overlaySetDPI(overlayWindowDPI(hwnd))
	if overlayOpacityFlag != 0 {
		postMessage.Call(hwnd, wmSetOpacity, uintptr(overlayOpacityFlag), 0) // applies and saves it
	}

	go overlayFeed(home, hwnd)

	var msg winMsg
	for {
		r, _, _ := getMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			return nil
		}
		translateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		dispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

// overlayDPIAware asks for per-monitor DPI (v2), so Windows sends WM_DPICHANGED instead of
// stretching the bitmap on a monitor with another scale. Older Windows: system DPI only.
func overlayDPIAware() {
	if setDPIContext.Find() == nil {
		const perMonitorV2 = ^uintptr(3) // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 = -4
		if ok, _, _ := setDPIContext.Call(perMonitorV2); ok != 0 {
			return
		}
	}
	setProcessDPIAware.Call()
}

// overlayWindowDPI is the DPI of the monitor the window is on.
func overlayWindowDPI(hwnd uintptr) uintptr {
	if getDpiForWindow.Find() == nil {
		if dpi, _, _ := getDpiForWindow.Call(hwnd); dpi != 0 {
			return dpi
		}
	}
	hdc, _, _ := getDC.Call(hwnd)
	dpi, _, _ := getDeviceCaps.Call(hdc, logPixelsY)
	releaseDC.Call(hwnd, hdc)
	if dpi == 0 {
		dpi = 96
	}
	return dpi
}

// overlaySetDPI sets the scale and rebuilds the fonts for dpi. Only the window thread calls it.
func overlaySetDPI(dpi uintptr) {
	for _, font := range []uintptr{overlay.font, overlay.bold, overlay.big} {
		if font != 0 {
			deleteObject.Call(font)
		}
	}
	overlay.scale = float64(dpi) / 96
	pct := overlay.textPct
	overlay.font = overlayFont(10*pct/100, 400, dpi)
	overlay.bold = overlayFont(10*pct/100, 600, dpi)
	overlay.big = overlayFont(13*pct/100, 700, dpi)
}

func overlayFont(points, weight int, dpi uintptr) uintptr {
	name, _ := windows.UTF16PtrFromString("Segoe UI")
	height := -points * int(dpi) / 72
	// Quality 4 is grayscale smoothing: ClearType's colour fringes need an opaque background.
	font, _, _ := createFont.Call(uintptr(int32ToPtr(height)), 0, 0, 0, uintptr(weight), 0, 0, 0, 1, 0, 0, 4, 0, uintptr(unsafe.Pointer(name)))
	return font
}

func int32ToPtr(v int) uintptr { return uintptr(int64(v)) }

func overlayFeed(home string, hwnd uintptr) {
	refreshPrices(home, nil, true)
	scanner := newSessionScanner(home)
	for {
		overlay.mu.Lock()
		window, limit5h, limitWeek := overlay.window, overlay.limit5h, overlay.limitWeek
		overlay.mu.Unlock()
		since := overlayPeriodStart(window, time.Now())
		scanner.scan(time.Time{})
		list, total := scanner.projectStatus(since)
		now := time.Now()
		footer := "≈" + costText(total) + " · " + scanner.overlayFooter(now)
		running := runningProjects(list, scanner.openClaudeProjects(), now)
		lines := make([]overlayLine, 0, len(running)+1)
		if len(running) == 0 {
			lines = append(lines, overlayLine{Project: "Không có project đang chạy"})
		}
		for _, row := range running {
			lines = append(lines, overlayLine{Project: cutRunes(row.Project, 18), Tokens: compactTokens(row.Total), Active: now.Sub(row.Last) < activeWindow, Total: row.Total, Cost: "≈" + costText(row)})
		}
		var detail []overlayLine
		for _, entry := range detailEntries(overlayDetailRows(running, list), scanner.modelsByProject(since), overlayMaxLines) {
			detail = append(detail, overlayDetailLine(entry, now))
		}
		series := scanner.overlaySeries(window, now)
		limits := scanner.overlayLimits(limit5h, limitWeek, now)
		whole := scanner.machineTotals(now)
		whole += fmt.Sprintf(" · %s ≈%s theo giá API %s", overlayPeriodTag(window), costText(total), pricesAsOf)
		shown := compactTokens(total.Total)
		if overlay.warn > 0 {
			shown += "/" + compactTokens(overlay.warn)
		}
		overlay.mu.Lock()
		overlay.title = overlayPeriodTag(window)
		overlay.lines, overlay.detail, overlay.total, overlay.footer, overlay.whole = lines, detail, shown, footer, whole
		overlay.warnLevel = warnLevel(total.Total, overlay.warn)
		overlay.used = total.Total
		overlay.series = series
		overlay.limits = limits
		overlay.mu.Unlock()
		postMessage.Call(hwnd, wmSnapshot, 0, 0)
		select {
		case <-time.After(overlayRefresh):
		case <-overlay.refresh:
		}
	}
}

func overlayDetailLine(row overlayDetailEntry, now time.Time) overlayLine {
	ago := "đang mở"
	if !row.Last.IsZero() {
		ago = agoText(now.Sub(row.Last))
	}
	line := overlayLine{
		Tokens: compactTokens(row.Total),
		Cost:   costText(row.usageRow),
		Detail: []string{
			strings.Join(row.Apps, "+"), ago, cutRunes(row.LastModel, 20),
			compactTokens(row.Input), compactTokens(row.Output), compactTokens(row.CacheRead + row.CacheWrite),
		},
	}
	if row.Sub {
		line.Detail[2] = cutRunes(row.Model, 20)
		line.Project = "   ›" // indent: this line belongs to the project above
		return line
	}
	line.Project = cutRunes(row.Project, 32)
	line.Active = now.Sub(row.Last) < activeWindow
	if row.Models > 1 {
		line.Detail[2] = fmt.Sprintf("%d model", row.Models)
	}
	return line
}

func overlayToggle(hwnd uintptr) {
	overlay.mu.Lock()
	overlay.expanded = !overlay.expanded
	overlay.mu.Unlock()
	overlayRender(hwnd)
}

func overlayProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	switch message {
	case wmLButtonDown:
		// Windows' own drag. A hit test of HTCAPTION would be simpler, but it loses double clicks.
		defWindowProc.Call(hwnd, wmSysCommand, scDragMove, 0)
		return 0
	case wmLButtonDblClk:
		overlayToggle(hwnd)
		return 0
	case wmSnapshot:
		overlayRender(hwnd)
		return 0
	case wmReloadLimits:
		if saved, ok := overlayLoadSettings(); ok {
			overlay.mu.Lock()
			overlay.limit5h, overlay.limitWeek = saved.Limit5h, saved.LimitWeek
			overlay.mu.Unlock()
		}
		select {
		case overlay.refresh <- struct{}{}:
		default:
		}
		return 0
	case wmSetOpacity:
		overlay.mu.Lock()
		overlay.alpha = opacityAlpha(int(wParam))
		overlay.mu.Unlock()
		overlayRender(hwnd)
		overlaySavePosition(hwnd)
		return 0
	case wmRButtonUp:
		overlayMenu(hwnd)
		return 0
	case wmEnterSizeMove:
		overlay.moving = true
		return 0
	case wmExitSizeMove:
		overlay.moving = false
		overlayRender(hwnd) // pulls a window dropped half off the monitor back in
		overlaySavePosition(hwnd)
		return 0
	case wmDpiChanged:
		// Moved to a monitor with another scale: new fonts, and Windows' suggested spot.
		overlaySetDPI(wParam & 0xFFFF)
		suggested := (*winRect)(unsafe.Add(nil, lParam)) // lParam points at a RECT Windows owns
		setWindowPos.Call(hwnd, 0, uintptr(suggested.Left), uintptr(suggested.Top),
			uintptr(suggested.Right-suggested.Left), uintptr(suggested.Bottom-suggested.Top), swpNoZOrder|swpNoActivate)
		// Posted, not drawn here: this can arrive inside overlayRender's own SetWindowPos.
		postMessage.Call(hwnd, wmSnapshot, 0, 0)
		return 0
	case wmDestroy:
		postQuitMessage.Call(0)
		return 0
	}
	r, _, _ := defWindowProc.Call(hwnd, uintptr(message), wParam, lParam)
	return r
}

func overlayMenu(hwnd uintptr) {
	menu, _, _ := createPopupMenu.Call()
	toggleText := "Mở rộng (bấm đúp)"
	if overlaySnapshot().expanded {
		toggleText = "Thu gọn (bấm đúp)"
	}
	toggle, _ := windows.UTF16PtrFromString(toggleText)
	appendMenu.Call(menu, 0, menuToggle, uintptr(unsafe.Pointer(toggle)))
	overlay.mu.Lock()
	window, alpha, pct := overlay.window, overlay.alpha, overlay.textPct
	overlay.mu.Unlock()
	submenu := func(title string, count int, id func(int) uintptr, text func(int) string, checked func(int) bool) {
		sub, _, _ := createPopupMenu.Call()
		for i := 0; i < count; i++ {
			flags := uintptr(0)
			if checked(i) {
				flags = mfChecked
			}
			item, _ := windows.UTF16PtrFromString(text(i))
			appendMenu.Call(sub, flags, id(i), uintptr(unsafe.Pointer(item)))
		}
		name, _ := windows.UTF16PtrFromString(title)
		appendMenu.Call(menu, mfPopup, sub, uintptr(unsafe.Pointer(name))) // destroyed with menu
	}
	submenu("Kỳ (như wj usage)", len(overlayWindows), func(i int) uintptr { return uintptr(menuSince + i) },
		func(i int) string { return overlayWindowNames[i] }, func(i int) bool { return overlayWindows[i] == window })
	submenu("Độ trong suốt", len(overlayOpacities), func(i int) uintptr { return uintptr(menuOpacity + i) },
		func(i int) string { return fmt.Sprintf("%d%%", overlayOpacities[i]) },
		func(i int) bool { return opacityAlpha(overlayOpacities[i]) == alpha })
	submenu("Cỡ chữ", len(overlayTextSizes), func(i int) uintptr { return uintptr(menuText + i) },
		func(i int) string { return fmt.Sprintf("%d%%", overlayTextSizes[i]) }, func(i int) bool { return overlayTextSizes[i] == pct })
	// Limits take a number, which a menu cannot ask for: say the command instead.
	hint, _ := windows.UTF16PtrFromString("Hạn mức: wj overlay -limit-5h 20M -limit-week 500M")
	appendMenu.Call(menu, mfGrayed, 0, uintptr(unsafe.Pointer(hint)))
	label, _ := windows.UTF16PtrFromString("Đóng overlay")
	appendMenu.Call(menu, 0, menuClose, uintptr(unsafe.Pointer(label)))
	setForegroundWindow.Call(hwnd)
	var at winPoint
	getCursorPos.Call(uintptr(unsafe.Pointer(&at)))
	choice, _, _ := trackPopupMenu.Call(menu, tpmReturnCmd, uintptr(at.X), uintptr(at.Y), 0, hwnd, 0)
	destroyMenu.Call(menu)
	switch choice {
	case menuClose:
		destroyWindow.Call(hwnd)
	case menuToggle:
		overlayToggle(hwnd)
	default:
		overlayChoose(hwnd, int(choice))
	}
}

// overlayChoose applies a time window, opacity or text size picked in the menu, and saves it.
func overlayChoose(hwnd uintptr, choice int) {
	switch {
	case choice >= menuSince && choice < menuSince+len(overlayWindows):
		overlay.mu.Lock()
		overlay.window = overlayWindows[choice-menuSince]
		overlay.title = overlayPeriodTag(overlay.window)
		overlay.mu.Unlock()
		select {
		case overlay.refresh <- struct{}{}:
		default:
		}
	case choice >= menuOpacity && choice < menuOpacity+len(overlayOpacities):
		overlay.mu.Lock()
		overlay.alpha = opacityAlpha(overlayOpacities[choice-menuOpacity])
		overlay.mu.Unlock()
	case choice >= menuText && choice < menuText+len(overlayTextSizes):
		overlay.textPct = overlayTextSizes[choice-menuText]
		overlaySetDPI(uintptr(overlay.scale*96 + 0.5))
	default:
		return
	}
	overlayRender(hwnd)
	overlaySavePosition(hwnd)
}

func textWidth(hdc uintptr, text string) (int32, int32) {
	units, _ := windows.UTF16FromString(text)
	var size winSize
	getTextExtent.Call(hdc, uintptr(unsafe.Pointer(&units[0])), uintptr(len(units)-1), uintptr(unsafe.Pointer(&size)))
	return size.CX, size.CY
}

func drawText(hdc uintptr, x, y int32, text string, color uint32) {
	units, _ := windows.UTF16FromString(text)
	setTextColor.Call(hdc, uintptr(color))
	textOut.Call(hdc, uintptr(x), uintptr(y), uintptr(unsafe.Pointer(&units[0])), uintptr(len(units)-1))
}

type overlayLayout struct {
	pad, gap, rowHeight, dot int32
	headerHeight, headerBase int32 // header block (with its rule and bar) and its text height
	barHeight                int32 // 0 without -warn
	chartTop, chartHeight    int32 // the column chart under the header; height 0 without data
	shareHeight              int32 // the share bar under each project row; 0 when expanded
	limitsTop, limitsText    int32 // the limit row: its top and its text height; text 0 without limits
	columns                  []int32
	width, height            int32
}

func (o *overlayView) px(v int) int32 { return int32(float64(v) * o.scale) }

func limitBarHeight() int32 { return overlay.px(3) }

// limitSegment is where limit i of n sits in the limit row: its left edge and width.
func limitSegment(l overlayLayout, i, n int) (x, width int32) {
	width = (l.width - l.pad*2 - l.gap*int32(n-1)) / int32(n)
	return l.pad + int32(i)*(width+l.gap), width
}

func overlayMeasure(hdc uintptr, f overlayFrame) overlayLayout {
	selectObject.Call(hdc, overlay.font)
	l := overlayLayout{pad: overlay.px(8), gap: overlay.px(14)}
	_, textHeight := textWidth(hdc, "Ag")
	l.rowHeight = textHeight + overlay.px(3)
	l.dot, _ = textWidth(hdc, "● ")
	measure := func(cells []string) {
		for len(l.columns) < len(cells) {
			l.columns = append(l.columns, 0)
		}
		for i, cell := range cells {
			if w, _ := textWidth(hdc, cell); w > l.columns[i] {
				l.columns[i] = w
			}
		}
	}
	selectObject.Call(hdc, overlay.big)
	_, l.headerBase = textWidth(hdc, "Ag")
	l.headerHeight = l.headerBase + overlay.px(7)
	if f.limit > 0 {
		l.barHeight = overlay.px(4)
		l.headerHeight += l.barHeight + overlay.px(4)
	}
	if len(f.series) > 0 {
		l.chartTop = l.pad + l.headerHeight
		l.chartHeight = overlay.px(22)
		l.headerHeight += l.chartHeight + overlay.px(4)
	}
	if len(f.limits) > 0 {
		l.limitsTop, l.limitsText = l.pad+l.headerHeight, textHeight
		l.headerHeight += textHeight + overlay.px(2) + limitBarHeight() + overlay.px(5)
	}
	if !f.expanded {
		l.shareHeight = overlay.px(2)
		l.rowHeight += l.shareHeight + overlay.px(1)
	}
	selectObject.Call(hdc, overlay.font)
	rows := int32(len(f.lines))
	if f.expanded {
		measure(overlayDetailHeading)
		rows += 2 // column names and the machine totals
	}
	for _, line := range f.lines {
		measure(line.cells(f.expanded))
	}
	if l.columns == nil {
		measure([]string{"", ""})
	}
	content := l.dot + l.gap*int32(len(l.columns)-1)
	for _, width := range l.columns {
		content += width
	}
	selectObject.Call(hdc, overlay.bold)
	titleWidth, _ := textWidth(hdc, f.title+" ")
	selectObject.Call(hdc, overlay.big)
	totalWidth, _ := textWidth(hdc, f.total)
	headerWidth := titleWidth + totalWidth
	selectObject.Call(hdc, overlay.font)
	footerWidth, _ := textWidth(hdc, " · "+f.footer)
	minimum := headerWidth + footerWidth
	if n := int32(len(f.limits)); n > 0 {
		// every segment as wide as the widest text, so the bars line up
		var widest int32
		for _, limit := range f.limits {
			w, _ := textWidth(hdc, limit.text(time.Now()))
			widest = max(widest, w)
		}
		minimum = max(minimum, widest*n+l.gap*(n-1))
	}
	if f.expanded {
		selectObject.Call(hdc, overlay.bold)
		wholeWidth, _ := textWidth(hdc, f.whole)
		selectObject.Call(hdc, overlay.font)
		minimum = max(minimum, wholeWidth)
	}
	if content < minimum {
		l.columns[0] += minimum - content
		content = minimum
	}
	l.width = content + l.pad*2
	l.height = l.headerHeight + rows*l.rowHeight + l.pad*2
	return l
}

func overlaySnapshot() overlayFrame {
	overlay.mu.Lock()
	defer overlay.mu.Unlock()
	f := overlayFrame{overlay.lines, overlay.title, overlay.total, overlay.footer, overlay.whole, overlay.expanded, overlay.warnLevel, overlay.used, overlay.warn, overlay.alpha, overlay.series, overlay.limits}
	if f.expanded {
		f.lines = overlay.detail
	}
	return f
}

var overlayPlaced bool

// overlayBitmap makes a 32-bit top-down bitmap whose pixels Go can read, all black at first.
func overlayBitmap(hdc uintptr, width, height int32) (uintptr, []byte) {
	info := winBitmapInfo{Width: width, Height: -height, Planes: 1, Bits: 32}
	info.Size = uint32(unsafe.Sizeof(info) - unsafe.Sizeof(info.Colors))
	var bits unsafe.Pointer
	bitmap, _, _ := createDIBSection.Call(hdc, uintptr(unsafe.Pointer(&info)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bitmap == 0 || bits == nil {
		return 0, nil
	}
	return bitmap, unsafe.Slice((*byte)(bits), int(width)*int(height)*4)
}

// overlayRender draws the text twice, then hands Windows per-pixel alpha so only the
// background is see through. GDI cannot write alpha itself. The window has no WM_PAINT:
// this image stays on screen until the next snapshot.
func overlayRender(hwnd uintptr) {
	f := overlaySnapshot()
	screen, _, _ := getDC.Call(0)
	defer releaseDC.Call(0, screen)
	hdc, _, _ := createCompatibleDC.Call(screen)
	defer deleteDC.Call(hdc)
	setBkMode.Call(hdc, 1)
	l := overlayMeasure(hdc, f)
	colourBitmap, colour := overlayBitmap(hdc, l.width, l.height)
	maskBitmap, mask := overlayBitmap(hdc, l.width, l.height)
	defer deleteObject.Call(colourBitmap)
	defer deleteObject.Call(maskBitmap)
	if colour == nil || mask == nil {
		return
	}
	previous, _, _ := selectObject.Call(hdc, maskBitmap)
	overlayDraw(hdc, l, f, true)
	selectObject.Call(hdc, colourBitmap)
	overlayDraw(hdc, l, f, false)
	gdiFlush.Call()
	composeOverlay(colour, mask, int(l.width), int(l.height), int(overlay.px(6)), bgPixel, f.alpha, haloAlpha, int(overlay.px(2)))
	composeBorder(colour, int(l.width), int(l.height), int(overlay.px(6)), borderPixel, borderAlpha, int(l.pad), int(l.pad+l.headerBase+overlay.px(3)), int(overlay.px(1)))
	width, height, pad, thin := int(l.width), int(l.height), int(l.pad), int(overlay.px(1))
	if f.limit > 0 {
		barY := int(l.pad + l.headerBase + overlay.px(3) + overlay.px(4))
		barWidth := width - pad*2
		composeBar(colour, width, height, pad, barY, barWidth, int(l.barHeight), barFill(f.used, f.limit, barWidth), borderPixel, barPixels[f.warnLevel])
	}
	if l.chartHeight > 0 {
		composeColumns(colour, width, height, pad, int(l.chartTop), width-pad*2, int(l.chartHeight), int(overlay.px(8)),
			f.series, pastColumnPixel, barPixels[f.warnLevel], borderPixel)
	}
	for i, limit := range f.limits {
		x, segment := limitSegment(l, i, len(f.limits))
		y := int(l.limitsTop + l.limitsText + overlay.px(2))
		composeBar(colour, width, height, int(x), y, int(segment), int(limitBarHeight()),
			barFill(limit.Used, limit.Limit, int(segment)), borderPixel, barPixels[limit.level()])
	}
	if l.shareHeight > 0 {
		// each project's share of the period's total, under its row
		for i, line := range f.lines {
			rowBottom := int(l.pad + l.headerHeight + int32(i+1)*l.rowHeight)
			x := pad + int(l.dot)
			barWidth := width - pad - x
			fillRect(colour, width, height, x, rowBottom-int(l.shareHeight), x+barFill(line.Total, f.used, barWidth), rowBottom, sharePixel)
		}
	}
	if f.expanded {
		// a rule under the column names
		composeRule(colour, width, height, pad, width-pad, int(l.pad+l.headerHeight+l.rowHeight)-thin-1, thin, borderPixel)
	}

	if !overlayPlaced {
		overlayPlaced = true
		x, y := overlayStartPosition(l.width)
		setWindowPos.Call(hwnd, 0, uintptr(x), uintptr(y), uintptr(l.width), uintptr(l.height), swpNoZOrder|swpNoActivate)
		showWindow.Call(hwnd, swShowNoActive)
	}
	size := winSize{l.width, l.height}
	var source winPoint
	blend := [4]byte{0, 0, 255, acSrcAlpha}
	var moveTo *winPoint // nil keeps the window where it is
	if position, moved := overlayKeepOnScreen(hwnd, l.width, l.height); moved {
		moveTo = &position
	}
	updateLayeredWindow.Call(hwnd, screen, uintptr(unsafe.Pointer(moveTo)), uintptr(unsafe.Pointer(&size)), hdc, uintptr(unsafe.Pointer(&source)), 0, uintptr(unsafe.Pointer(&blend)), ulwAlpha)
	selectObject.Call(hdc, previous)
}

// overlayKeepOnScreen keeps the window inside the work area of the monitor it is on
// (taskbar excluded), on both axes. Not during a drag: the user may be crossing monitors.
func overlayKeepOnScreen(hwnd uintptr, width, height int32) (winPoint, bool) {
	var rect winRect
	getWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rect)))
	if overlay.moving {
		return winPoint{}, false
	}
	monitor, _, _ := monitorFromRect.Call(uintptr(unsafe.Pointer(&rect)), monitorNearest)
	var info winMonitorInfo
	info.Size = uint32(unsafe.Sizeof(info))
	if monitor == 0 {
		return winPoint{}, false
	}
	if ok, _, _ := getMonitorInfo.Call(monitor, uintptr(unsafe.Pointer(&info))); ok == 0 || info.Work.Right <= info.Work.Left {
		return winPoint{}, false
	}
	at := overlayFit(rect, info.Work, width, height)
	return at, at.X != rect.Left || at.Y != rect.Top
}

// overlayFit places a window that was at rect and is now width x height inside work.
// On the right half of the monitor it grows to the left, on the bottom half it grows up,
// so expanding never runs off the edge. Too big for the work area: its top left stays in.
func overlayFit(rect, work winRect, width, height int32) winPoint {
	x, y := rect.Left, rect.Top
	if rect.Right-rect.Left != width && rect.Left+rect.Right > work.Left+work.Right {
		x = rect.Right - width
	}
	if rect.Bottom-rect.Top != height && rect.Top+rect.Bottom > work.Top+work.Bottom {
		y = rect.Bottom - height
	}
	x = max(min(x, work.Right-width), work.Left)
	y = max(min(y, work.Bottom-height), work.Top)
	return winPoint{x, y}
}

// overlayStartPosition restores the last spot if that monitor is still there.
func overlayStartPosition(width int32) (int32, int32) {
	if data, err := os.ReadFile(overlay.statePath); err == nil {
		var saved overlaySettings
		if json.Unmarshal(data, &saved) == nil {
			probe := winRect{saved.X, saved.Y, saved.X + 40, saved.Y + 40}
			if monitor, _, _ := monitorFromRect.Call(uintptr(unsafe.Pointer(&probe)), 0); monitor != 0 {
				var info winMonitorInfo
				info.Size = uint32(unsafe.Sizeof(info))
				getMonitorInfo.Call(monitor, uintptr(unsafe.Pointer(&info)))
				if over := saved.X + width - info.Work.Right; info.Work.Right != 0 && over > 0 {
					saved.X -= over // saved when the window was narrower
				}
				return saved.X, saved.Y
			}
		}
	}
	screen, _, _ := getSystemMetrics.Call(0)
	margin := overlay.px(20)
	return int32(screen) - width - margin, margin
}

func overlaySavePosition(hwnd uintptr) {
	var rect winRect
	getWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rect)))
	overlay.mu.Lock()
	saved := overlaySettings{X: rect.Left, Y: rect.Top, Since: overlay.window, Opacity: (int(overlay.alpha)*100 + 127) / 255, Text: overlay.textPct,
		Limit5h: overlay.limit5h, LimitWeek: overlay.limitWeek}
	overlay.mu.Unlock()
	data, err := json.Marshal(saved)
	if err != nil {
		return
	}
	if os.MkdirAll(filepath.Dir(overlay.statePath), 0o755) == nil {
		_ = os.WriteFile(overlay.statePath, data, 0o644)
	}
}

// overlaySaveLimits writes the limits given on the command line into overlay.json,
// keeping everything else in it. "off" clears a limit; a flag not given keeps it.
func overlaySaveLimits(path string, flags *flag.FlagSet, limit5h, limitWeek string) error {
	var saved overlaySettings
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &saved)
	}
	clean := func(text string) string {
		if t := strings.ToLower(strings.TrimSpace(text)); t == "off" || t == "0" {
			return ""
		}
		return strings.TrimSpace(text)
	}
	flags.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "limit-5h":
			saved.Limit5h = clean(limit5h)
		case "limit-week":
			saved.LimitWeek = clean(limitWeek)
		}
	})
	data, err := json.Marshal(saved)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("lưu hạn mức: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("lưu hạn mức: %w", err)
	}
	return nil
}

// overlayDraw writes all the text on the selected bitmap; asMask draws every colour white.
func overlayDraw(hdc uintptr, l overlayLayout, f overlayFrame, asMask bool) {
	paint := func(color uint32) uint32 {
		if asMask {
			return 0xFFFFFF
		}
		return color
	}
	y := l.pad
	selectObject.Call(hdc, overlay.bold)
	title := f.title + " "
	titleWidth, titleHeight := textWidth(hdc, title)
	drawText(hdc, l.pad, y+l.headerBase-titleHeight, title, paint(textColor))
	selectObject.Call(hdc, overlay.big)
	totalWidth, _ := textWidth(hdc, f.total)
	drawText(hdc, l.pad+titleWidth, y, f.total, paint([]uint32{textColor, warnColor, overColor}[f.warnLevel]))
	selectObject.Call(hdc, overlay.font)
	footer := " · " + f.footer
	_, footerHeight := textWidth(hdc, footer)
	drawText(hdc, l.pad+titleWidth+totalWidth, y+l.headerBase-footerHeight, footer, paint(mutedColor))
	now := time.Now()
	for i, limit := range f.limits {
		x, _ := limitSegment(l, i, len(f.limits))
		drawText(hdc, x, l.limitsTop, limit.text(now), paint([]uint32{mutedColor, warnColor, overColor}[limit.level()]))
	}
	y += l.headerHeight - l.rowHeight
	firstNumber := 1
	if f.expanded {
		firstNumber = overlayFirstNumber
	}
	row := func(cells []string, color func(column int) uint32) {
		y += l.rowHeight
		x := l.pad + l.dot
		for i, cell := range cells {
			at := x
			if i >= firstNumber {
				cellWidth, _ := textWidth(hdc, cell)
				at += l.columns[i] - cellWidth
			}
			drawText(hdc, at, y, cell, paint(color(i)))
			x += l.columns[i] + l.gap
		}
	}
	if f.expanded {
		row(overlayDetailHeading, func(int) uint32 { return mutedColor })
	}
	for _, line := range f.lines {
		if line.Active {
			drawText(hdc, l.pad, y+l.rowHeight, "●", paint(activeColor))
		}
		cells := line.cells(f.expanded)
		row(cells, func(column int) uint32 {
			if !f.expanded {
				// compact: name and token bright, the money muted
				return []uint32{textColor, textColor, mutedColor}[column]
			}
			if column == 0 || column >= len(cells)-1 {
				return textColor
			}
			return mutedColor
		})
	}
	if f.expanded {
		selectObject.Call(hdc, overlay.bold)
		drawText(hdc, l.pad, y+l.rowHeight, f.whole, paint(textColor))
	}
}
