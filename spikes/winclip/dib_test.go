package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/png"
	"testing"
)

const (
	dibRedMaskOffset = 40
	dibRowBytes      = 4 * 4
)

func syntheticNRGBA() *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, 4, 3))
	alphas := []uint8{0, 64, 128, 255}
	for y := range 3 {
		for x := range 4 {
			img.SetNRGBA(x, y, color.NRGBA{
				R: uint8(x * 80),
				G: uint8(y * 100),
				B: uint8(x*y*30 + 7),
				A: alphas[(x+y)%len(alphas)],
			})
		}
	}
	return img
}

func assertSameNRGBA(t *testing.T, got, want *image.NRGBA) {
	t.Helper()
	if got.Rect.Size() != want.Rect.Size() {
		t.Fatalf("size mismatch: got %v want %v", got.Rect.Size(), want.Rect.Size())
	}
	if bytes.Equal(got.Pix, want.Pix) {
		return
	}
	for i := 0; i < len(want.Pix); i += 4 {
		if !bytes.Equal(got.Pix[i:i+4], want.Pix[i:i+4]) {
			t.Fatalf("pixel %d differs: got RGBA % X want RGBA % X", i/4, got.Pix[i:i+4], want.Pix[i:i+4])
		}
	}
}

func TestDIB_PNGRoundTrip32bpp(t *testing.T) {
	src := syntheticNRGBA()
	pngIn, err := encodePNG(src)
	if err != nil {
		t.Fatalf("encodePNG: %v", err)
	}
	dib, err := pngToDIBV5(pngIn)
	if err != nil {
		t.Fatalf("pngToDIBV5: %v", err)
	}
	if size := binary.LittleEndian.Uint32(dib[0:4]); size != bitmapV5HeaderSize {
		t.Fatalf("bV5Size = %d, want %d", size, bitmapV5HeaderSize)
	}
	if bitCount := binary.LittleEndian.Uint16(dib[14:16]); bitCount != dibBitCount {
		t.Fatalf("bV5BitCount = %d, want %d", bitCount, dibBitCount)
	}
	if compression := binary.LittleEndian.Uint32(dib[16:20]); compression != dibCompressionBits {
		t.Fatalf("bV5Compression = %d, want BI_BITFIELDS (%d)", compression, dibCompressionBits)
	}
	if alphaMask := binary.LittleEndian.Uint32(dib[dibRedMaskOffset+12 : dibRedMaskOffset+16]); alphaMask != 0xFF000000 {
		t.Fatalf("bV5AlphaMask = 0x%08X, want 0xFF000000", alphaMask)
	}

	pngOut, err := dibV5ToPNG(dib)
	if err != nil {
		t.Fatalf("dibV5ToPNG: %v", err)
	}
	decoded, err := png.Decode(bytes.NewReader(pngOut))
	if err != nil {
		t.Fatalf("decode round-tripped PNG: %v", err)
	}
	assertSameNRGBA(t, toNRGBA(decoded), src)
}

func TestDIB_BottomUpAndTopDown(t *testing.T) {
	src := syntheticNRGBA()
	bottomUp, err := encodeDIBV5(src, false)
	if err != nil {
		t.Fatalf("encode bottom-up: %v", err)
	}
	topDown, err := encodeDIBV5(src, true)
	if err != nil {
		t.Fatalf("encode top-down: %v", err)
	}

	if height := int32(binary.LittleEndian.Uint32(bottomUp[8:12])); height != 3 {
		t.Fatalf("bottom-up bV5Height = %d, want 3", height)
	}
	if height := int32(binary.LittleEndian.Uint32(topDown[8:12])); height != -3 {
		t.Fatalf("top-down bV5Height = %d, want -3", height)
	}

	firstStoredBottomUp := bottomUp[bitmapV5HeaderSize : bitmapV5HeaderSize+dibRowBytes]
	lastStoredTopDown := topDown[bitmapV5HeaderSize+2*dibRowBytes : bitmapV5HeaderSize+3*dibRowBytes]
	if !bytes.Equal(firstStoredBottomUp, lastStoredTopDown) {
		t.Fatal("bottom-up payload does not store the last image row first")
	}

	gotBottomUp, err := decodeDIBV5(bottomUp)
	if err != nil {
		t.Fatalf("decode bottom-up: %v", err)
	}
	assertSameNRGBA(t, gotBottomUp, src)
	gotTopDown, err := decodeDIBV5(topDown)
	if err != nil {
		t.Fatalf("decode top-down: %v", err)
	}
	assertSameNRGBA(t, gotTopDown, src)
}

func TestDIB_HeaderSizeIs124(t *testing.T) {
	if size := binary.Size(bitmapV5Header{}); size != bitmapV5HeaderSize {
		t.Fatalf("binary size of bitmapV5Header = %d, want %d", size, bitmapV5HeaderSize)
	}
}

func TestDIB_RejectsMalformedPayloads(t *testing.T) {
	dib, err := encodeDIBV5(syntheticNRGBA(), false)
	if err != nil {
		t.Fatalf("encodeDIBV5: %v", err)
	}

	if _, err := decodeDIBV5(dib[:len(dib)-1]); !errors.Is(err, errDIBLayout) {
		t.Fatalf("truncated pixels error = %v, want %v", err, errDIBLayout)
	}
	if _, err := decodeDIBV5(dib[:bitmapV5HeaderSize-1]); !errors.Is(err, errDIBHeader) {
		t.Fatalf("truncated header error = %v, want %v", err, errDIBHeader)
	}

	nonContiguous := bytes.Clone(dib)
	binary.LittleEndian.PutUint32(nonContiguous[dibRedMaskOffset:], 0x00FF00FF)
	if _, err := decodeDIBV5(nonContiguous); !errors.Is(err, errDIBChannelMask) {
		t.Fatalf("non-contiguous mask error = %v, want %v", err, errDIBChannelMask)
	}

	wrongSize := bytes.Clone(dib)
	binary.LittleEndian.PutUint32(wrongSize[0:4], 108)
	if _, err := decodeDIBV5(wrongSize); !errors.Is(err, errDIBHeader) {
		t.Fatalf("wrong bV5Size error = %v, want %v", err, errDIBHeader)
	}
}

func TestDIB_BIRGBOpaqueWithoutAlpha(t *testing.T) {
	src := syntheticNRGBA()
	dib, err := encodeDIBV5(src, false)
	if err != nil {
		t.Fatalf("encodeDIBV5: %v", err)
	}
	binary.LittleEndian.PutUint32(dib[16:20], dibCompressionRGB)
	got, err := decodeDIBV5(dib)
	if err != nil {
		t.Fatalf("decode BI_RGB: %v", err)
	}
	for i := 3; i < len(got.Pix); i += 4 {
		if got.Pix[i] != 0xFF {
			t.Fatalf("pixel %d alpha = %d, want opaque 255 for BI_RGB", i/4, got.Pix[i])
		}
	}
	if got.Rect.Size() != src.Rect.Size() {
		t.Fatalf("size mismatch: got %v want %v", got.Rect.Size(), src.Rect.Size())
	}
}
