package opus

import "github.com/DubbaThonyVibecodes/go-libopus/celt"

// C: src/extensions.c, opus_extension_data / OpusExtensionIterator from src/opus_private.h

// OpusExtensionData is the C struct opus_extension_data.
type OpusExtensionData struct {
	Id    int
	Frame int
	Data  []byte
	Len   int32
}

// OpusExtensionIterator is the C struct OpusExtensionIterator. The C
// pointers into the padding buffer (curr_data, repeat_data, last_long,
// src_data) are kept as byte offsets into data; -1 stands for NULL.
type OpusExtensionIterator struct {
	data               []byte
	curr_data          int32
	repeat_data        int32
	last_long          int32
	src_data           int32
	len                int32
	curr_len           int32
	repeat_len         int32
	src_len            int32
	trailing_short_len int32
	nb_frames          int
	frame_max          int
	curr_frame         int
	repeat_frame       int
	repeat_l           byte
}

// Given an extension payload (i.e., excluding the initial ID byte), advance
// data to the next extension and return the length of the remaining
// extensions.
// N.B., a "Repeat These Extensions" extension (ID==2) does not advance past
// the repeated extension payloads.
// That requires higher-level logic.
//
// base is the buffer the C pointers index; *pdata is the offset of the C
// *pdata pointer in it.
//
// C: static opus_int32 skip_extension_payload(const unsigned char **pdata, opus_int32 len, opus_int32 *pheader_size, int id_byte, opus_int32 trailing_short_len)
func skip_extension_payload(base []byte, pdata *int32,
	length int32, pheader_size *int32, id_byte int,
	trailing_short_len int32) int32 {
	var data int32
	var header_size int32
	var id, L int
	data = *pdata
	header_size = 0
	id = id_byte >> 1
	L = id_byte & 1
	if (id == 0 && L == 1) || id == 2 {
		// Nothing to do.
	} else if id > 0 && id < 32 {
		if length < int32(L) {
			return -1
		}
		data += int32(L)
		length -= int32(L)
	} else {
		if L == 0 {
			if length < trailing_short_len {
				return -1
			}
			data += length - trailing_short_len
			length = trailing_short_len
		} else {
			var bytes int32 = 0
			var lacing int32
			for {
				if length < 1 {
					return -1
				}
				lacing = int32(base[data])
				data++
				bytes += lacing
				header_size++
				length -= lacing + 1
				if lacing != 255 {
					break
				}
			}
			if length < 0 {
				return -1
			}
			data += bytes
		}
	}
	*pdata = data
	*pheader_size = header_size
	return length
}

// Given an extension, advance data to the next extension and return the
// length of the remaining extensions.
// N.B., a "Repeat These Extensions" extension (ID==2) only advances past the
// extension ID byte.
// Higher-level logic is required to skip the extension payloads that come
// after it.
//
// C: static opus_int32 skip_extension(const unsigned char **pdata, opus_int32 len, opus_int32 *pheader_size)
func skip_extension(base []byte, pdata *int32, length int32,
	pheader_size *int32) int32 {
	var data int32
	var id_byte int
	if length == 0 {
		*pheader_size = 0
		return 0
	}
	if length < 1 {
		return -1
	}
	data = *pdata
	id_byte = int(base[data])
	data++
	length--
	length = skip_extension_payload(base, &data, length, pheader_size, id_byte, 0)
	if length >= 0 {
		*pdata = data
		(*pheader_size)++
	}
	return length
}

// C: void opus_extension_iterator_init(OpusExtensionIterator *iter, const unsigned char *data, opus_int32 len, opus_int32 nb_frames)
func (iter *OpusExtensionIterator) ExtensionIteratorInit(data []byte, length int32, nb_frames int32) {
	celt.Assert(length >= 0)
	celt.Assert(data != nil || length == 0)
	celt.Assert(nb_frames >= 0 && nb_frames <= 48)
	iter.data = data
	iter.repeat_data, iter.curr_data = 0, 0
	iter.last_long, iter.src_data = -1, -1
	iter.curr_len, iter.len = length, length
	iter.repeat_len, iter.src_len = 0, 0
	iter.trailing_short_len = 0
	iter.frame_max, iter.nb_frames = int(nb_frames), int(nb_frames)
	iter.repeat_frame, iter.curr_frame = 0, 0
	iter.repeat_l = 0
}

