package silk

// C: silk/typedef.h

const (
	int64_MAX = int64(0x7FFFFFFFFFFFFFFF)  //  2^63 - 1
	int64_MIN = int64(-0x8000000000000000) // -2^63
	int32_MAX = int32(0x7FFFFFFF)          //  2^31 - 1 =  2147483647
	int32_MIN = int32(-0x80000000)         // -2^31     = -2147483648
	int16_MAX = int16(0x7FFF)              //  2^15 - 1 =  32767
	int16_MIN = int16(-0x8000)             // -2^15     = -32768
	int8_MAX  = int8(0x7F)                 //  2^7 - 1  =  127
	int8_MIN  = int8(-0x80)                // -2^7      = -128
	uint8_MAX = uint8(0xFF)                //  2^8 - 1 = 255

	TRUE  = 1
	FALSE = 0
)

// enableAssertions mirrors ENABLE_ASSERTIONS.
const enableAssertions = false

// C: silk_assert(COND)
func silk_assert(cond bool) {
	if enableAssertions && !cond {
		panic("silk: assertion failed")
	}
}

type integer interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 | ~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64
}

// b2i converts a C boolean expression to int.
func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// Assert is silk_assert for other packages.
func Assert(cond bool) { silk_assert(cond) }

// Int32MAX returns silk_int32_MAX for other packages.
func Int32MAX() int32 { return int32_MAX }
