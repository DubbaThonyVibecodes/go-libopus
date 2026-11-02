package celt

// C: celt/cpu_support.h — no run-time CPU detection is compiled in
// (OPUS_HAVE_RTCD is not defined), so OPUS_ARCHMASK is 0.

// C: static OPUS_INLINE int opus_select_arch(void)
func SelectArch() int {
	return 0
}