// Reset the iterator so it can start iterating again from the first
// extension.
//
// C: void opus_extension_iterator_reset(OpusExtensionIterator *iter)
func (iter *OpusExtensionIterator) ExtensionIteratorReset() {
	iter.repeat_data, iter.curr_data = 0, 0
	iter.last_long = -1
	iter.curr_len = iter.len
	iter.repeat_frame, iter.curr_frame = 0, 0
	iter.trailing_short_len = 0
}

// Tell the iterator not to return any extensions for frames of index
// frame_max or larger.
// This can allow it to stop iterating early if these extensions are not
// needed.
//
// C: void opus_extension_iterator_set_frame_max(OpusExtensionIterator *iter, int frame_max)
func (iter *OpusExtensionIterator) ExtensionIteratorSetFrameMax(frame_max int) {
	iter.frame_max = frame_max
}

// Return the next repeated extension.
// The return value is non-zero if one is found, negative on error, or 0 if we
// have finished repeating extensions.
//
// C: static int opus_extension_iterator_next_repeat(OpusExtensionIterator *iter, opus_extension_data *ext)
func (iter *OpusExtensionIterator) extensionIteratorNextRepeat(ext *OpusExtensionData) int {
	var header_size int32
	celt.Assert(iter.repeat_frame > 0)
	for ; iter.repeat_frame < iter.nb_frames; iter.repeat_frame++ {
		for iter.src_len > 0 {
			var curr_data0 int32
			var repeat_id_byte int
			repeat_id_byte = int(iter.data[iter.src_data])
			iter.src_len = skip_extension(iter.data, &iter.src_data, iter.src_len,
				&header_size)
			// We skipped this extension earlier, so it should not fail now.
			celt.Assert(iter.src_len >= 0)
			// Don't repeat padding or frame separators with a 0 increment.
			if repeat_id_byte <= 3 {
				continue
			}
			// If the "Repeat These Extensions" extension had L == 0 and this
			// is the last repeated long extension, then force decoding the
			// payload with L = 0.
			if iter.repeat_l == 0 &&
				iter.repeat_frame+1 >= iter.nb_frames &&
				iter.src_data == iter.last_long {
				repeat_id_byte &= ^1
			}
			curr_data0 = iter.curr_data
			iter.curr_len = skip_extension_payload(iter.data, &iter.curr_data,
				iter.curr_len, &header_size, repeat_id_byte,
				iter.trailing_short_len)
			if iter.curr_len < 0 {
				return OPUS_INVALID_PACKET
			}
			celt.Assert(iter.curr_data == iter.len-iter.curr_len)
			// If we were asked to stop at frame_max, skip extensions for later
			// frames.
			if iter.repeat_frame >= iter.frame_max {
				continue
			}
			if ext != nil {
				ext.Id = repeat_id_byte >> 1
				ext.Frame = iter.repeat_frame
				ext.Data = iter.data[curr_data0+header_size : iter.curr_data]
				ext.Len = iter.curr_data - curr_data0 - header_size
			}
			return 1
		}
		// We finished repeating the extensions for this frame.
		iter.src_data = iter.repeat_data
		iter.src_len = iter.repeat_len
	}
	// We finished repeating extensions.
	iter.repeat_data = iter.curr_data
	iter.last_long = -1
	// If L == 0, advance the frame number to handle the case where we did
	// not consume all of the data with an L == 0 long extension.
	if iter.repeat_l == 0 {
		iter.curr_frame++
		// Ignore additional padding if this was already the last frame.
		if iter.curr_frame >= iter.nb_frames {
			iter.curr_len = 0
		}
	}
	iter.repeat_frame = 0
	return 0
}

