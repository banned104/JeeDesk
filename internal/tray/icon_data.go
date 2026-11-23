package tray

import (
	"encoding/binary"
	"math"
)

const (
	iconSize       = 32
	icoHeaderSize  = 6
	icoEntrySize   = 16
	dibHeaderSize  = 40
	xorDataSize    = iconSize * iconSize * 4
	andMaskRowSize = ((iconSize + 31) / 32) * 4
	andMaskSize    = iconSize * andMaskRowSize
	icoTotalSize   = icoHeaderSize + icoEntrySize + dibHeaderSize + xorDataSize + andMaskSize
)

var cachedIcon = buildIconICO()

// IconBytes returns a fresh copy of the 32x32 32-bit BGRA BMP-in-ICO icon for JeeDesk.
func IconBytes() []byte {
	out := make([]byte, len(cachedIcon))
	copy(out, cachedIcon)
	return out
}

func buildIconICO() []byte {
	buf := make([]byte, icoTotalSize)

	// ICONDIR (6 bytes)
	binary.LittleEndian.PutUint16(buf[0:2], 0) // idReserved = 0
	binary.LittleEndian.PutUint16(buf[2:4], 1) // idType = 1 (icon)
	binary.LittleEndian.PutUint16(buf[4:6], 1) // idCount = 1

	// ICONDIRENTRY (16 bytes)
	entry := buf[6:22]
	entry[0] = iconSize                                                         // bWidth
	entry[1] = iconSize                                                         // bHeight
	entry[2] = 0                                                                // bColorCount
	entry[3] = 0                                                                // bReserved
	binary.LittleEndian.PutUint16(entry[4:6], 1)                                // wPlanes
	binary.LittleEndian.PutUint16(entry[6:8], 32)                               // wBitCount
	binary.LittleEndian.PutUint32(entry[8:12], dibHeaderSize+xorDataSize+andMaskSize) // dwBytesInRes
	binary.LittleEndian.PutUint32(entry[12:16], icoHeaderSize+icoEntrySize)     // dwImageOffset

	// BITMAPINFOHEADER (40 bytes)
	dib := buf[22:62]
	binary.LittleEndian.PutUint32(dib[0:4], dibHeaderSize)
	binary.LittleEndian.PutUint32(dib[4:8], uint32(iconSize))
	binary.LittleEndian.PutUint32(dib[8:12], uint32(iconSize*2)) // XOR + AND height
	binary.LittleEndian.PutUint16(dib[12:14], 1)                 // biPlanes
	binary.LittleEndian.PutUint16(dib[14:16], 32)                // biBitCount
	binary.LittleEndian.PutUint32(dib[16:20], 0)                 // biCompression = BI_RGB
	binary.LittleEndian.PutUint32(dib[20:24], xorDataSize)       // biSizeImage

	xorPixels := buf[62 : 62+xorDataSize]
	andMask := buf[62+xorDataSize : icoTotalSize]

	const supersample = 4
	const invSamples = 1.0 / float64(supersample*supersample)

	for y := 0; y < iconSize; y++ {
		// BMP-in-ICO stores rows bottom-up.
		bmpRow := iconSize - 1 - y
		for x := 0; x < iconSize; x++ {
			var sumR, sumG, sumB, sumA float64
			for sy := 0; sy < supersample; sy++ {
				py := float64(y) + (float64(sy)+0.5)/float64(supersample)
				for sx := 0; sx < supersample; sx++ {
					px := float64(x) + (float64(sx)+0.5)/float64(supersample)
					r, g, b, a := sampleIconPixel(px, py)
					// Premultiply during supersample averaging, then unpremultiply below.
					sumR += r * a
					sumG += g * a
					sumB += b * a
					sumA += a
				}
			}

			alpha := sumA * invSamples
			var r, g, b float64
			if alpha > 1e-6 {
				r = (sumR * invSamples) / alpha
				g = (sumG * invSamples) / alpha
				b = (sumB * invSamples) / alpha
			}

			off := (bmpRow*iconSize + x) * 4
			xorPixels[off+0] = clampByte(b * 255.0)
			xorPixels[off+1] = clampByte(g * 255.0)
			xorPixels[off+2] = clampByte(r * 255.0)
			xorPixels[off+3] = clampByte(alpha * 255.0)

			if alpha < 0.05 {
				// Mark transparent in 1-bit AND mask as fallback.
				andOff := bmpRow*andMaskRowSize + x/8
				andMask[andOff] |= 1 << uint(7-(x%8))
			}
		}
	}

	return buf
}

