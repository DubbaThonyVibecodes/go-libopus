package opus

import "github.com/DubbaThonyVibecodes/go-libopus/celt"

// C: src/repacketizer.c, struct OpusRepacketizer from src/opus_private.h

// OpusRepacketizer is the C struct OpusRepacketizer.
type OpusRepacketizer struct {
	toc         byte
	nb_frames   int
	frames      [48][]byte
	len         [48]int16
	framesize   int
	paddings    [48][]byte
	padding_len [48]int32
}

// C: int opus_repacketizer_get_size(void)
func RepacketizerGetSize() int {
	return 48*8 + 48*2 + 16 + 48*8 + 48*4 // sizeof(OpusRepacketizer) == 1072 on a 64-bit platform, for API parity
}

// C: OpusRepacketizer *opus_repacketizer_init(OpusRepacketizer *rp)
func (rp *OpusRepacketizer) RepacketizerInit() *OpusRepacketizer {
	rp.nb_frames = 0
	return rp
}

// C: OpusRepacketizer *opus_repacketizer_create(void)
func RepacketizerCreate() *OpusRepacketizer {
	rp := new(OpusRepacketizer)
	return rp.RepacketizerInit()
}

// C: void opus_repacketizer_destroy(OpusRepacketizer *rp)
func (rp *OpusRepacketizer) RepacketizerDestroy() {}

// C: static int opus_repacketizer_cat_impl(OpusRepacketizer *rp, const unsigned char *data, opus_int32 len, int self_delimited)
func (rp *OpusRepacketizer) repacketizerCatImpl(data []byte, length int32, self_delimited int) int {
	var tmp_toc byte
	var curr_nb_frames, ret int
	// Set of check ToC
	if length < 1 {
		return OPUS_INVALID_PACKET
	}
	if rp.nb_frames == 0 {
		rp.toc = data[0]
		rp.framesize = PacketGetSamplesPerFrame(data, 8000)
	} else if (rp.toc & 0xFC) != (data[0] & 0xFC) {
		// fprintf(stderr, "toc mismatch: 0x%x vs 0x%x\n", rp->toc, data[0]);
		return OPUS_INVALID_PACKET
	}
	curr_nb_frames = PacketGetNbFrames(data, length)
	if curr_nb_frames < 1 {
		return OPUS_INVALID_PACKET
	}

	// Check the 120 ms maximum packet size
	if (curr_nb_frames+rp.nb_frames)*rp.framesize > 960 {
		return OPUS_INVALID_PACKET
	}

	ret = opus_packet_parse_impl(data, length, self_delimited, &tmp_toc, rp.frames[rp.nb_frames:], rp.len[rp.nb_frames:],
		nil, nil, &rp.paddings[rp.nb_frames], &rp.padding_len[rp.nb_frames])
	if ret < 1 {
		return ret
	}

	// set padding length to zero for all but the first frame
	for curr_nb_frames > 1 {
		rp.nb_frames++
		rp.padding_len[rp.nb_frames] = 0
		rp.paddings[rp.nb_frames] = nil
		curr_nb_frames--
	}
	rp.nb_frames++
	return OPUS_OK
}

// C: int opus_repacketizer_cat(OpusRepacketizer *rp, const unsigned char *data, opus_int32 len)
func (rp *OpusRepacketizer) RepacketizerCat(data []byte, length int32) int {
	return rp.repacketizerCatImpl(data, length, 0)
}

// C: int opus_repacketizer_get_nb_frames(OpusRepacketizer *rp)
func (rp *OpusRepacketizer) RepacketizerGetNbFrames() int {
	return rp.nb_frames
}