// Return the next extension (excluding real padding, separators, and repeat
// indicators, but including the repeated extensions) in bitstream order.
// Due to the extension repetition mechanism, extensions are not necessarily
// returned in frame order.
//
// C: int opus_extension_iterator_next(OpusExtensionIterator *iter, opus_extension_data *ext)
func (iter *OpusExtensionIterator) ExtensionIteratorNext(ext *OpusExtensionData) int {
	var header_size int32
	if iter.curr_len < 0 {
		return OPUS_INVALID_PACKET
	}
	if iter.repeat_frame > 0 {
		var ret int
		// We are in the process of repeating some extensions.
		ret = iter.extensionIteratorNextRepeat(ext)
		if ret != 0 {
			return ret
		}
	}
	// Checking this here allows opus_extension_iterator_set_frame_max() to be
	// called at any point.
	if iter.curr_frame >= iter.frame_max {
		return 0
	}
	for iter.curr_len > 0 {
		var curr_data0 int32
		var id int
		var L int
		curr_data0 = iter.curr_data
		id = int(iter.data[curr_data0] >> 1)
		L = int(iter.data[curr_data0] & 1)
		iter.curr_len = skip_extension(iter.data, &iter.curr_data, iter.curr_len,
			&header_size)
		if iter.curr_len < 0 {
			return OPUS_INVALID_PACKET
		}
		celt.Assert(iter.curr_data == iter.len-iter.curr_len)
		if id == 1 {
			if L == 0 {
				iter.curr_frame++
			} else {
				// A frame increment of 0 is a no-op.
				if iter.data[curr_data0+1] == 0 {
					continue
				}
				iter.curr_frame += int(iter.data[curr_data0+1])
			}
			if iter.curr_frame >= iter.nb_frames {
				iter.curr_len = -1
				return OPUS_INVALID_PACKET
			}
			// If we were asked to stop at frame_max, skip extensions for later
			// frames.
			if iter.curr_frame >= iter.frame_max {
				iter.curr_len = 0
			}
			iter.repeat_data = iter.curr_data
			iter.last_long = -1
			iter.trailing_short_len = 0
		} else if id == 2 {
			var ret int
			iter.repeat_l = byte(L)
			iter.repeat_frame = iter.curr_frame + 1
			iter.repeat_len = curr_data0 - iter.repeat_data
			iter.src_data = iter.repeat_data
			iter.src_len = iter.repeat_len
			ret = iter.extensionIteratorNextRepeat(ext)
			if ret != 0 {
				return ret
			}
		} else if id > 2 {
			// Update the location of the last long extension.
			// This lets us know when we need to modify the last L flag if we
			// repeat these extensions with L=0.
			if id >= 32 {
				iter.last_long = iter.curr_data
				iter.trailing_short_len = 0
			} else {
				// Otherwise, keep track of how many payload bytes follow the last
				// long extension.
				iter.trailing_short_len += int32(L)
			}
			if ext != nil {
				ext.Id = id
				ext.Frame = iter.curr_frame
				ext.Data = iter.data[curr_data0+header_size : iter.curr_data]
				ext.Len = iter.curr_data - curr_data0 - header_size
			}
			return 1
		}
	}
	return 0
}

// C: int opus_extension_iterator_find(OpusExtensionIterator *iter, opus_extension_data *ext, int id)
func (iter *OpusExtensionIterator) ExtensionIteratorFind(ext *OpusExtensionData, id int) int {
	var curr_ext OpusExtensionData
	var ret int
	for {
		ret = iter.ExtensionIteratorNext(&curr_ext)
		if ret <= 0 {
			return ret
		}
		if curr_ext.Id == id {
			*ext = curr_ext
			return ret
		}
	}
}

// PacketExtensionsCount counts the number of extensions, excluding real
// padding, separators, and repeat indicators, but including the repeated
// extensions.
//
// C: opus_int32 opus_packet_extensions_count(const unsigned char *data, opus_int32 len, int nb_frames)
func PacketExtensionsCount(data []byte, length int32, nb_frames int) int32 {
	var iter OpusExtensionIterator
	var count int
	iter.ExtensionIteratorInit(data, length, int32(nb_frames))
	for count = 0; iter.ExtensionIteratorNext(nil) > 0; count++ {
	}
	return int32(count)
}

