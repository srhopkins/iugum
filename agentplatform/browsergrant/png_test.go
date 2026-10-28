package browsergrant

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/png"
	"testing"
)

func TestStrictPNGBoundaryRejectsTrailingUnknownAndCRC(t *testing.T) {
	var b bytes.Buffer
	png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, 2, 2)))
	valid := b.Bytes()
	if e := strictPNG(valid); e != nil {
		t.Fatal(e)
	}
	crc := append([]byte{}, valid...)
	crc[len(crc)-1] ^= 1
	chunk := make([]byte, 13)
	binary.BigEndian.PutUint32(chunk[:4], 1)
	copy(chunk[4:8], "tEXt")
	chunk[8] = 'x'
	binary.BigEndian.PutUint32(chunk[9:], crc32.ChecksumIEEE(chunk[4:9]))
	unknown := append(append(append([]byte{}, valid[:len(valid)-12]...), chunk...), valid[len(valid)-12:]...)
	for _, v := range [][]byte{append(append([]byte{}, valid...), []byte("<script>")...), crc, unknown} {
		if e := strictPNG(v); e == nil {
			t.Fatal("unsafe PNG accepted")
		}
	}
}
