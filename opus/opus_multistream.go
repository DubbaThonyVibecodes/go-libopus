package opus

// C: src/opus_multistream.c, include/opus_multistream.h, struct ChannelLayout from src/opus_private.h

// ChannelLayout is the C struct ChannelLayout.
type ChannelLayout struct {
	nb_channels        int
	nb_streams         int
	nb_coupled_streams int
	mapping            [256]byte
}

// C: int validate_layout(const ChannelLayout *layout)
func validate_layout(layout *ChannelLayout) int {
	var i, max_channel int

	max_channel = layout.nb_streams + layout.nb_coupled_streams
	if max_channel > 255 {
		return 0
	}
	for i = 0; i < layout.nb_channels; i++ {
		if int(layout.mapping[i]) >= max_channel && layout.mapping[i] != 255 {
			return 0
		}
	}
	return 1
}

// C: int get_left_channel(const ChannelLayout *layout, int stream_id, int prev)
func get_left_channel(layout *ChannelLayout, stream_id int, prev int) int {
	var i int
	if prev < 0 {
		i = 0
	} else {
		i = prev + 1
	}
	for ; i < layout.nb_channels; i++ {
		if int(layout.mapping[i]) == stream_id*2 {
			return i
		}
	}
	return -1
}

// C: int get_right_channel(const ChannelLayout *layout, int stream_id, int prev)
func get_right_channel(layout *ChannelLayout, stream_id int, prev int) int {
	var i int
	if prev < 0 {
		i = 0
	} else {
		i = prev + 1
	}
	for ; i < layout.nb_channels; i++ {
		if int(layout.mapping[i]) == stream_id*2+1 {
			return i
		}
	}
	return -1
}

// C: int get_mono_channel(const ChannelLayout *layout, int stream_id, int prev)
func get_mono_channel(layout *ChannelLayout, stream_id int, prev int) int {
	var i int
	if prev < 0 {
		i = 0
	} else {
		i = prev + 1
	}
	for ; i < layout.nb_channels; i++ {
		if int(layout.mapping[i]) == stream_id+layout.nb_coupled_streams {
			return i
		}
	}
	return -1
}