// C: opus_int32 opus_repacketizer_out_range_impl(OpusRepacketizer *rp, int begin, int end,
//
//	unsigned char *data, opus_int32 maxlen, int self_delimited, int pad, const opus_extension_data *extensions, int nb_extensions)
//
// data may overlap the frames (in-place unpadding from PacketUnpad /
// MultistreamPacketUnpad); copy has memmove semantics like OPUS_MOVE.
func (rp *OpusRepacketizer) repacketizerOutRangeImpl(begin, end int, data []byte, maxlen int32, self_delimited int, pad int, extensions []OpusExtensionData, nb_extensions int) int32 {
	var i, count int
	var tot_size int32
	var length []int16
	var frames [][]byte
	var ptr int // index of the C `ptr` pointer in data
	var ones_begin, ones_end int = 0, 0
	var ext_begin, ext_len int = 0, 0
	var ext_count, total_ext_count int
	var all_extensions []OpusExtensionData

	if begin < 0 || begin >= end || end > rp.nb_frames {
		// fprintf(stderr, "%d %d %d\n", begin, end, rp->nb_frames);
		return OPUS_BAD_ARG
	}
	count = end - begin

	length = rp.len[begin:]
	frames = rp.frames[begin:]
	if self_delimited != 0 {
		tot_size = int32(1 + b2i(length[count-1] >= 252))
	} else {
		tot_size = 0
	}

	// figure out total number of extensions
	total_ext_count = nb_extensions
	for i = begin; i < end; i++ {
		n := int(PacketExtensionsCount(rp.paddings[i], rp.padding_len[i]))
		if n > 0 {
			total_ext_count += n
		}
	}
	if total_ext_count != 0 {
		all_extensions = make([]OpusExtensionData, total_ext_count)
	} else {
		all_extensions = make([]OpusExtensionData, ALLOC_NONE)
	}
	// copy over any extensions that were passed in
	for ext_count = 0; ext_count < nb_extensions; ext_count++ {
		all_extensions[ext_count] = extensions[ext_count]
	}

	// incorporate any extensions from the repacketizer padding
	for i = begin; i < end; i++ {
		var j int
		var frame_ext_count int32
		frame_ext_count = int32(total_ext_count - ext_count)
		ret := PacketExtensionsParse(rp.paddings[i], rp.padding_len[i],
			all_extensions[ext_count:], &frame_ext_count)
		if ret < 0 {
			return OPUS_INTERNAL_ERROR
		}
		// renumber the extension frame numbers
		for j = 0; j < int(frame_ext_count); j++ {
			all_extensions[ext_count+j].Frame += i - begin
		}
		ext_count += int(frame_ext_count)
	}

	ptr = 0
	if count == 1 {
		// Code 0
		tot_size += int32(length[0]) + 1
		if tot_size > maxlen {
			return OPUS_BUFFER_TOO_SMALL
		}
		data[ptr] = rp.toc & 0xFC
		ptr++
	} else if count == 2 {
		if length[1] == length[0] {
			// Code 1
			tot_size += 2*int32(length[0]) + 1
			if tot_size > maxlen {
				return OPUS_BUFFER_TOO_SMALL
			}
			data[ptr] = (rp.toc & 0xFC) | 0x1
			ptr++
		} else {
			// Code 2
			tot_size += int32(length[0]) + int32(length[1]) + 2 + int32(b2i(length[0] >= 252))
			if tot_size > maxlen {
				return OPUS_BUFFER_TOO_SMALL
			}
			data[ptr] = (rp.toc & 0xFC) | 0x2
			ptr++
			ptr += encode_size(int(length[0]), data[ptr:])
		}
	}
	if count > 2 || (pad != 0 && tot_size < maxlen) || ext_count > 0 {
		// Code 3
		var vbr int
		var pad_amount int = 0

		// Restart the process for the padding case
		ptr = 0
		if self_delimited != 0 {
			tot_size = int32(1 + b2i(length[count-1] >= 252))
		} else {
			tot_size = 0
		}
		vbr = 0
		for i = 1; i < count; i++ {
			if length[i] != length[0] {
				vbr = 1
				break
			}
		}
		if vbr != 0 {
			tot_size += 2
			for i = 0; i < count-1; i++ {
				tot_size += int32(1 + b2i(length[i] >= 252) + int(length[i]))
			}
			tot_size += int32(length[count-1])

			if tot_size > maxlen {
				return OPUS_BUFFER_TOO_SMALL
			}
			data[ptr] = (rp.toc & 0xFC) | 0x3
			ptr++
			data[ptr] = byte(count | 0x80)
			ptr++
		} else {
			tot_size += int32(count*int(length[0]) + 2)
			if tot_size > maxlen {
				return OPUS_BUFFER_TOO_SMALL
			}
			data[ptr] = (rp.toc & 0xFC) | 0x3
			ptr++
			data[ptr] = byte(count)
			ptr++
		}
		if pad != 0 {
			pad_amount = int(maxlen - tot_size)
		} else {
			pad_amount = 0
		}
		if ext_count > 0 {
			// figure out how much space we need for the extensions
			ext_len = int(PacketExtensionsGenerate(nil, maxlen-tot_size, all_extensions, int32(ext_count), 0))
			if ext_len < 0 {
				return int32(ext_len)
			}
			if pad == 0 {
				pad_amount = ext_len + ext_len/254 + 1
			}
		}
		if pad_amount != 0 {
			var nb_255s int
			data[1] |= 0x40
			nb_255s = (pad_amount - 1) / 255
			if int(tot_size)+ext_len+nb_255s+1 > int(maxlen) {
				return OPUS_BUFFER_TOO_SMALL
			}
			ext_begin = int(tot_size) + pad_amount - ext_len
			// Prepend 0x01 padding
			ones_begin = int(tot_size) + nb_255s + 1
			ones_end = int(tot_size) + pad_amount - ext_len
			for i = 0; i < nb_255s; i++ {
				data[ptr] = 255
				ptr++
			}
			data[ptr] = byte(pad_amount - 255*nb_255s - 1)
			ptr++
			tot_size += int32(pad_amount)
		}
		if vbr != 0 {
			for i = 0; i < count-1; i++ {
				ptr += encode_size(int(length[i]), data[ptr:])
			}
		}
	}
	if self_delimited != 0 {
		sdlen := encode_size(int(length[count-1]), data[ptr:])
		ptr += sdlen
	}
	// Copy the actual data
	for i = 0; i < count; i++ {
		// Using OPUS_MOVE() instead of OPUS_COPY() in case we're doing in-place
		// padding from opus_packet_pad or opus_packet_unpad().
		// assert disabled because it's not valid in C.
		// celt_assert(frames[i] + len[i] <= data || ptr <= frames[i]);
		copy(data[ptr:ptr+int(length[i])], frames[i][:length[i]])
		ptr += int(length[i])
	}
	if ext_len > 0 {
		ret := PacketExtensionsGenerate(data[ext_begin:], int32(ext_len), all_extensions, int32(ext_count), 0)
		celt.Assert(int(ret) == ext_len)
	}
	for i = ones_begin; i < ones_end; i++ {
		data[i] = 0x01
	}
	if pad != 0 && ext_count == 0 {
		// Fill padding with zeros.
		for ptr < int(maxlen) {
			data[ptr] = 0
			ptr++
		}
	}
	return tot_size
}

