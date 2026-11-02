// Package testvec holds the file formats and deterministic helpers shared by
// the corpus generator (tools/corpus), the Go oracle twin (internal/oracle)
// and the conformance tests.  Nothing in here depends on the codec itself.
package testvec

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
)

// Packet is one entry of an opus_demo-style packet file.  Len(Data)==0 means
// the packet was lost (or encoded to nothing, e.g. DTX).
type Packet struct {
	Data []byte
	Rng  uint32 // encoder final range as reported by OPUS_GET_FINAL_RANGE
}

// MaxPacket mirrors MAX_PACKET in gen.c and opus_demo.c.
const MaxPacket = 1500

// ReadPackets parses the opus_demo packet format:
// 4-byte big-endian length, 4-byte big-endian final range, payload.
func ReadPackets(r io.Reader) ([]Packet, error) {
	br := bufio.NewReader(r)
	var out []Packet
	var hdr [8]byte
	for {
		_, err := io.ReadFull(br, hdr[:])
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return out, fmt.Errorf("packet %d: truncated header: %w", len(out), err)
		}
		n := binary.BigEndian.Uint32(hdr[0:4])
		rng := binary.BigEndian.Uint32(hdr[4:8])
		if n > MaxPacket {
			return out, fmt.Errorf("packet %d: length %d exceeds %d", len(out), n, MaxPacket)
		}
		data := make([]byte, n)
		if _, err := io.ReadFull(br, data); err != nil {
			return out, fmt.Errorf("packet %d: truncated payload: %w", len(out), err)
		}
		out = append(out, Packet{Data: data, Rng: rng})
	}
}

// WritePackets writes packets in the opus_demo format.
func WritePackets(w io.Writer, pkts []Packet) error {
	bw := bufio.NewWriter(w)
	var hdr [8]byte
	for _, p := range pkts {
		binary.BigEndian.PutUint32(hdr[0:4], uint32(len(p.Data)))
		binary.BigEndian.PutUint32(hdr[4:8], p.Rng)
		if _, err := bw.Write(hdr[:]); err != nil {
			return err
		}
		if _, err := bw.Write(p.Data); err != nil {
			return err
		}
	}
	return bw.Flush()
}

// ReadPacketFile is ReadPackets on a path.
func ReadPacketFile(path string) ([]Packet, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ReadPackets(f)
}

// ReadS16 reads little-endian signed 16-bit PCM.
func ReadS16(r io.Reader) ([]int16, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if len(b)%2 != 0 {
		return nil, errors.New("odd byte count in s16 stream")
	}
	out := make([]int16, len(b)/2)
	for i := range out {
		out[i] = int16(binary.LittleEndian.Uint16(b[2*i:]))
	}
	return out, nil
}

// ReadS16File is ReadS16 on a path.
func ReadS16File(path string) ([]int16, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ReadS16(f)
}

// S16Bytes serialises PCM as little-endian bytes (the layout CRCs and hashes
// are computed over).
func S16Bytes(pcm []int16) []byte {
	b := make([]byte, 2*len(pcm))
	for i, v := range pcm {
		binary.LittleEndian.PutUint16(b[2*i:], uint16(v))
	}
	return b
}

// WriteS16 writes little-endian signed 16-bit PCM.
func WriteS16(w io.Writer, pcm []int16) error {
	_, err := w.Write(S16Bytes(pcm))
	return err
}