// PacketExtensionsCountExt counts the number of extensions for each frame,
// excluding real padding and separators and repeat indicators, but
// including the repeated extensions.
//
// C: opus_int32 opus_packet_extensions_count_ext(const unsigned char *data, opus_int32 len, opus_int32 *nb_frame_exts, int nb_frames)
func PacketExtensionsCountExt(data []byte, length int32, nb_frame_exts []int32, nb_frames int) int32 {
	var iter OpusExtensionIterator
	var ext OpusExtensionData
	var count int
	iter.ExtensionIteratorInit(data, length, int32(nb_frames))
	clear(nb_frame_exts[:nb_frames])
	for count = 0; iter.ExtensionIteratorNext(&ext) > 0; count++ {
		nb_frame_exts[ext.Frame]++
	}
	return int32(count)
}

// PacketExtensionsParse extracts extensions from Opus padding (excluding
// real padding, separators, and repeat indicators, but including the
// repeated extensions) in bitstream order. Due to the extension repetition
// mechanism, extensions are not necessarily returned in frame order. On
// entry *nb_extensions is the capacity of extensions; on return it is the
// number of extensions found. Each extension's Data is the slice of data
// holding its payload (Len bytes).
//
// C: opus_int32 opus_packet_extensions_parse(const unsigned char *data, opus_int32 len, opus_extension_data *extensions, opus_int32 *nb_extensions, int nb_frames)
func PacketExtensionsParse(data []byte, length int32, extensions []OpusExtensionData, nb_extensions *int32, nb_frames int) int32 {
	var iter OpusExtensionIterator
	var count int
	var ret int
	celt.Assert(nb_extensions != nil)
	celt.Assert(extensions != nil || *nb_extensions == 0)
	iter.ExtensionIteratorInit(data, length, int32(nb_frames))
	for count = 0; ; count++ {
		var ext OpusExtensionData
		ret = iter.ExtensionIteratorNext(&ext)
		if ret <= 0 {
			break
		}
		if int32(count) == *nb_extensions {
			return OPUS_BUFFER_TOO_SMALL
		}
		extensions[count] = ext
	}
	*nb_extensions = int32(count)
	return int32(ret)
}

// PacketExtensionsParseExt extracts extensions from Opus padding (excluding
// real padding, separators, and repeat indicators, but including the
// repeated extensions) in frame order. nb_frame_exts must be filled with
// the output of PacketExtensionsCountExt().
//
// C: opus_int32 opus_packet_extensions_parse_ext(const unsigned char *data, opus_int32 len, opus_extension_data *extensions, opus_int32 *nb_extensions, const opus_int32 *nb_frame_exts, int nb_frames)
func PacketExtensionsParseExt(data []byte, length int32, extensions []OpusExtensionData, nb_extensions *int32, nb_frame_exts []int32, nb_frames int) int32 {
	var iter OpusExtensionIterator
	var ext OpusExtensionData
	var nb_frames_cum [49]int32
	var count int
	var prev_total int
	var ret int
	celt.Assert(nb_extensions != nil)
	celt.Assert(extensions != nil || *nb_extensions == 0)
	celt.Assert(nb_frames <= 48)
	// Convert the frame extension count array to a cumulative sum.
	prev_total = 0
	for count = 0; count < nb_frames; count++ {
		var total int
		total = int(nb_frame_exts[count]) + prev_total
		nb_frames_cum[count] = int32(prev_total)
		prev_total = total
	}
	nb_frames_cum[count] = int32(prev_total)
	iter.ExtensionIteratorInit(data, length, int32(nb_frames))
	for count = 0; ; count++ {
		var idx int32
		ret = iter.ExtensionIteratorNext(&ext)
		if ret <= 0 {
			break
		}
		idx = nb_frames_cum[ext.Frame]
		nb_frames_cum[ext.Frame]++
		if idx >= *nb_extensions {
			return OPUS_BUFFER_TOO_SMALL
		}
		celt.Assert(idx < nb_frames_cum[ext.Frame+1])
		extensions[idx] = ext
	}
	*nb_extensions = int32(count)
	return int32(ret)
}

