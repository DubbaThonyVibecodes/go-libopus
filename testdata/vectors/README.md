# Official decoder test vectors

`rfc6716/testvectorNN.bit` are the Opus decoder conformance bitstreams
published with RFC 6716 (https://opus-codec.org/docs/opus_testvectors.tar.gz),
in `opus_demo` packet format. The reference decoded output (`.dec` files,
39 MB) is not committed; `make -C tools/corpus vectors` downloads it into
`build/dl/` for the `opus_compare`-style quality test.
