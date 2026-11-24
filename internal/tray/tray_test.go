package tray

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"
)

func TestIconBytesValidICOAndVisual(t *testing.T) {
	data := IconBytes()
	const expectedLen = 6 + 16 + 40 + 32*32*4 + 32*4
	if len(data) != expectedLen {
		t.Fatalf("unexpected ICO byte length: got %d, want %d", len(data), expectedLen)
	}

	// ICONDIR: 00 00 01 00 01 00
	wantHeader := []byte{0x00, 0x00, 0x01, 0x00, 0x01, 0x00}
	if !bytes.Equal(data[:6], wantHeader) {
		t.Fatalf("unexpected ICONDIR header: got %x, want %x", data[:6], wantHeader)
	}

	// ICONDIRENTRY
	if data[6] != 32 || data[7] != 32 {
		t.Fatalf("unexpected dimensions in ICONDIRENTRY: got %dx%d, want 32x32", data[6], data[7])
	}
	if planes := binary.LittleEndian.Uint16(data[10:12]); planes != 1 {
		t.Fatalf("unexpected planes: got %d, want 1", planes)
	}
	if bpp := binary.LittleEndian.Uint16(data[12:14]); bpp != 32 {
		t.Fatalf("unexpected bit count: got %d, want 32", bpp)
	}
	if bytesInRes := binary.LittleEndian.Uint32(data[14:18]); bytesInRes != 40+32*32*4+32*4 {
		t.Fatalf("unexpected dwBytesInRes: got %d, want %d", bytesInRes, 40+32*32*4+32*4)
	}
	if imgOffset := binary.LittleEndian.Uint32(data[18:22]); imgOffset != 22 {
		t.Fatalf("unexpected dwImageOffset: got %d, want 22", imgOffset)
	}

	// BITMAPINFOHEADER
	dib := data[22:62]
	if biSize := binary.LittleEndian.Uint32(dib[0:4]); biSize != 40 {
		t.Fatalf("unexpected DIB header size: got %d, want 40", biSize)
	}
	if biWidth := int32(binary.LittleEndian.Uint32(dib[4:8])); biWidth != 32 {
		t.Fatalf("unexpected biWidth: got %d, want 32", biWidth)
	}
	if biHeight := int32(binary.LittleEndian.Uint32(dib[8:12])); biHeight != 64 {
		t.Fatalf("unexpected biHeight: got %d, want 64", biHeight)
	}
	if biBitCount := binary.LittleEndian.Uint16(dib[14:16]); biBitCount != 32 {
		t.Fatalf("unexpected biBitCount: got %d, want 32", biBitCount)
	}
	if biSizeImage := binary.LittleEndian.Uint32(dib[20:24]); biSizeImage != 32*32*4 {
		t.Fatalf("unexpected biSizeImage: got %d, want %d", biSizeImage, 32*32*4)
	}

	xorPixels := data[62 : 62+32*32*4]
	andMask := data[62+32*32*4:]

	// The artwork is the blade mark: it must carry real transparency in the corners and at least
	// one strongly saturated red pixel (the blade) rather than a flat placeholder.
	cornerOff := (31*32 + 0) * 4
	if xorPixels[cornerOff+3] != 0 {
		t.Fatalf("expected outer corner alpha == 0, got %d", xorPixels[cornerOff+3])
	}
	if andMask[31*4]&0x80 == 0 {
		t.Fatalf("expected AND mask bit set for transparent corner pixel")
	}

	opaque, redDominant := 0, 0
	for i := 0; i < len(xorPixels); i += 4 {
		b, g, r, a := xorPixels[i], xorPixels[i+1], xorPixels[i+2], xorPixels[i+3]
		if a > 160 {
			opaque++
			if r > 90 && r > g*2 && r > b*2 {
				redDominant++
			}
		}
	}
	if opaque < 100 {
		t.Fatalf("expected a substantial opaque area, got %d pixels", opaque)
	}
	if redDominant < 5 {
		t.Fatalf("expected red blade pixels in the tray icon, got %d", redDominant)
	}
}

func TestIconBytesReturnsCopy(t *testing.T) {
	first := IconBytes()
	orig := first[0]
	first[0] ^= 0xFF

	second := IconBytes()
	if second[0] != orig {
		t.Fatalf("mutating returned IconBytes slice affected subsequent calls")
	}
}

func TestStopWithoutStartIsNoOp(t *testing.T) {
	// Must not panic or block when Start has never been called.
	Stop()
	Stop()
}

func TestInvokeAsync(t *testing.T) {
	invokeAsync(nil)

	done := make(chan struct{})
	invokeAsync(func() {
		close(done)
	})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for invokeAsync callback")
	}
}