// write_extension_payload returns the new position, or a negative error
// code.
//
// C: static int write_extension_payload(unsigned char *data, opus_int32 len, opus_int32 pos, const opus_extension_data *ext, int last)
func write_extension_payload(data []byte, length int32,
	pos int32, ext *OpusExtensionData, last int) int32 {
	celt.Assert(ext.Id >= 3 && ext.Id <= 127)
	if ext.Id < 32 {
		if ext.Len < 0 || ext.Len > 1 {
			return OPUS_BAD_ARG
		}
		if ext.Len > 0 {
			if length-pos < ext.Len {
				return OPUS_BUFFER_TOO_SMALL
			}
			if data != nil {
				data[pos] = ext.Data[0]
			}
			pos++
		}
	} else {
		var length_bytes int32
		if ext.Len < 0 {
			return OPUS_BAD_ARG
		}
		length_bytes = 1 + ext.Len/255
		if last != 0 {
			length_bytes = 0
		}
		if length-pos < length_bytes+ext.Len {
			return OPUS_BUFFER_TOO_SMALL
		}
		if last == 0 {
			var j int32
			for j = 0; j < ext.Len/255; j++ {
				if data != nil {
					data[pos] = 255
				}
				pos++
			}
			if data != nil {
				data[pos] = byte(ext.Len % 255)
			}
			pos++
		}
		if data != nil {
			copy(data[pos:pos+ext.Len], ext.Data[:ext.Len])
		}
		pos += ext.Len
	}
	return pos
}

// C: static int write_extension(unsigned char *data, opus_int32 len, opus_int32 pos, const opus_extension_data *ext, int last)
func write_extension(data []byte, length int32, pos int32,
	ext *OpusExtensionData, last int) int32 {
	if length-pos < 1 {
		return OPUS_BUFFER_TOO_SMALL
	}
	celt.Assert(ext.Id >= 3 && ext.Id <= 127)
	if data != nil {
		var l int32
		if ext.Id < 32 {
			l = ext.Len
		} else {
			l = int32(b2i(last == 0))
		}
		data[pos] = byte(int32(ext.Id<<1) + l)
	}
	pos++
	return write_extension_payload(data, length, pos, ext, last)
}