// sampleIconPixel evaluates the dark-glass rounded square and glowing cyan "J" monogram at (px, py) in [0, 32].
func sampleIconPixel(px, py float64) (r, g, b, a float64) {
	// Rounded square centered at (16, 16) with half-extent 14.2 and corner radius 6.6.
	dBox := sdRoundedBox(px-16.0, py-16.0, 14.2, 14.2, 6.6)
	if dBox > 0.75 {
		return 0, 0, 0, 0
	}

	// Smooth outer edge coverage.
	boxAlpha := smoothstep(0.75, -0.75, dBox)

	// Dark-glass background gradient (deep navy-slate top-left to obsidian bottom-right).
	gradT := clamp01((px*0.35 + py*0.65) / 32.0)
	bgR := lerp(0.086, 0.039, gradT) // #16 -> #0A
	bgG := lerp(0.133, 0.067, gradT) // #22 -> #11
	bgB := lerp(0.220, 0.125, gradT) // #38 -> #20

	// Subtle top-half frosted glass sheen.
	sheen := math.Exp(-math.Pow((py-5.5)/5.0, 2.0)) * 0.10
	bgR = clamp01(bgR + sheen*0.65)
	bgG = clamp01(bgG + sheen*0.85)
	bgB = clamp01(bgB + sheen*1.10)

	// Crisp 1px glass rim highlight near the inner boundary.
	rim := math.Exp(-math.Pow((dBox+0.65)/0.65, 2.0))
	rimDirectional := lerp(0.58, 0.22, clamp01((px+py)/64.0))
	bgR = lerp(bgR, 0.35, rim*rimDirectional)
	bgG = lerp(bgG, 0.78, rim*rimDirectional)
	bgB = lerp(bgB, 0.98, rim*rimDirectional)

	// Distance to the "J" monogram glyph.
	dJ := sdMonogramJ(px, py)

	// Soft cyan ambient glow around the "J".
	if dJ > 0 {
		glow := math.Exp(-dJ * 0.42) * 0.48
		bgR = clamp01(bgR + glow*0.08)
		bgG = clamp01(bgG + glow*0.58)
		bgB = clamp01(bgB + glow*0.95)
	}

	// Crisp glowing cyan/blue gradient fill for the "J" stroke.
	jFill := smoothstep(0.7, -0.7, dJ)
	if jFill > 0 {
		jT := clamp01((py - 7.5) / 16.5)
		// Top: bright ice-cyan (#93E5FF) -> Bottom: electric cyan-blue (#228BE6)
		jR := lerp(0.58, 0.14, jT)
		jG := lerp(0.92, 0.56, jT)
		jB := lerp(1.00, 0.96, jT)

		// Inner core highlight for crisp contrast.
		core := smoothstep(-0.3, -1.5, dJ) * 0.25
		jR = clamp01(jR + core)
		jG = clamp01(jG + core*0.8)
		jB = clamp01(jB + core*0.5)

		bgR = lerp(bgR, jR, jFill)
		bgG = lerp(bgG, jG, jFill)
		bgB = lerp(bgB, jB, jFill)
	}

	return bgR, bgG, bgB, boxAlpha
}

// sdMonogramJ computes the signed distance to a modern geometric "J" monogram.
func sdMonogramJ(px, py float64) float64 {
	const strokeRadius = 1.75

	// Top short horizontal cap/bar so the "J" stays crisp even at 16x16.
	dTop := sdSegment(px, py, 14.5, 8.8, 19.6, 8.8)

	// Main vertical stem.
	dStem := sdSegment(px, py, 19.2, 8.8, 19.2, 18.8)

	// Bottom circular hook centered at (14.5, 18.8) with radius 4.7, sweeping from 0 deg (right) to 165 deg (left-up).
	dArc := sdHookArc(px-14.5, py-18.8, 4.7, 0.0, 165.0*math.Pi/180.0)

	d := math.Min(dTop, math.Min(dStem, dArc))
	return d - strokeRadius
}

func sdRoundedBox(px, py, bx, by, r float64) float64 {
	qx := math.Abs(px) - bx + r
	qy := math.Abs(py) - by + r
	ox := math.Max(qx, 0)
	oy := math.Max(qy, 0)
	return math.Hypot(ox, oy) + math.Min(math.Max(qx, qy), 0) - r
}

func sdSegment(px, py, ax, ay, bx, by float64) float64 {
	pax, pay := px-ax, py-ay
	bax, bay := bx-ax, by-ay
	denom := bax*bax + bay*bay
	if denom == 0 {
		return math.Hypot(pax, pay)
	}
	h := clamp01((pax*bax + pay*bay) / denom)
	return math.Hypot(pax-bax*h, pay-bay*h)
}

// sdHookArc returns unsigned distance to a circular arc of radius r from angle a0 to a1 (radians, positive = +x down to +y).
func sdHookArc(px, py, r, a0, a1 float64) float64 {
	ang := math.Atan2(py, px)
	if ang < 0 {
		ang += 2 * math.Pi
	}
	if ang >= a0 && ang <= a1 {
		return math.Abs(math.Hypot(px, py) - r)
	}
	d0 := math.Hypot(px-r*math.Cos(a0), py-r*math.Sin(a0))
	d1 := math.Hypot(px-r*math.Cos(a1), py-r*math.Sin(a1))
	return math.Min(d0, d1)
}

func smoothstep(edge0, edge1, x float64) float64 {
	t := clamp01((x - edge0) / (edge1 - edge0))
	return t * t * (3.0 - 2.0*t)
}

func lerp(a, b, t float64) float64 {
	return a + (b-a)*t
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func clampByte(v float64) byte {
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return byte(math.Round(v))
}
