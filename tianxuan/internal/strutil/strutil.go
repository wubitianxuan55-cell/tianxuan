// Package strutil provides zero-allocation string utilities shared across
// tianxuan packages, avoiding fmt.Sprintf in hot paths.
package strutil

// Itoa converts an integer to its decimal string representation without
// allocating through fmt. Negative values and zero both return "0".
func Itoa(n int) string {
	if n <= 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// TrimSpace drops leading and trailing ASCII whitespace from a byte slice
// without allocating a new backing array (it returns a sub-slice of b).
func TrimSpace(b []byte) []byte {
	i, j := 0, len(b)
	for i < j && isSpace(b[i]) {
		i++
	}
	for j > i && isSpace(b[j-1]) {
		j--
	}
	return b[i:j]
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}