// PacketExtensionsGenerate writes the extensions into data (which may be
// nil to only compute the size) and returns the number of bytes used.
//
// C: opus_int32 opus_packet_extensions_generate(unsigned char *data, opus_int32 len, const opus_extension_data  *extensions, opus_int32 nb_extensions, int nb_frames, int pad)
func PacketExtensionsGenerate(data []byte, length int32,
	extensions []OpusExtensionData, nb_extensions int32,
	nb_frames int, pad int) int32 {
	var frame_min_idx [48]int32
	var frame_max_idx [48]int32
	var frame_repeat_idx [48]int32
	var i int32
	var f int
	var curr_frame int = 0
	var pos int32 = 0
	var written int32 = 0

	celt.Assert(length >= 0)
	if nb_frames > 48 {
		return OPUS_BAD_ARG
	}

	// Do a little work up-front to make this O(nb_extensions) instead of
	// O(nb_extensions*nb_frames) so long as the extensions are in frame
	// order (without requiring that they be in frame order).
	for f = 0; f < nb_frames; f++ {
		frame_min_idx[f] = nb_extensions
	}
	clear(frame_max_idx[:nb_frames])
	for i = 0; i < nb_extensions; i++ {
		f = extensions[i].Frame
		if f < 0 || f >= nb_frames {
			return OPUS_BAD_ARG
		}
		if extensions[i].Id < 3 || extensions[i].Id > 127 {
			return OPUS_BAD_ARG
		}
		frame_min_idx[f] = IMIN(frame_min_idx[f], i)
		frame_max_idx[f] = IMAX(frame_max_idx[f], i+1)
	}
	for f = 0; f < nb_frames; f++ {
		frame_repeat_idx[f] = frame_min_idx[f]
	}
	for f = 0; f < nb_frames; f++ {
		var last_long_idx int32
		var repeat_count int
		repeat_count = 0
		last_long_idx = -1
		if f+1 < nb_frames {
			for i = frame_min_idx[f]; i < frame_max_idx[f]; i++ {
				if extensions[i].Frame == f {
					var g int
					// Test if we can repeat this extension in future frames.
					for g = f + 1; g < nb_frames; g++ {
						if frame_repeat_idx[g] >= frame_max_idx[g] {
							break
						}
						celt.Assert(extensions[frame_repeat_idx[g]].Frame == g)
						if extensions[frame_repeat_idx[g]].Id != extensions[i].Id {
							break
						}
						if extensions[frame_repeat_idx[g]].Id < 32 &&
							extensions[frame_repeat_idx[g]].Len !=
								extensions[i].Len {
							break
						}
					}
					if g < nb_frames {
						break
					}
					// We can!
					// If this is a long extension, save the index of the last
					// instance, so we can modify its L flag.
					if extensions[i].Id >= 32 {
						last_long_idx = frame_repeat_idx[nb_frames-1]
					}
					// Using the repeat mechanism almost always makes the
					// encoding smaller (or at least no larger).
					// However, there's one case where that might not be true: if
					// the last repeated long extension in the last frame was
					// previously the last extension, but using the repeat
					// mechanism makes that no longer true (because there are other
					// non-repeated extensions in earlier frames that must now be
					// coded after it), and coding its length requires more bytes
					// than the repeat mechanism saves.
					// This can only be true if its length is at least 255 bytes
					// (although sometimes it requires even more).
					// Currently we do not check for that, and just always use the
					// repeat mechanism if we can.
					// See git history for code that does the check.
					// Advance the repeat pointers.
					for g = f + 1; g < nb_frames; g++ {
						var j int32
						for j = frame_repeat_idx[g] + 1; j < frame_max_idx[g] &&
							extensions[j].Frame != g; j++ {
						}
						frame_repeat_idx[g] = j
					}
					repeat_count++
					// Point the repeat pointer for this frame to the current
					// extension, so we know when to trigger the repeats.
					frame_repeat_idx[f] = i
				}
			}
		}
		for i = frame_min_idx[f]; i < frame_max_idx[f]; i++ {
			if extensions[i].Frame == f {
				// Insert separator when needed.
				if f != curr_frame {
					diff := f - curr_frame
					if length-pos < 2 {
						return OPUS_BUFFER_TOO_SMALL
					}
					if diff == 1 {
						if data != nil {
							data[pos] = 0x02
						}
						pos++
					} else {
						if data != nil {
							data[pos] = 0x03
						}
						pos++
						if data != nil {
							data[pos] = byte(diff)
						}
						pos++
					}
					curr_frame = f
				}

				pos = write_extension(data, length, pos, &extensions[i],
					b2i(written == nb_extensions-1))
				if pos < 0 {
					return pos
				}
				written++

				if repeat_count > 0 && frame_repeat_idx[f] == i {
					var nb_repeated int
					var last int
					var g int
					// Add the repeat indicator.
					nb_repeated = repeat_count * (nb_frames - (f + 1))
					last = b2i(int(written)+nb_repeated == int(nb_extensions) ||
						(last_long_idx < 0 && i+1 >= frame_max_idx[f]))
					if length-pos < 1 {
						return OPUS_BUFFER_TOO_SMALL
					}
					if data != nil {
						data[pos] = byte(0x04 + b2i(last == 0))
					}
					pos++
					for g = f + 1; g < nb_frames; g++ {
						var j int32
						for j = frame_min_idx[g]; j < frame_repeat_idx[g]; j++ {
							if extensions[j].Frame == g {
								pos = write_extension_payload(data, length, pos,
									&extensions[j], b2i(last != 0 && j == last_long_idx))
								if pos < 0 {
									return pos
								}
								written++
							}
						}
						frame_min_idx[g] = j
					}
					if last != 0 {
						curr_frame++
					}
				}
			}
		}
	}
	celt.Assert(written == nb_extensions)
	// If we need to pad, just prepend 0x01 bytes. Even better would be to fill the
	// end with zeros, but that requires checking that turning the last extension into
	// an L=1 case still fits.
	if pad != 0 && pos < length {
		padding := length - pos
		if data != nil {
			copy(data[padding:padding+pos], data[:pos])
			for i = 0; i < padding; i++ {
				data[i] = 0x01
			}
		}
		pos += padding
	}
	return pos
}