// C: opus_int32 opus_repacketizer_out_range(OpusRepacketizer *rp, int begin, int end, unsigned char *data, opus_int32 maxlen)
func (rp *OpusRepacketizer) RepacketizerOutRange(begin, end int, data []byte, maxlen int32) int32 {
	return rp.repacketizerOutRangeImpl(begin, end, data, maxlen, 0, 0, nil, 0)
}

// C: opus_int32 opus_repacketizer_out(OpusRepacketizer *rp, unsigned char *data, opus_int32 maxlen)
func (rp *OpusRepacketizer) RepacketizerOut(data []byte, maxlen int32) int32 {
	return rp.repacketizerOutRangeImpl(0, rp.nb_frames, data, maxlen, 0, 0, nil, 0)
}

// PacketPadImpl pads a packet in place to new_len bytes, optionally
// adding extensions; data must have room for new_len bytes.
//
// C: opus_int32 opus_packet_pad_impl(unsigned char *data, opus_int32 len, opus_int32 new_len, int pad, const opus_extension_data  *extensions, int nb_extensions)
func PacketPadImpl(data []byte, length int32, new_len int32, pad int, extensions []OpusExtensionData, nb_extensions int) int32 {
	var rp OpusRepacketizer
	var ret int32
	var cpy []byte
	if length < 1 {
		return OPUS_BAD_ARG
	}
	if length == new_len {
		return OPUS_OK
	} else if length > new_len {
		return OPUS_BAD_ARG
	}
	cpy = make([]byte, length)
	rp.RepacketizerInit()
	// Moving payload to the end of the packet so we can do in-place padding
	copy(cpy, data[:length])
	ret = int32(rp.RepacketizerCat(cpy, length))
	if ret != OPUS_OK {
		return ret
	}
	ret = rp.repacketizerOutRangeImpl(0, rp.nb_frames, data, new_len, 0, pad, extensions, nb_extensions)
	return ret
}

