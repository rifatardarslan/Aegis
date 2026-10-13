package p2p

import (
	"encoding/binary"
	"fmt"
	"io"
)

// ProtocolID is the libp2p protocol identifier for Aegis messaging streams.
const ProtocolID = "/aegis/2.0.0"

// WriteFrame writes a 4-byte big-endian length-prefixed payload to the writer.
func WriteFrame(w io.Writer, payload []byte) error {
	if len(payload) == 0 || len(payload) > 512*1024 {
		return fmt.Errorf("invalid frame size: %d", len(payload))
	}
	length := uint32(len(payload))
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], length)
	if err := writeAll(w, header[:]); err != nil {
		return fmt.Errorf("failed to write frame length: %w", err)
	}
	if err := writeAll(w, payload); err != nil {
		return fmt.Errorf("failed to write frame payload: %w", err)
	}
	return nil
}

// ReadFrame reads a 4-byte big-endian length-prefixed payload from the reader.
func ReadFrame(r io.Reader) ([]byte, error) { return readLimitedFrame(r, 512*1024) }

func readControl(r io.Reader) ([]byte, error) { return readLimitedFrame(r, 4096) }

func readLimitedFrame(r io.Reader, maximum uint32) ([]byte, error) {
	var length uint32
	if err := binary.Read(r, binary.BigEndian, &length); err != nil {
		return nil, err
	}

	// Reject zero-length frames — no valid Aegis frame is empty.
	if length == 0 {
		return nil, fmt.Errorf("received zero-length frame")
	}

	// Strict siber güvenlik sınırı: Aegis ağındaki en büyük geçerli paket 256 KB'lık dosya parçasıdır.
	// JSON kodlama ve imza ek yüküyle birlikte maksimum limit 512 KB (524288 byte) olarak belirlenmiştir.
	if length > maximum {
		return nil, fmt.Errorf("frame size exceeds limit %d: %d bytes", maximum, length)
	}

	buf := make([]byte, length)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, fmt.Errorf("failed to read frame payload: %w", err)
	}
	return buf, nil
}

func writeAll(w io.Writer, b []byte) error {
	for len(b) > 0 {
		n, err := w.Write(b)
		if err != nil {
			return err
		}
		if n <= 0 || n > len(b) {
			return io.ErrShortWrite
		}
		b = b[n:]
	}
	return nil
}
