package resolver

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32DLL  = windows.NewLazySystemDLL("user32.dll")
	shell32DLL = windows.NewLazySystemDLL("shell32.dll")
	gdi32DLL   = windows.NewLazySystemDLL("gdi32.dll")

	procPrivateExtractIconsW = user32DLL.NewProc("PrivateExtractIconsW")
	procDestroyIcon          = user32DLL.NewProc("DestroyIcon")
	procGetIconInfo          = user32DLL.NewProc("GetIconInfo")
	procGetDC                = user32DLL.NewProc("GetDC")
	procReleaseDC            = user32DLL.NewProc("ReleaseDC")

	procExtractIconExW = shell32DLL.NewProc("ExtractIconExW")
	procSHGetFileInfoW = shell32DLL.NewProc("SHGetFileInfoW")

	procDeleteObject = gdi32DLL.NewProc("DeleteObject")
	procGetObjectW   = gdi32DLL.NewProc("GetObjectW")
	procGetDIBits    = gdi32DLL.NewProc("GetDIBits")
)

const (
	shgfiIcon              = 0x000000100
	shgfiLargeIcon         = 0x000000000
	shgfiUseFileAttributes = 0x000000010
	fileAttributeNormal    = 0x00000080

	biRGB        = 0
	dibRGBColors = 0
	maxPathUTF16 = 260
)

type winIconInfo struct {
	fIcon    int32
	xHotspot uint32
	yHotspot uint32
	hbmMask  uintptr
	hbmColor uintptr
}

type winBitmap struct {
	bmType       int32
	bmWidth      int32
	bmHeight     int32
	bmWidthBytes int32
	bmPlanes     uint16
	bmBitsPixel  uint16
	bmBits       uintptr
}

type winBitmapInfoHeader struct {
	biSize          uint32
	biWidth         int32
	biHeight        int32
	biPlanes        uint16
	biBitCount      uint16
	biCompression   uint32
	biSizeImage     uint32
	biXPelsPerMeter int32
	biYPelsPerMeter int32
	biClrUsed       uint32
	biClrImportant  uint32
}

type winSHFileInfoW struct {
	hIcon         uintptr
	iIcon         int32
	dwAttributes  uint32
	szDisplayName [maxPathUTF16]uint16
	szTypeName    [80]uint16
}