// PacketPad pads a packet in place to new_len bytes; data must have room
// for new_len bytes.
//
// C: int opus_packet_pad(unsigned char *data, opus_int32 len, opus_int32 new_len)
func PacketPad(data []byte, length int32, new_len int32) int {
	var ret int32
	ret = PacketPadImpl(data, length, new_len, 1, nil, 0)
	if ret > 0 {
		return OPUS_OK
	} else {
		return int(ret)
	}
}

// PacketUnpad removes all padding from a packet in place and returns the
// new length.
//
// C: opus_int32 opus_packet_unpad(unsigned char *data, opus_int32 len)
func PacketUnpad(data []byte, length int32) int32 {
	var rp OpusRepacketizer
	var ret int32
	var i int
	if length < 1 {
		return OPUS_BAD_ARG
	}
	rp.RepacketizerInit()
	ret = int32(rp.RepacketizerCat(data, length))
	if ret < 0 {
		return ret
	}
	// Discard all padding and extensions.
	for i = 0; i < rp.nb_frames; i++ {
		rp.padding_len[i] = 0
		rp.paddings[i] = nil
	}
	ret = rp.repacketizerOutRangeImpl(0, rp.nb_frames, data, length, 0, 0, nil, 0)
	celt.Assert(ret > 0 && ret <= length)
	return ret
}

// C: int opus_multistream_packet_pad(unsigned char *data, opus_int32 len, opus_int32 new_len, int nb_streams)
func MultistreamPacketPad(data []byte, length int32, new_len int32, nb_streams int) int {
	var s int
	var count int
	var toc byte
	var size [48]int16
	var packet_offset int32
	var amount int32

	if length < 1 {
		return OPUS_BAD_ARG
	}
	if length == new_len {
		return OPUS_OK
	} else if length > new_len {
		return OPUS_BAD_ARG
	}
	amount = new_len - length
	// Seek to last stream
	for s = 0; s < nb_streams-1; s++ {
		if length <= 0 {
			return OPUS_INVALID_PACKET
		}
		count = opus_packet_parse_impl(data, length, 1, &toc, nil,
			size[:], nil, &packet_offset, nil, nil)
		if count < 0 {
			return count
		}
		data = data[packet_offset:]
		length -= packet_offset
	}
	return PacketPad(data, length, length+amount)
}

// C: opus_int32 opus_multistream_packet_unpad(unsigned char *data, opus_int32 len, int nb_streams)
func MultistreamPacketUnpad(data []byte, length int32, nb_streams int) int32 {
	var s int
	var toc byte
	var size [48]int16
	var packet_offset int32
	var rp OpusRepacketizer
	var dst []byte
	var dst_len int32

	if length < 1 {
		return OPUS_BAD_ARG
	}
	dst = data
	dst_len = 0
	// Unpad all frames
	for s = 0; s < nb_streams; s++ {
		var ret int32
		var i int
		self_delimited := b2i(s != nb_streams-1)
		if length <= 0 {
			return OPUS_INVALID_PACKET
		}
		rp.RepacketizerInit()
		ret = int32(opus_packet_parse_impl(data, length, self_delimited, &toc, nil,
			size[:], nil, &packet_offset, nil, nil))
		if ret < 0 {
			return ret
		}
		ret = int32(rp.repacketizerCatImpl(data, packet_offset, self_delimited))
		if ret < 0 {
			return ret
		}
		// Discard all padding and extensions.
		for i = 0; i < rp.nb_frames; i++ {
			rp.padding_len[i] = 0
			rp.paddings[i] = nil
		}
		ret = rp.repacketizerOutRangeImpl(0, rp.nb_frames, dst, length, self_delimited, 0, nil, 0)
		if ret < 0 {
			return ret
		} else {
			dst_len += ret
		}
		dst = dst[ret:]
		data = data[packet_offset:]
		length -= packet_offset
	}
	return dst_len
}
