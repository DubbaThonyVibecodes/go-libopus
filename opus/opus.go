package opus

import "github.com/DubbaThonyVibecodes/go-libopus/celt"

// C: src/opus.c

// C: int encode_size(int size, unsigned char *data)
func encode_size(size int, data []byte) int {
	if size < 252 {
		data[0] = byte(size)
		return 1
	}
	data[0] = byte(252 + (size & 0x3))
	data[1] = byte((size - int(data[0])) >> 2)
	return 2
}

// C: static inline int align(int i)   (opus_private.h)
func align(i int) int {
	const ptr = 8
	return (i + ptr - 1) & -ptr
}

// IMIN/IMAX etc. used by the C src/ files come from arch.h via celt.
func IMIN[T celtInteger](a, b T) T { return celt.IMIN(a, b) }
func IMAX[T celtInteger](a, b T) T { return celt.IMAX(a, b) }

type celtInteger interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 | ~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64
}

// C: const char *opus_strerror(int error)
func Strerror(error int) string { return celt.Strerror(error) }

// C: const char *opus_get_version_string(void)
func GetVersionString() string { return celt.GetVersionString() }

// b2i converts a C boolean expression to int.
func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}