// IconHash returns a deterministic lowercase MD5 hex digest for a normalized
// file or folder path.
func IconHash(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		sum := md5.Sum(nil)
		return hex.EncodeToString(sum[:])
	}
	normalized := strings.ToLower(filepath.Clean(trimmed))
	sum := md5.Sum([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

// ExtractAndCacheIcon extracts the Win32 icon for targetPath (file or folder),
// converts it to a PNG image, atomically writes it into iconsDir, and returns
// the portable relative path "icons/<hash>.png".
func ExtractAndCacheIcon(targetPath string, iconsDir string) (string, error) {
	trimmedTarget := strings.TrimSpace(targetPath)
	if trimmedTarget == "" {
		return "", fmt.Errorf("target path is empty")
	}
	if strings.TrimSpace(iconsDir) == "" {
		return "", fmt.Errorf("icons directory is empty")
	}

	cleanedTarget := filepath.Clean(expandEnvWindows(trimmedTarget))
	if err := os.MkdirAll(iconsDir, 0o755); err != nil {
		return "", fmt.Errorf("create icons dir: %w", err)
	}

	// The cache key is the target path, so an existing PNG is the icon for this target. Hitting
	// it avoids re-extracting the same icon on every panel refresh. An updated target (a portable
	// exe swapped in place, an app updated to a new icon) is re-extracted: the cached file older
	// than the target means the artwork may have changed.
	filename := IconHash(cleanedTarget) + ".png"
	outPath := filepath.Join(iconsDir, filename)
	if info, statErr := os.Stat(outPath); statErr == nil && info.Mode().IsRegular() && info.Size() > 0 {
		targetInfo, targetErr := os.Stat(cleanedTarget)
		stale := targetErr == nil && !targetInfo.IsDir() && targetInfo.ModTime().After(info.ModTime())
		if !stale {
			return "icons/" + filename, nil
		}
	}

	img, err := extractIconImage(cleanedTarget)
	if err != nil {
		return "", err
	}

	if err := writePNGAtomic(iconsDir, outPath, img); err != nil {
		return "", err
	}

	return "icons/" + filename, nil
}

// extractIconImage returns the first usable icon for targetPath. Explorer's own strategies are
// tried in order and a blank result is rejected, because some executables hand out an empty 48px
// icon from PrivateExtractIcons while still having a valid one through ExtractIconEx or the shell.
func extractIconImage(targetPath string) (*image.NRGBA, error) {
	pathPtr, err := windows.UTF16PtrFromString(targetPath)
	if err != nil {
		return nil, fmt.Errorf("encode target path: %w", err)
	}

	strategies := make([]func() uintptr, 0, 4)
	ext := strings.ToLower(filepath.Ext(targetPath))
	if ext == ".exe" || ext == ".dll" || ext == ".ico" {
		strategies = append(strategies,
			func() uintptr { return privateExtractIcon48(pathPtr) },
			func() uintptr { return extractLargeIconEx(pathPtr) },
		)
	}
	strategies = append(strategies, func() uintptr { return shGetFileIcon(pathPtr, 0, shgfiIcon|shgfiLargeIcon) })
	if !pathExists(targetPath) {
		// Missing target: ask the shell for the icon of its type instead of the file itself.
		strategies = append(strategies, func() uintptr {
			return shGetFileIcon(pathPtr, fileAttributeNormal, shgfiIcon|shgfiLargeIcon|shgfiUseFileAttributes)
		})
	}

	var lastErr error
	for _, next := range strategies {
		hIcon := next()
		if hIcon == 0 {
			continue
		}
		img, convErr := hiconToNRGBA(hIcon)
		destroyIcon(hIcon)
		if convErr != nil {
			lastErr = convErr
			continue
		}
		if !imageHasContent(img) {
			lastErr = fmt.Errorf("icon for %q is blank", targetPath)
			continue
		}
		return img, nil
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("failed to extract icon for %q", targetPath)
}

// imageHasContent reports whether an icon has at least one visible pixel, which rejects the fully
// transparent placeholders some applications ship.
func imageHasContent(img *image.NRGBA) bool {
	if img == nil {
		return false
	}
	for i := 3; i < len(img.Pix); i += 4 {
		if img.Pix[i] > 8 {
			return true
		}
	}
	return false
}

func privateExtractIcon48(pathPtr *uint16) uintptr {
	var hIcon uintptr
	var iconID uint32
	ret, _, _ := procPrivateExtractIconsW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		0,
		48,
		48,
		uintptr(unsafe.Pointer(&hIcon)),
		uintptr(unsafe.Pointer(&iconID)),
		1,
		0,
	)
	if ret == 0 || ret == 0xffffffff {
		return 0
	}
	return hIcon
}

func extractLargeIconEx(pathPtr *uint16) uintptr {
	var hIconLarge uintptr
	ret, _, _ := procExtractIconExW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		0,
		uintptr(unsafe.Pointer(&hIconLarge)),
		0,
		1,
	)
	if ret == 0 || ret == 0xffffffff {
		return 0
	}
	return hIconLarge
}

func shGetFileIcon(pathPtr *uint16, attrs uint32, flags uint32) uintptr {
	var sfi winSHFileInfoW
	ret, _, _ := procSHGetFileInfoW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(attrs),
		uintptr(unsafe.Pointer(&sfi)),
		unsafe.Sizeof(sfi),
		uintptr(flags),
	)
	if ret == 0 {
		return 0
	}
	return sfi.hIcon
}

func destroyIcon(hIcon uintptr) {
	if hIcon != 0 {
		procDestroyIcon.Call(hIcon)
	}
}

func deleteObject(hObj uintptr) {
	if hObj != 0 {
		procDeleteObject.Call(hObj)
	}
}

func releaseDC(hWnd, hdc uintptr) {
	if hdc != 0 {
		procReleaseDC.Call(hWnd, hdc)
	}
}

