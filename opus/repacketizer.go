package opus

// C: src/repacketizer.c, struct OpusRepacketizer from src/opus_private.h

// OpusRepacketizer is the C struct OpusRepacketizer.
type OpusRepacketizer struct {
	toc       byte
	nb_frames int
	frames    [48][]byte
	len       [48]int16
	framesize int
}

// C: int opus_repacketizer_get_size(void)
func RepacketizerGetSize() int {
	return 48*8 + 48*2 + 16 // sizeof(OpusRepacketizer) on a 64-bit platform, for API parity
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

// C: int opus_repacketizer_cat(OpusRepacketizer *rp, const unsigned char *data, opus_int32 len)
func (rp *OpusRepacketizer) RepacketizerCat(data []byte, length int32) int {
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

	ret = PacketParse(data, length, &tmp_toc, rp.frames[rp.nb_frames:], rp.len[rp.nb_frames:], nil)
	if ret < 1 {
		return ret
	}

	rp.nb_frames += curr_nb_frames
	return OPUS_OK
}

// C: int opus_repacketizer_get_nb_frames(OpusRepacketizer *rp)
func (rp *OpusRepacketizer) RepacketizerGetNbFrames() int {
	return rp.nb_frames
}

// C: opus_int32 opus_repacketizer_out_range_impl(OpusRepacketizer *rp, int begin, int end, unsigned char *data, opus_int32 maxlen, int self_delimited)
func (rp *OpusRepacketizer) repacketizerOutRangeImpl(begin, end int, data []byte, maxlen int32, self_delimited int) int32 {
	var i, count int
	var tot_size int32
	var length []int16
	var frames [][]byte
	pos := 0 // index of the C `data` pointer

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

	switch count {
	case 1:
		// Code 0
		tot_size += int32(length[0]) + 1
		if tot_size > maxlen {
			return OPUS_BUFFER_TOO_SMALL
		}
		data[pos] = rp.toc & 0xFC
		pos++
	case 2:
		if length[1] == length[0] {
			// Code 1
			tot_size += 2*int32(length[0]) + 1
			if tot_size > maxlen {
				return OPUS_BUFFER_TOO_SMALL
			}
			data[pos] = (rp.toc & 0xFC) | 0x1
			pos++
		} else {
			// Code 2
			tot_size += int32(length[0]) + int32(length[1]) + 2 + int32(b2i(length[0] >= 252))
			if tot_size > maxlen {
				return OPUS_BUFFER_TOO_SMALL
			}
			data[pos] = (rp.toc & 0xFC) | 0x2
			pos++
			pos += encode_size(int(length[0]), data[pos:])
		}
	default:
		// Code 3
		var vbr int

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
			data[pos] = (rp.toc & 0xFC) | 0x3
			pos++
			data[pos] = byte(count | 0x80)
			pos++
			for i = 0; i < count-1; i++ {
				pos += encode_size(int(length[i]), data[pos:])
			}
		} else {
			tot_size += int32(count*int(length[0]) + 2)
			if tot_size > maxlen {
				return OPUS_BUFFER_TOO_SMALL
			}
			data[pos] = (rp.toc & 0xFC) | 0x3
			pos++
			data[pos] = byte(count)
			pos++
		}
	}
	if self_delimited != 0 {
		sdlen := encode_size(int(length[count-1]), data[pos:])
		pos += sdlen
	}
	// Copy the actual data
	for i = 0; i < count; i++ {
		copy(data[pos:pos+int(length[i])], frames[i][:length[i]])
		pos += int(length[i])
	}
	return tot_size
}

// C: opus_int32 opus_repacketizer_out_range(OpusRepacketizer *rp, int begin, int end, unsigned char *data, opus_int32 maxlen)
func (rp *OpusRepacketizer) RepacketizerOutRange(begin, end int, data []byte, maxlen int32) int32 {
	return rp.repacketizerOutRangeImpl(begin, end, data, maxlen, 0)
}

// C: opus_int32 opus_repacketizer_out(OpusRepacketizer *rp, unsigned char *data, opus_int32 maxlen)
func (rp *OpusRepacketizer) RepacketizerOut(data []byte, maxlen int32) int32 {
	return rp.repacketizerOutRangeImpl(0, rp.nb_frames, data, maxlen, 0)
}
