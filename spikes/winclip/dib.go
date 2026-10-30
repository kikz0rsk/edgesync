package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"math/bits"
)

var (
	errDIBHeader      = errors.New("unsupported or malformed BITMAPV5HEADER")
	errDIBLayout      = errors.New("unsupported DIB layout")
	errDIBChannelMask = errors.New("channel mask is not a contiguous 8-bit field")
)

const (
	bitmapV5HeaderSize = 124
	dibCompressionRGB  = 0
	dibCompressionBits = 3
	lcsSRGB            = 0x73524742
	lcsGMImages        = 4
	dibBitCount        = 32
)

// bitmapV5Header mirrors the BITMAPV5HEADER member order from Microsoft Learn; its binary size is asserted to be 124
// bytes in the tests.
type bitmapV5Header struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
	RedMask       uint32
	GreenMask     uint32
	BlueMask      uint32
	AlphaMask     uint32
	CSType        uint32
	Endpoints     [9]uint32
	GammaRed      uint32
	GammaGreen    uint32
	GammaBlue     uint32
	Intent        uint32
	ProfileData   uint32
	ProfileSize   uint32
	Reserved      uint32
}

// encodeDIBV5 writes img as a 32 bpp BI_BITFIELDS CF_DIBV5 with BGRA byte order. Alpha is stored straight
// (non-premultiplied), the same representation as image.NRGBA and PNG; Microsoft Learn does not document the CF_DIBV5
// alpha convention, so this is the spike's assumption and is checked by pasting into Paint and Word.
func encodeDIBV5(img *image.NRGBA, topDown bool) ([]byte, error) {
	width := img.Rect.Dx()
	height := img.Rect.Dy()
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("%w: image is %dx%d", errDIBLayout, width, height)
	}
	stride := width * 4
	headerHeight := int32(height)
	if topDown {
		headerHeight = -headerHeight
	}
	header := bitmapV5Header{
		Size:        bitmapV5HeaderSize,
		Width:       int32(width),
		Height:      headerHeight,
		Planes:      1,
		BitCount:    dibBitCount,
		Compression: dibCompressionBits,
		SizeImage:   uint32(stride * height),
		RedMask:     0x00FF0000,
		GreenMask:   0x0000FF00,
		BlueMask:    0x000000FF,
		AlphaMask:   0xFF000000,
		CSType:      lcsSRGB,
		Intent:      lcsGMImages,
	}
	var headerBuf bytes.Buffer
	if err := binary.Write(&headerBuf, binary.LittleEndian, &header); err != nil {
		return nil, fmt.Errorf("encode BITMAPV5HEADER: %w", err)
	}

	pixels := make([]byte, stride*height)
	for row := range height {
		srcY := row
		if !topDown {
			srcY = height - 1 - row
		}
		src := img.PixOffset(img.Rect.Min.X, img.Rect.Min.Y+srcY)
		dst := pixels[row*stride : (row+1)*stride]
		for x := range width {
			p := img.Pix[src+4*x : src+4*x+4]
			d := dst[4*x : 4*x+4]
			d[0], d[1], d[2], d[3] = p[2], p[1], p[0], p[3]
		}
	}
	return append(headerBuf.Bytes(), pixels...), nil
}

