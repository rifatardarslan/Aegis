package transfer

import (
	"encoding/hex"
	"fmt"
)

type FrameType string

const (
	TypeMsg          FrameType = "MSG"
	TypeFileOffer    FrameType = "FILE_OFFER"
	TypeFileAccept   FrameType = "FILE_ACCEPT"
	TypeFileReject   FrameType = "FILE_REJECT"
	TypeChunk        FrameType = "CHUNK"
	TypeChunkAck     FrameType = "CHUNK_ACK"
	TypeFileDone     FrameType = "FILE_DONE"
	TypeFileVerified FrameType = "FILE_VERIFIED"
	TypeFileSaved    FrameType = "FILE_SAVED"
	TypeFileCancel   FrameType = "FILE_CANCEL"
	TypeTyping       FrameType = "TYPING"
	TypeMsgDelivered FrameType = "MSG_DELIVERED"
	TypePing         FrameType = "PING"
	TypePong         FrameType = "PONG"
)
const ChunkSize = 256 * 1024
const MaxFileSize int64 = 100 * 1024 * 1024

type AegisFrame struct {
	Type       FrameType `json:"t"`
	TransferID string    `json:"x,omitempty"`
	Name       string    `json:"n,omitempty"`
	Size       int64     `json:"s,omitempty"`
	Chunks     int       `json:"c,omitempty"`
	SHA256     string    `json:"h,omitempty"`
	Index      int       `json:"i,omitempty"`
	Data       []byte    `json:"d,omitempty"`
	MsgText    string    `json:"m,omitempty"`
}

func (f AegisFrame) Validate() error {
	switch f.Type {
	case TypeMsg:
		if len(f.MsgText) > 8192 || f.Index < 0 {
			return fmt.Errorf("invalid message")
		}
	case TypeTyping, TypePing, TypePong:
	case TypeMsgDelivered:
		if f.Index < 0 {
			return fmt.Errorf("invalid message acknowledgement")
		}
	case TypeFileOffer, TypeFileAccept, TypeFileReject, TypeChunk, TypeChunkAck, TypeFileDone, TypeFileVerified, TypeFileSaved, TypeFileCancel:
		id, err := hex.DecodeString(f.TransferID)
		if err != nil || len(id) != 16 {
			return fmt.Errorf("invalid transfer ID")
		}
		if f.Index < 0 || len(f.Data) > ChunkSize {
			return fmt.Errorf("invalid chunk")
		}
		if f.Type == TypeFileOffer {
			if f.Size <= 0 || f.Size > MaxFileSize || f.Chunks != int((f.Size+ChunkSize-1)/ChunkSize) {
				return fmt.Errorf("invalid file size or chunk count")
			}
			hash, err := hex.DecodeString(f.SHA256)
			if err != nil || len(hash) != 32 {
				return fmt.Errorf("invalid file digest")
			}
			if len(f.Name) == 0 || len(f.Name) > 255 {
				return fmt.Errorf("invalid file name")
			}
		}
	default:
		return fmt.Errorf("unsupported packet type")
	}
	return nil
}
