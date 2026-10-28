package browsergrant

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
)

const MaxRasterBytes = 8 * 1024 * 1024
const MaxRasterPixels = 16 * 1024 * 1024

// strictPNG matches the companion chunk boundary; no active/unknown payloads,
// unverified CRC, duplicate structural chunks or bytes after IEND are accepted.
func strictPNG(b []byte) error {
	if len(b) > MaxRasterBytes || len(b) < 8 || !bytes.Equal(b[:8], []byte{137, 80, 78, 71, 13, 10, 26, 10}) {
		return errors.New("PNG signature/size")
	}
	offset, count := 8, 0
	ihdr, idat, end, closedIDAT := false, false, false, false
	plte, trns := false, false
	for offset < len(b) {
		count++
		if count > 4096 || len(b)-offset < 12 {
			return errors.New("PNG chunk boundary")
		}
		n := uint64(binary.BigEndian.Uint32(b[offset : offset+4]))
		if n > uint64(len(b)-offset-12) {
			return errors.New("PNG chunk length")
		}
		size := int(n)
		kind := string(b[offset+4 : offset+8])
		body := b[offset+4 : offset+8+size]
		expected := binary.BigEndian.Uint32(b[offset+8+size : offset+12+size])
		if crc32.ChecksumIEEE(body) != expected {
			return errors.New("PNG CRC")
		}
		if count == 1 && kind != "IHDR" {
			return errors.New("PNG missing first IHDR")
		}
		switch kind {
		case "IHDR":
			if ihdr || count != 1 || size != 13 {
				return errors.New("PNG IHDR")
			}
			ihdr = true
		case "PLTE":
			if plte || idat || size == 0 || size%3 != 0 || size > 768 {
				return errors.New("PNG PLTE")
			}
			plte = true
		case "tRNS":
			if trns || idat {
				return errors.New("PNG tRNS")
			}
			trns = true
		case "IDAT":
			if !ihdr || closedIDAT {
				return errors.New("PNG IDAT order")
			}
			idat = true
		case "IEND":
			if !idat || end || size != 0 {
				return errors.New("PNG IEND")
			}
			end = true
		default:
			return errors.New("unsupported PNG chunk")
		}
		if idat && kind != "IDAT" {
			closedIDAT = true
		}
		offset += 12 + size
		if end {
			if offset != len(b) {
				return errors.New("trailing PNG payload")
			}
			return nil
		}
	}
	return errors.New("PNG missing IEND")
}
