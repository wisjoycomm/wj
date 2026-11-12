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

	wmDestroy         = 0x0002
	wmClose           = 0x0010
	wmLButtonDown     = 0x0201
	wmLButtonDblClk   = 0x0203
	smXVirtualScreen  = 76
	smCXVirtualScreen = 78
	menuToggle        = 2
	wmRButtonUp       = 0x0205
	wmExitSizeMove    = 0x0232
	wmSnapshot        = 0x8001
	wmSysCommand      = 0x0112
	scDragMove        = 0xF012
	csDblClks         = 0x0008
	wsPopup           = 0x80000000
	wsExTopmost       = 0x00000008
	wsExToolWindow    = 0x00000080
	wsExLayered       = 0x00080000
	wsExNoActivate    = 0x08000000
	ulwAlpha          = 2
	acSrcAlpha        = 1
	swShowNoActive    = 4
	swpNoActivate     = 0x0010
	swpNoZOrder       = 0x0004
	swpNoMove         = 0x0002
	tpmReturnCmd      = 0x0100
	menuClose         = 1
	logPixelsY        = 90
	bgAlpha           = 235 // background only (0-255); text is always fully opaque
	borderAlpha       = 255
	textColor         = 0xFFFFFF
	mutedColor        = 0xD8D2CC
	activeColor       = 0x80DE4A
	warnColor         = 0x3CC8FF
	overColor         = 0x6060FF
	detachedProcess   = 0x00000008
)

var bgPixel = [3]byte{0x1D, 0x18, 0x16}     // blue, green, red
var borderPixel = [3]byte{0x6A, 0x62, 0x5C} // blue, green, red

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	gdi32    = windows.NewLazySystemDLL("gdi32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	getModuleHandle     = kernel32.NewProc("GetModuleHandleW")
	setProcessDPIAware  = user32.NewProc("SetProcessDPIAware")
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
}

// Columns from overlayFirstNumber on are right aligned when expanded.
var overlayDetailHeading = []string{"Project", "App", "Lúc", "Model", "Vào", "Ra", "Cache", "Tổng", "≈USD"}

const overlayFirstNumber = 4

func (line overlayLine) cells(expanded bool) []string {
	if !expanded {
		return []string{line.Project, line.Tokens}
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
}

type overlayView struct {
	mu         sync.Mutex
	footer     string
	title      string
	total      string
	whole      string
	lines      []overlayLine
	detail     []overlayLine
	expanded   bool
	warn       int64 // the user's limit for the window's total; 0 means none
	warnLevel  int
	hwnd       uintptr
	font, bold uintptr
	big        uintptr // bold, larger: the total
	scale      float64
	statePath  string
}

var overlay overlayView

func runOverlay(args []string, out io.Writer, home string) error {
	flags := flag.NewFlagSet("overlay", flag.ContinueOnError)
	flags.SetOutput(out)
	window := flags.Duration("since", 24*time.Hour, "khoảng thời gian, ví dụ 1h, 24h, 168h (7 ngày)")
	foreground := flags.Bool("run", false, "chạy cửa sổ ngay trong tiến trình này")
	warn := flags.String("warn", "", "đổi màu dòng tổng khi gần tới ngưỡng này, ví dụ 5M, 500k")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
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
		return errors.New("cách dùng: wj overlay [-since 24h] [-warn 5M] | wj overlay stop")
	}
	if existing != 0 {
		fmt.Fprintln(out, "Overlay đang mở rồi. Đóng: wj overlay stop")
		return nil
	}
	if *foreground {
		return overlayLoop(*window, home)
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "overlay", "-run", "-since", window.String(), "-warn", *warn)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: detachedProcess | windows.CREATE_NEW_PROCESS_GROUP}
	if err = cmd.Start(); err != nil {
		return err
	}
	_ = cmd.Process.Release()
	fmt.Fprintln(out, "Đã mở overlay token. Kéo để đổi chỗ, chuột phải để đóng (hoặc: wj overlay stop).")
	return nil
}