func hiconToNRGBA(hIcon uintptr) (*image.NRGBA, error) {
	var info winIconInfo
	ret, _, err := procGetIconInfo.Call(hIcon, uintptr(unsafe.Pointer(&info)))
	if ret == 0 {
		return nil, fmt.Errorf("GetIconInfo failed: %w", err)
	}
	defer deleteObject(info.hbmColor)
	defer deleteObject(info.hbmMask)

	hdc, _, err := procGetDC.Call(0)
	if hdc == 0 {
		return nil, fmt.Errorf("GetDC failed: %w", err)
	}
	defer releaseDC(0, hdc)

	if info.hbmColor == 0 {
		return monochromeIconToNRGBA(hdc, info.hbmMask)
	}

	bmp, err := getBitmapInfo(info.hbmColor)
	if err != nil {
		return nil, err
	}
	width := int(bmp.bmWidth)
	height := int(bmp.bmHeight)
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("invalid icon dimensions %dx%d", width, height)
	}

	colorBits, err := readBitmapBGRA(hdc, info.hbmColor, width, height)
	if err != nil {
		return nil, err
	}

	hasNonZeroAlpha := false
	pixelCount := width * height
	for i := 0; i < pixelCount; i++ {
		if colorBits[i*4+3] != 0 {
			hasNonZeroAlpha = true
			break
		}
	}

	var maskBits []byte
	if !hasNonZeroAlpha && info.hbmMask != 0 {
		maskBits, _ = readBitmapBGRA(hdc, info.hbmMask, width, height)
	}

	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for i := 0; i < pixelCount; i++ {
		off := i * 4
		alpha := colorBits[off+3]
		if !hasNonZeroAlpha {
			if len(maskBits) == len(colorBits) {
				// Mask black (0) = opaque, white (non-zero) = transparent.
				if maskBits[off] == 0 && maskBits[off+1] == 0 && maskBits[off+2] == 0 {
					alpha = 255
				} else {
					alpha = 0
				}
			} else {
				alpha = 255
			}
		}

		// Convert BGRA -> RGBA.
		img.Pix[off+0] = colorBits[off+2]
		img.Pix[off+1] = colorBits[off+1]
		img.Pix[off+2] = colorBits[off+0]
		img.Pix[off+3] = alpha
	}

	return img, nil
}

func monochromeIconToNRGBA(hdc, hbmMask uintptr) (*image.NRGBA, error) {
	if hbmMask == 0 {
		return nil, fmt.Errorf("icon has neither color nor mask bitmap")
	}
	bmp, err := getBitmapInfo(hbmMask)
	if err != nil {
		return nil, err
	}
	width := int(bmp.bmWidth)
	// Monochrome icons pack AND (top half) and XOR (bottom half) into hbmMask.
	height := int(bmp.bmHeight) / 2
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("invalid monochrome icon dimensions %dx%d", width, bmp.bmHeight)
	}

	maskBits, err := readBitmapBGRA(hdc, hbmMask, width, height*2)
	if err != nil {
		return nil, err
	}

	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	xorOffset := width * height * 4
	for i := 0; i < width*height; i++ {
		off := i * 4
		andZero := maskBits[off] == 0 && maskBits[off+1] == 0 && maskBits[off+2] == 0
		if andZero {
			img.Pix[off+0] = maskBits[xorOffset+off+2]
			img.Pix[off+1] = maskBits[xorOffset+off+1]
			img.Pix[off+2] = maskBits[xorOffset+off+0]
			img.Pix[off+3] = 255
		} else {
			img.Pix[off+3] = 0
		}
	}
	return img, nil
}

func getBitmapInfo(hbm uintptr) (winBitmap, error) {
	var bmp winBitmap
	ret, _, err := procGetObjectW.Call(hbm, unsafe.Sizeof(bmp), uintptr(unsafe.Pointer(&bmp)))
	if ret == 0 {
		return winBitmap{}, fmt.Errorf("GetObjectW failed: %w", err)
	}
	return bmp, nil
}

func readBitmapBGRA(hdc, hbm uintptr, width, height int) ([]byte, error) {
	bmi := winBitmapInfoHeader{
		biSize:        uint32(unsafe.Sizeof(winBitmapInfoHeader{})),
		biWidth:       int32(width),
		biHeight:      -int32(height), // top-down scanlines
		biPlanes:      1,
		biBitCount:    32,
		biCompression: biRGB,
	}

	bits := make([]byte, width*height*4)
	ret, _, err := procGetDIBits.Call(
		hdc,
		hbm,
		0,
		uintptr(height),
		uintptr(unsafe.Pointer(&bits[0])),
		uintptr(unsafe.Pointer(&bmi)),
		dibRGBColors,
	)
	if ret == 0 {
		return nil, fmt.Errorf("GetDIBits failed: %w", err)
	}
	return bits, nil
}

func writePNGAtomic(dir, outPath string, img *image.NRGBA) error {
	tmpFile, err := os.CreateTemp(dir, ".icon-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp icon file: %w", err)
	}
	tmpPath := tmpFile.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if err := png.Encode(tmpFile, img); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("encode png: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp icon file: %w", err)
	}
	if err := os.Rename(tmpPath, outPath); err != nil {
		return fmt.Errorf("rename icon file: %w", err)
	}
	cleanup = false
	return nil
}
