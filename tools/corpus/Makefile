# Builds the upstream libopus reference (fixed-point, static) out of tree and
# the `gen` oracle driver linked against it.  Nothing here touches the host's
# system libopus.
ROOT      := $(abspath $(CURDIR)/../..)
UPSTREAM  := $(ROOT)/third_party/opus
BUILD     := $(ROOT)/build
REF       := $(BUILD)/opus-fixed
REF_FLOAT := $(BUILD)/opus-float
CFLAGS_REF := -O2 -g -ffp-contract=off

.PHONY: all gen ref ref-float clean

all: gen

$(UPSTREAM)/configure:
	cd $(UPSTREAM) && ./autogen.sh

$(REF)/.libs/libopus.a: $(UPSTREAM)/configure
	mkdir -p $(REF)
	cd $(REF) && $(UPSTREAM)/configure --enable-fixed-point --disable-shared --enable-static --disable-doc --disable-extra-programs CFLAGS="$(CFLAGS_REF)" >configure.out 2>&1
	$(MAKE) -C $(REF) >$(REF)/make.out 2>&1

$(REF_FLOAT)/.libs/libopus.a: $(UPSTREAM)/configure
	mkdir -p $(REF_FLOAT)
	cd $(REF_FLOAT) && $(UPSTREAM)/configure --disable-shared --enable-static --disable-doc --disable-extra-programs CFLAGS="$(CFLAGS_REF)" >configure.out 2>&1
	$(MAKE) -C $(REF_FLOAT) >$(REF_FLOAT)/make.out 2>&1

ref: $(REF)/.libs/libopus.a
ref-float: $(REF_FLOAT)/.libs/libopus.a

$(BUILD)/gen-fixed: cgen/gen.c $(REF)/.libs/libopus.a
	$(CC) -O2 -Wall -Wextra -o $@ -I$(UPSTREAM)/include -I$(UPSTREAM)/src -I$(UPSTREAM)/celt -I$(REF) cgen/gen.c $(REF)/.libs/libopus.a -lm

$(BUILD)/gen-float: cgen/gen.c $(REF_FLOAT)/.libs/libopus.a
	$(CC) -O2 -Wall -Wextra -o $@ -I$(UPSTREAM)/include -I$(UPSTREAM)/src -I$(UPSTREAM)/celt -I$(REF_FLOAT) cgen/gen.c $(REF_FLOAT)/.libs/libopus.a -lm

gen: $(BUILD)/gen-fixed $(BUILD)/gen-float

clean:
	rm -rf $(BUILD)/gen-fixed $(BUILD)/gen-float

# Reference decoded output of the RFC 6716 test vectors (39 MB, not committed).
.PHONY: vectors
vectors:
	mkdir -p $(BUILD)/dl
	[ -s $(BUILD)/dl/opus_testvectors.tar.gz ] || curl -sSL -o $(BUILD)/dl/opus_testvectors.tar.gz https://opus-codec.org/docs/opus_testvectors.tar.gz
	cd $(BUILD)/dl && tar xzf opus_testvectors.tar.gz
