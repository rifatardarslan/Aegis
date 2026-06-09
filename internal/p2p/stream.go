package p2p

import (
	"encoding/binary"
	"fmt"
	"io"
)

// ProtocolID is the libp2p protocol identifier for Aegis messaging streams.
const ProtocolID = "/aegis/1.0.0"

// WriteFrame writes a 4-byte big-endian length-prefixed payload to the writer.
func WriteFrame(w io.Writer, payload []byte) error {
	length := uint32(len(payload))
	if err := binary.Write(w, binary.BigEndian, length); err != nil {
		return fmt.Errorf("failed to write frame length: %w", err)
	}
	if _, err := w.Write(payload); err != nil {
		return fmt.Errorf("failed to write frame payload: %w", err)
	}
	return nil
}

// ReadFrame reads a 4-byte big-endian length-prefixed payload from the reader.
func ReadFrame(r io.Reader) ([]byte, error) {
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
	if length > 512*1024 {
		return nil, fmt.Errorf("frame size exceeds secure limit of 512 KB: %d bytes", length)
	}

	buf := make([]byte, length)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, fmt.Errorf("failed to read frame payload: %w", err)
	}
	return buf, nil
}
