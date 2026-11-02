# Builds the upstream libopus variants the benchmark compares against, from
# the submodule's current checkout, plus the cbench driver for each.  Builds
# go to build/bench/<upstream version>/ so switching release branches does not
# clobber them.  Nothing here touches the host's system libopus.
#
#   fixed        --enable-fixed-point, no SIMD/asm   (exactly what the Go port is)
#   fixed-simd   --enable-fixed-point, upstream default SIMD/asm/RTCD
#   float        float build, no SIMD/asm
#   float-simd   upstream defaults (float + SIMD): what a distro libopus is
ROOT     := $(abspath $(CURDIR)/../..)
UPSTREAM := $(ROOT)/third_party/opus
VER      := $(shell git -C $(UPSTREAM) describe --tags --always)
OUT      := $(ROOT)/build/bench/$(VER)
CFLAGS_REF := -O2
NOSIMD   := --disable-intrinsics --disable-asm --disable-rtcd
COMMON   := --disable-shared --enable-static --disable-doc --disable-extra-programs

VARIANTS := fixed fixed-simd float float-simd
CONF_fixed      := --enable-fixed-point $(NOSIMD)
CONF_fixed-simd := --enable-fixed-point
CONF_float      := $(NOSIMD)
CONF_float-simd :=

.PHONY: all clean
all: $(foreach v,$(VARIANTS),$(OUT)/cbench-$(v))

# configure must belong to the checked-out version: regenerate it when the
# submodule moved (autogen of 1.5+ also downloads the DNN model data).
$(UPSTREAM)/.configure-$(VER):
	cd $(UPSTREAM) && rm -f .configure-* && ./autogen.sh >/dev/null 2>&1 && touch .configure-$(VER)

$(OUT)/%/.libs/libopus.a: $(UPSTREAM)/.configure-$(VER)
	mkdir -p $(OUT)/$*
	cd $(OUT)/$* && $(UPSTREAM)/configure $(CONF_$*) $(COMMON) CFLAGS="$(CFLAGS_REF)" >configure.out 2>&1
	$(MAKE) -C $(OUT)/$* >$(OUT)/$*/make.out 2>&1

$(OUT)/cbench-%: cbench/cbench.c $(OUT)/%/.libs/libopus.a
	$(CC) -O2 -o $@ -I$(UPSTREAM)/include cbench/cbench.c $(OUT)/$*/.libs/libopus.a -lm

clean:
	rm -rf $(OUT)
