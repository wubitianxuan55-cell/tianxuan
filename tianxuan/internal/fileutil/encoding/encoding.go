// Package encoding detects and converts file encodings for the built-in
// file tools. The detection cascade (BOM → strict UTF-8 → GB18030 → lossy
// UTF-8) mirrors v1's file-encoding.ts and keeps CJK Windows files editable
// without silently mangling their bytes.
package encoding

import (
	"bytes"
	"encoding/binary"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

type Kind int

const (
	UTF8 Kind = iota
	UTF8BOM
	UTF16LE
	UTF16BE
	GB18030
	LossyUTF8
	UTF16LENoBOM
	UTF16BENoBOM
)

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

func Detect(data []byte) (Kind, []byte) {
	switch {
	case len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF:
		return UTF8BOM, data
	case len(data) >= 2 && data[0] == 0xFF && data[1] == 0xFE:
		return UTF16LE, data
	case len(data) >= 2 && data[0] == 0xFE && data[1] == 0xFF:
		return UTF16BE, data
	}
	if k, ok := DetectUTF16NoBOM(data); ok {
		return k, data
	}
	if utf8.Valid(data) {
		return UTF8, data
	}
	dec := simplifiedchinese.GB18030.NewDecoder()
	if _, _, err := transform.Bytes(dec, data); err == nil {
		return GB18030, data
	}
	return LossyUTF8, data
}

func DetectQuick(peek []byte) Kind {
	switch {
	case len(peek) >= 3 && peek[0] == 0xEF && peek[1] == 0xBB && peek[2] == 0xBF:
		return UTF8BOM
	case len(peek) >= 2 && peek[0] == 0xFF && peek[1] == 0xFE:
		return UTF16LE
	case len(peek) >= 2 && peek[0] == 0xFE && peek[1] == 0xFF:
		return UTF16BE
	}
	return UTF8
}

func DetectUTF16NoBOM(b []byte) (Kind, bool) {
	n := len(b)
	if n < 16 {
		return UTF8, false
	}
	n &^= 1
	var evenNUL, oddNUL int
	for i := 0; i < n; i++ {
		if b[i] != 0 {
			continue
		}
		if i%2 == 0 {
			evenNUL++
		} else {
			oddNUL++
		}
	}
	half := n / 2
	switch {
	case oddNUL*10 >= half*3 && evenNUL*20 <= half:
		return UTF16LENoBOM, true
	case evenNUL*10 >= half*3 && oddNUL*20 <= half:
		return UTF16BENoBOM, true
	}
	return UTF8, false
}

func Decode(data []byte, enc Kind) []byte {
	switch enc {
	case UTF8BOM:
		return data[3:]
	case UTF16LE:
		return decodeUTF16(data[2:], binary.LittleEndian)
	case UTF16BE:
		return decodeUTF16(data[2:], binary.BigEndian)
	case UTF16LENoBOM:
		return decodeUTF16(data, binary.LittleEndian)
	case UTF16BENoBOM:
		return decodeUTF16(data, binary.BigEndian)
	case GB18030:
		out, _, err := transform.Bytes(simplifiedchinese.GB18030.NewDecoder(), data)
		if err != nil {
			return data
		}
		return out
	}
	return data
}

func Decoder(enc Kind) transform.Transformer {
	switch enc {
	case UTF8BOM:
		return nil
	case GB18030:
		return simplifiedchinese.GB18030.NewDecoder()
	}
	return nil
}

func Encode(text string, enc Kind) []byte {
	switch enc {
	case UTF8BOM:
		return append(utf8BOM, []byte(text)...)
	case UTF16LE:
		return encodeUTF16(text, binary.LittleEndian, true)
	case UTF16BE:
		return encodeUTF16(text, binary.BigEndian, true)
	case UTF16LENoBOM:
		return encodeUTF16(text, binary.LittleEndian, false)
	case UTF16BENoBOM:
		return encodeUTF16(text, binary.BigEndian, false)
	case GB18030:
		out, _, err := transform.Bytes(simplifiedchinese.GB18030.NewEncoder(), []byte(text))
		if err != nil {
			return []byte(text)
		}
		return out
	}
	return []byte(text)
}

func decodeUTF16(b []byte, order binary.ByteOrder) []byte {
	u := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		u = append(u, order.Uint16(b[i:i+2]))
	}
	return []byte(string(utf16Decode(u)))
}

func encodeUTF16(text string, order binary.ByteOrder, withBOM bool) []byte {
	runes := []rune(text)
	encoded := utf16Encode(runes)
	var buf bytes.Buffer
	if withBOM {
		var bom [2]byte
		if order == binary.LittleEndian {
			bom[0], bom[1] = 0xFF, 0xFE
		} else {
			bom[0], bom[1] = 0xFE, 0xFF
		}
		buf.Write(bom[:])
	}
	for _, u := range encoded {
		var b [2]byte
		order.PutUint16(b[:], u)
		buf.Write(b[:])
	}
	return buf.Bytes()
}

func utf16Decode(u []uint16) []rune {
	var out []rune
	for i := 0; i < len(u); i++ {
		c := u[i]
		if c >= 0xD800 && c <= 0xDBFF && i+1 < len(u) {
			c2 := u[i+1]
			if c2 >= 0xDC00 && c2 <= 0xDFFF {
				out = append(out, rune(c-0xD800)<<10|rune(c2-0xDC00)+0x10000)
				i++
				continue
			}
		}
		out = append(out, rune(c))
	}
	return out
}

func utf16Encode(runes []rune) []uint16 {
	var out []uint16
	for _, r := range runes {
		if r >= 0x10000 && r <= 0x10FFFF {
			r -= 0x10000
			out = append(out, uint16(0xD800+(r>>10)), uint16(0xDC00+(r&0x3FF)))
		} else {
			out = append(out, uint16(r))
		}
	}
	return out
}
