package opus

import "github.com/DubbaThonyVibecodes/go-libopus/celt"

// C: src/extensions.c, opus_extension_data from src/opus_private.h

// OpusExtensionData is the C struct opus_extension_data.
type OpusExtensionData struct {
	Id    int
	Frame int
	Data  []byte
	Len   int32
}

// Given an extension payload, advance data to the next extension and return the
// length of the remaining extensions.
//
// C: opus_int32 skip_extension(const unsigned char **data, opus_int32 len, opus_int32 *header_size)
func skip_extension(data *[]byte, length int32, header_size *int32) int32 {
	var id, L int
	if length == 0 {
		return 0
	}
	id = int((*data)[0] >> 1)
	L = int((*data)[0] & 1)
	if id == 0 && L == 1 {
		*header_size = 1
		if length < 1 {
			return -1
		}
		*data = (*data)[1:]
		length--
		return length
	} else if id > 0 && id < 32 {
		if length < int32(1+L) {
			return -1
		}
		*data = (*data)[1+L:]
		length -= int32(1 + L)
		*header_size = 1
		return length
	} else {
		if L == 0 {
			*data = (*data)[length:]
			*header_size = 1
			return 0
		} else {
			var bytes int32 = 0
			*header_size = 1
			for {
				*data = (*data)[1:]
				length--
				if length == 0 {
					return -1
				}
				bytes += int32((*data)[0])
				(*header_size)++
				if (*data)[0] != 255 {
					break
				}
			}
			*data = (*data)[1:]
			length--
			if bytes <= length {
				length -= bytes
				*data = (*data)[bytes:]
			} else {
				return -1
			}
			return length
		}
	}
}

// PacketExtensionsCount counts the number of extensions, excluding real
// padding and separators.
//
// C: opus_int32 opus_packet_extensions_count(const unsigned char *data, opus_int32 len)
func PacketExtensionsCount(data []byte, length int32) int32 {
	var curr_len int32
	var count int32 = 0
	curr_data := data

	celt.Assert(length >= 0)
	celt.Assert(data != nil || length == 0)

	curr_len = length
	for curr_len > 0 {
		var id int
		var header_size int32
		id = int(curr_data[0] >> 1)
		curr_len = skip_extension(&curr_data, curr_len, &header_size)
		if curr_len < 0 {
			return OPUS_INVALID_PACKET
		}
		if id > 1 {
			count++
		}
	}
	return count
}

// PacketExtensionsParse extracts extensions from Opus padding (excluding
// real padding and separators). On entry *nb_extensions is the capacity of
// extensions; on return it is the number of extensions found. Each
// extension's Data is the slice of data holding its payload (Len bytes).
//
// C: opus_int32 opus_packet_extensions_parse(const unsigned char *data, opus_int32 len, opus_extension_data *extensions, opus_int32 *nb_extensions)
func PacketExtensionsParse(data []byte, length int32, extensions []OpusExtensionData, nb_extensions *int32) int32 {
	var curr_data []byte
	var curr_len int32
	var curr_frame int = 0
	var count int32 = 0

	celt.Assert(length >= 0)
	celt.Assert(data != nil || length == 0)
	celt.Assert(nb_extensions != nil)
	celt.Assert(extensions != nil || *nb_extensions == 0)

	curr_data = data
	curr_len = length
	for curr_len > 0 {
		var id int
		var header_size int32
		var curr_ext OpusExtensionData
		id = int(curr_data[0] >> 1)
		if id > 1 {
			curr_ext.Id = id
			curr_ext.Frame = curr_frame
			curr_ext.Data = curr_data
		} else if id == 1 {
			L := int(curr_data[0] & 1)
			if L == 0 {
				curr_frame++
			} else {
				if curr_len >= 2 {
					curr_frame += int(curr_data[1])
				}
				// Else we're at the end and it doesn't matter.
			}
			if curr_frame >= 48 {
				*nb_extensions = count
				return OPUS_INVALID_PACKET
			}
		}
		curr_len = skip_extension(&curr_data, curr_len, &header_size)
		// printf("curr_len = %d, header_size = %d\n", curr_len, header_size);
		if curr_len < 0 {
			*nb_extensions = count
			return OPUS_INVALID_PACKET
		}
		celt.Assert(int32(len(data)-len(curr_data)) == length-curr_len)
		if id > 1 {
			if count == *nb_extensions {
				return OPUS_BUFFER_TOO_SMALL
			}
			curr_ext.Len = int32(len(curr_ext.Data)-len(curr_data)) - header_size
			curr_ext.Data = curr_ext.Data[header_size : header_size+curr_ext.Len]
			extensions[count] = curr_ext
			count++
		}
	}
	celt.Assert(curr_len == 0)
	*nb_extensions = count
	return OPUS_OK
}

// PacketExtensionsGenerate writes the extensions into data (which may be
// nil to only compute the size) and returns the number of bytes used.
//
// C: opus_int32 opus_packet_extensions_generate(unsigned char *data, opus_int32 len, const opus_extension_data  *extensions, opus_int32 nb_extensions, int pad)
func PacketExtensionsGenerate(data []byte, length int32, extensions []OpusExtensionData, nb_extensions int32, pad int) int32 {
	var max_frame int = 0
	var i int32
	var frame int
	var curr_frame int = 0
	var pos int32 = 0
	var written int32 = 0

	celt.Assert(length >= 0)

	for i = 0; i < nb_extensions; i++ {
		max_frame = IMAX(max_frame, extensions[i].Frame)
		if extensions[i].Id < 2 || extensions[i].Id > 127 {
			return OPUS_BAD_ARG
		}
	}
	if max_frame >= 48 {
		return OPUS_BAD_ARG
	}
	for frame = 0; frame <= max_frame; frame++ {
		for i = 0; i < nb_extensions; i++ {
			if extensions[i].Frame == frame {
				// Insert separator when needed.
				if frame != curr_frame {
					diff := frame - curr_frame
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
					curr_frame = frame
				}
				if extensions[i].Id < 32 {
					if extensions[i].Len < 0 || extensions[i].Len > 1 {
						return OPUS_BAD_ARG
					}
					if length-pos < extensions[i].Len+1 {
						return OPUS_BUFFER_TOO_SMALL
					}
					if data != nil {
						data[pos] = byte(int32(extensions[i].Id<<1) + extensions[i].Len)
					}
					pos++
					if extensions[i].Len > 0 {
						if data != nil {
							data[pos] = extensions[i].Data[0]
						}
						pos++
					}
				} else {
					var last int
					var length_bytes int32
					if extensions[i].Len < 0 {
						return OPUS_BAD_ARG
					}
					last = b2i(written == nb_extensions-1)
					length_bytes = 1 + extensions[i].Len/255
					if last != 0 {
						length_bytes = 0
					}
					if length-pos < 1+length_bytes+extensions[i].Len {
						return OPUS_BUFFER_TOO_SMALL
					}
					if data != nil {
						data[pos] = byte((extensions[i].Id << 1) + b2i(last == 0))
					}
					pos++
					if last == 0 {
						var j int32
						for j = 0; j < extensions[i].Len/255; j++ {
							if data != nil {
								data[pos] = 255
							}
							pos++
						}
						if data != nil {
							data[pos] = byte(extensions[i].Len % 255)
						}
						pos++
					}
					if data != nil {
						copy(data[pos:pos+extensions[i].Len], extensions[i].Data[:extensions[i].Len])
					}
					pos += extensions[i].Len
				}
				written++
			}
		}
	}
	// If we need to pad, just prepend 0x01 bytes. Even better would be to fill the
	// end with zeros, but that requires checking that turning the last extesion into
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