// decodeDIBV5 reads a 32 bpp CF_DIBV5 with BI_RGB or BI_BITFIELDS and returns straight-alpha NRGBA pixels. A zero
// alpha mask means the alpha channel is unused and every pixel is opaque. Any other layout is an error.
func decodeDIBV5(dib []byte) (*image.NRGBA, error) {
	if len(dib) < bitmapV5HeaderSize {
		return nil, fmt.Errorf("%w: %d bytes is shorter than the header", errDIBHeader, len(dib))
	}
	var header bitmapV5Header
	if err := binary.Read(bytes.NewReader(dib[:bitmapV5HeaderSize]), binary.LittleEndian, &header); err != nil {
		return nil, fmt.Errorf("decode BITMAPV5HEADER: %w", err)
	}
	if header.Size != bitmapV5HeaderSize {
		return nil, fmt.Errorf("%w: bV5Size=%d", errDIBHeader, header.Size)
	}
	if header.Planes != 1 || header.BitCount != dibBitCount {
		return nil, fmt.Errorf("%w: planes=%d bitCount=%d", errDIBLayout, header.Planes, header.BitCount)
	}
	if header.Compression != dibCompressionRGB && header.Compression != dibCompressionBits {
		return nil, fmt.Errorf("%w: compression=%d", errDIBLayout, header.Compression)
	}
	if header.ClrUsed != 0 {
		return nil, fmt.Errorf("%w: colour table of %d entries", errDIBLayout, header.ClrUsed)
	}
	width := int(header.Width)
	height := int(header.Height)
	topDown := height < 0
	if topDown {
		height = -height
	}
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("%w: image is %dx%d", errDIBLayout, width, height)
	}
	stride := width * 4
	pixels := dib[bitmapV5HeaderSize:]
	if len(pixels) < stride*height {
		return nil, fmt.Errorf("%w: pixel data is %d bytes, need %d", errDIBLayout, len(pixels), stride*height)
	}

	redMask, greenMask, blueMask, alphaMask := header.RedMask, header.GreenMask, header.BlueMask, header.AlphaMask
	if header.Compression == dibCompressionRGB {
		redMask, greenMask, blueMask, alphaMask = 0x00FF0000, 0x0000FF00, 0x000000FF, 0
	}
	redShift, err := channelShift(redMask)
	if err != nil {
		return nil, fmt.Errorf("red: %w", err)
	}
	greenShift, err := channelShift(greenMask)
	if err != nil {
		return nil, fmt.Errorf("green: %w", err)
	}
	blueShift, err := channelShift(blueMask)
	if err != nil {
		return nil, fmt.Errorf("blue: %w", err)
	}
	opaque := alphaMask == 0
	var alphaShift uint
	if !opaque {
		if alphaShift, err = channelShift(alphaMask); err != nil {
			return nil, fmt.Errorf("alpha: %w", err)
		}
	}

	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		srcRow := y
		if !topDown {
			srcRow = height - 1 - y
		}
		row := pixels[srcRow*stride : (srcRow+1)*stride]
		for x := range width {
			px := binary.LittleEndian.Uint32(row[4*x:])
			o := img.PixOffset(x, y)
			img.Pix[o] = uint8(px >> redShift)
			img.Pix[o+1] = uint8(px >> greenShift)
			img.Pix[o+2] = uint8(px >> blueShift)
			if opaque {
				img.Pix[o+3] = 0xFF
			} else {
				img.Pix[o+3] = uint8(px >> alphaShift)
			}
		}
	}
	return img, nil
}

// channelShift returns the bit position of an 8-bit channel mask such as 0x0000FF00.
func channelShift(mask uint32) (uint, error) {
	shift := uint(bits.TrailingZeros32(mask))
	if mask == 0 || mask != 0xFF<<shift {
		return 0, fmt.Errorf("%w: 0x%08X", errDIBChannelMask, mask)
	}
	return shift, nil
}

// pngToDIBV5 converts PNG bytes to a bottom-up CF_DIBV5 payload.
func pngToDIBV5(pngData []byte) ([]byte, error) {
	decoded, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		return nil, fmt.Errorf("decode PNG: %w", err)
	}
	return encodeDIBV5(toNRGBA(decoded), false)
}

// dibV5ToPNG converts a CF_DIBV5 payload to PNG bytes.
func dibV5ToPNG(dib []byte) ([]byte, error) {
	img, err := decodeDIBV5(dib)
	if err != nil {
		return nil, err
	}
	return encodePNG(img)
}

func encodePNG(img *image.NRGBA) ([]byte, error) {
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return nil, fmt.Errorf("encode PNG: %w", err)
	}
	return out.Bytes(), nil
}

// toNRGBA returns img as straight-alpha NRGBA with its origin at (0,0). draw.Draw converts premultiplied sources to
// straight alpha through the NRGBA colour model.
func toNRGBA(img image.Image) *image.NRGBA {
	if n, ok := img.(*image.NRGBA); ok && n.Rect.Min == (image.Point{}) {
		return n
	}
	bounds := img.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(dst, dst.Bounds(), img, bounds.Min, draw.Src)
	return dst
}