func overlayLoop(window time.Duration, home string) error {
	runtime.LockOSThread()
	overlay.statePath = filepath.Join(stateDir(home), "overlay.json")
	overlay.title = windowTag(window)
	setProcessDPIAware.Call()
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
	hdc, _, _ := getDC.Call(hwnd)
	dpi, _, _ := getDeviceCaps.Call(hdc, logPixelsY)
	releaseDC.Call(hwnd, hdc)
	overlay.scale = float64(dpi) / 96
	overlay.font = overlayFont(10, 400, dpi)
	overlay.bold = overlayFont(10, 600, dpi)
	overlay.big = overlayFont(13, 700, dpi)

	go overlayFeed(window, home, hwnd)

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

func overlayFont(points, weight int, dpi uintptr) uintptr {
	name, _ := windows.UTF16PtrFromString("Segoe UI")
	height := -points * int(dpi) / 72
	// Quality 4 is grayscale smoothing: ClearType's colour fringes need an opaque background.
	font, _, _ := createFont.Call(uintptr(int32ToPtr(height)), 0, 0, 0, uintptr(weight), 0, 0, 0, 1, 0, 0, 4, 0, uintptr(unsafe.Pointer(name)))
	return font
}

func int32ToPtr(v int) uintptr { return uintptr(int64(v)) }

func overlayFeed(window time.Duration, home string, hwnd uintptr) {
	scanner := newSessionScanner(home)
	for {
		since := time.Now().Add(-window)
		scanner.scan(time.Time{})
		list, total := scanner.projectStatus(since)
		now := time.Now()
		footer := scanner.overlayFooter(now)
		running := runningProjects(list, scanner.openClaudeProjects(), now)
		lines := make([]overlayLine, 0, len(running)+1)
		if len(running) == 0 {
			lines = append(lines, overlayLine{Project: "Không có project đang chạy"})
		}
		for _, row := range running {
			lines = append(lines, overlayLine{Project: cutRunes(row.Project, 18), Tokens: compactTokens(row.Total), Active: now.Sub(row.Last) < activeWindow})
		}
		var detail []overlayLine
		for _, entry := range detailEntries(overlayDetailRows(running, list), scanner.modelsByProject(since), overlayMaxLines) {
			detail = append(detail, overlayDetailLine(entry, now))
		}
		whole := scanner.machineTotals(now)
		whole += fmt.Sprintf(" · %s ≈%s theo giá API %s", windowTag(window), costText(total), pricesAsOf)
		shown := compactTokens(total.Total)
		if overlay.warn > 0 {
			shown += "/" + compactTokens(overlay.warn)
		}
		overlay.mu.Lock()
		overlay.lines, overlay.detail, overlay.total, overlay.footer, overlay.whole = lines, detail, shown, footer, whole
		overlay.warnLevel = warnLevel(total.Total, overlay.warn)
		overlay.mu.Unlock()
		postMessage.Call(hwnd, wmSnapshot, 0, 0)
		time.Sleep(overlayRefresh)
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
	case wmRButtonUp:
		overlayMenu(hwnd)
		return 0
	case wmExitSizeMove:
		overlaySavePosition(hwnd)
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
	}
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
	headerHeight, headerBase int32 // header block (with its rule) and its text height
	columns                  []int32
	width, height            int32
}

func (o *overlayView) px(v int) int32 { return int32(float64(v) * o.scale) }

func overlayMeasure(hdc uintptr, f overlayFrame) overlayLayout {
	selectObject.Call(hdc, overlay.font)
	l := overlayLayout{pad: overlay.px(8), gap: overlay.px(14)}
	_, textHeight := textWidth(hdc, "Ag")
	l.rowHeight = textHeight + overlay.px(3)
	l.dot, _ = textWidth(hdc, "● ")
	measure := func(cells []string) {
		if l.columns == nil {
			l.columns = make([]int32, len(cells))
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
	if f.expanded {
		wholeWidth, _ := textWidth(hdc, f.whole)
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
	f := overlayFrame{overlay.lines, overlay.title, overlay.total, overlay.footer, overlay.whole, overlay.expanded, overlay.warnLevel}
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
	composeOverlay(colour, mask, int(l.width), int(l.height), int(overlay.px(6)), bgPixel, bgAlpha)
	composeBorder(colour, int(l.width), int(l.height), int(overlay.px(6)), borderPixel, borderAlpha, int(l.pad), int(l.pad+l.headerBase+overlay.px(3)), int(overlay.px(1)))

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
	if position, moved := overlayKeepOnScreen(hwnd, l.width); moved {
		moveTo = &position
	}
	updateLayeredWindow.Call(hwnd, screen, uintptr(unsafe.Pointer(moveTo)), uintptr(unsafe.Pointer(&size)), hdc, uintptr(unsafe.Pointer(&source)), 0, uintptr(unsafe.Pointer(&blend)), ulwAlpha)
	selectObject.Call(hdc, previous)
}

// overlayKeepOnScreen holds the right edge still when a window on the right side of
// the screen changes width, so expanding it never runs off the screen.
func overlayKeepOnScreen(hwnd uintptr, width int32) (winPoint, bool) {
	var rect winRect
	getWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rect)))
	left, _, _ := getSystemMetrics.Call(smXVirtualScreen)
	span, _, _ := getSystemMetrics.Call(smCXVirtualScreen)
	middle := int32(left) + int32(span)/2
	if rect.Right-rect.Left == width || (rect.Left+rect.Right)/2 <= middle {
		return winPoint{}, false
	}
	return winPoint{rect.Right - width, rect.Top}, true
}

// overlayStartPosition restores the last spot if that monitor is still there.
func overlayStartPosition(width int32) (int32, int32) {
	if data, err := os.ReadFile(overlay.statePath); err == nil {
		var saved struct{ X, Y int32 }
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
	data, err := json.Marshal(struct{ X, Y int32 }{rect.Left, rect.Top})
	if err != nil {
		return
	}
	if os.MkdirAll(filepath.Dir(overlay.statePath), 0o755) == nil {
		_ = os.WriteFile(overlay.statePath, data, 0o644)
	}
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
			if column == 0 || column >= len(cells)-1 {
				return textColor
			}
			return mutedColor
		})
	}
	if f.expanded {
		drawText(hdc, l.pad, y+l.rowHeight, f.whole, paint(mutedColor))
	}
}
