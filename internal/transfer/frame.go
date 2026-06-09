package transfer

// FrameType identifies the kind of Aegis frame multiplexed over the P2P stream.
type FrameType string

const (
	TypeMsg          FrameType = "MSG"            // Standard text chat message
	TypeFileOffer    FrameType = "FILE_OFFER"     // Offer a file to the peer
	TypeFileAccept   FrameType = "FILE_ACCEPT"    // Accept the file transfer
	TypeFileReject   FrameType = "FILE_REJECT"    // Reject the file transfer
	TypeChunk        FrameType = "CHUNK"          // File chunk payload
	TypeChunkAck     FrameType = "CHUNK_ACK"      // Acknowledge a chunk receipt
	TypeFileDone     FrameType = "FILE_DONE"      // File transfer complete and verified
	TypeTyping       FrameType = "TYPING"         // Peer is typing
	TypeMsgDelivered FrameType = "MSG_DELIVERED"  // Message delivered ack
	TypeMsgRead      FrameType = "MSG_READ"       // Message read ack
	TypePing         FrameType = "PING"           // Latency ping
	TypePong         FrameType = "PONG"           // Latency pong
)

// AegisFrame represents the JSON frame structure encrypted and signed over the wire.
type AegisFrame struct {
	Type    FrameType `json:"t"`
	Name    string    `json:"n,omitempty"`
	Size    int64     `json:"s,omitempty"`
	Chunks  int       `json:"c,omitempty"`
	SHA256  string    `json:"h,omitempty"`
	Index   int       `json:"i,omitempty"`
	Data    []byte    `json:"d,omitempty"`
	MsgText string    `json:"m,omitempty"`
}
