package transfer

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/aegis-p2p/aegis/internal/crypto"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"
)

const Timeout = 30 * time.Second

type Upload struct {
	file           *os.File
	ID, Name, Hash string
	Size           int64
	Chunks         int
}

func Prepare(path string) (*Upload, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	success := false
	defer func() {
		if !success {
			_ = f.Close()
		}
	}()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > MaxFileSize {
		return nil, fmt.Errorf("choose a regular, nonempty file up to 100 MB")
	}
	if _, err := safeName(filepath.Base(path)); err != nil {
		return nil, err
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, io.LimitReader(f, MaxFileSize+1)); err != nil {
		return nil, err
	}
	after, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()) {
		return nil, fmt.Errorf("file changed while preparing")
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return nil, err
	}
	success = true
	return &Upload{file: f, ID: hex.EncodeToString(id[:]), Name: info.Name(), Hash: hex.EncodeToString(hash.Sum(nil)), Size: info.Size(), Chunks: int((info.Size() + ChunkSize - 1) / ChunkSize)}, nil
}
func (u *Upload) Close() {
	if u != nil && u.file != nil {
		_ = u.file.Close()
	}
}

type download struct {
	digest             hash.Hash
	offer              AegisFrame
	data               []byte
	chunks             int
	accepted, verified bool
}
type Snapshot struct {
	State, Name, ID      string
	Current, Total       int
	Unsaved, Offer, Busy bool
}

// Engine owns transfer metadata and buffers independently of the interface.
// A single transfer is allowed in either direction; its lock also protects async IO.
type Engine struct {
	mu        sync.Mutex
	upload    *Upload
	recv      *download
	current   int
	waiting   bool
	saving    bool
	state     string
	lastID    string
	lastName  string
	lastTotal int
	activity  time.Time
}

func safeName(name string) (string, error) {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	if name == "." || name == ".." || name == "" {
		return "", fmt.Errorf("invalid file name")
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return "", fmt.Errorf("invalid file name")
		}
	}
	return name, nil
}
func (e *Engine) Snapshot() Snapshot {
	e.mu.Lock()
	defer e.mu.Unlock()
	s := Snapshot{State: e.state, Name: e.lastName, ID: e.lastID, Current: e.current, Total: e.lastTotal}
	if e.upload != nil {
		s.Busy = true
		s.Name = e.upload.Name
		s.ID = e.upload.ID
		s.Total = e.upload.Chunks
	}
	if e.recv != nil {
		s.Busy = true
		s.Name = e.recv.offer.Name
		s.ID = e.recv.offer.TransferID
		s.Current = e.recv.chunks
		s.Total = e.recv.offer.Chunks
		s.Offer = !e.recv.accepted
		s.Unsaved = e.recv.verified
	}
	return s
}
func (e *Engine) Offer(u *Upload) (AegisFrame, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.upload != nil || e.recv != nil {
		u.Close()
		return AegisFrame{}, fmt.Errorf("another transfer is active")
	}
	e.upload = u
	e.current = 0
	e.waiting = false
	e.state = "waiting for acceptance"
	e.activity = time.Now()
	e.lastID = u.ID
	e.lastName = u.Name
	e.lastTotal = u.Chunks
	return AegisFrame{Type: TypeFileOffer, TransferID: u.ID, Name: u.Name, Size: u.Size, Chunks: u.Chunks, SHA256: u.Hash}, nil
}
func (e *Engine) Accept(accept bool) (AegisFrame, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.recv == nil || e.recv.accepted {
		return AegisFrame{}, fmt.Errorf("no pending offer")
	}
	id := e.recv.offer.TransferID
	if !accept {
		e.clearLocked()
		e.state = "rejected"
		return AegisFrame{Type: TypeFileReject, TransferID: id}, nil
	}
	e.recv.accepted = true
	e.recv.digest = sha256.New()
	e.recv.data = make([]byte, 0, e.recv.offer.Size)
	e.state = "receiving"
	e.activity = time.Now()
	return AegisFrame{Type: TypeFileAccept, TransferID: id}, nil
}
func (e *Engine) NextChunk() (AegisFrame, error) {
	e.mu.Lock()
	if e.upload == nil || e.state != "sending" || e.waiting {
		e.mu.Unlock()
		return AegisFrame{}, fmt.Errorf("transfer no longer sending")
	}
	u, index := e.upload, e.current
	size := int64(ChunkSize)
	offset := int64(index) * ChunkSize
	if remaining := u.Size - offset; remaining < size {
		size = remaining
	}
	if size <= 0 {
		e.mu.Unlock()
		return AegisFrame{}, fmt.Errorf("invalid chunk offset")
	}
	e.waiting = true
	e.activity = time.Now()
	e.mu.Unlock()
	buf := make([]byte, size)
	if _, err := io.ReadFull(io.NewSectionReader(u.file, offset, size), buf); err != nil {
		crypto.Bytes(buf)
		return AegisFrame{}, err
	}
	e.mu.Lock()
	valid := e.upload == u && e.state == "sending" && e.current == index
	e.mu.Unlock()
	if !valid {
		crypto.Bytes(buf)
		return AegisFrame{}, fmt.Errorf("transfer cancelled")
	}
	return AegisFrame{Type: TypeChunk, TransferID: u.ID, Index: index, Data: buf}, nil
}

// Handle returns response frames and whether a background chunk read is needed.
func (e *Engine) Handle(f AegisFrame) ([]AegisFrame, bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := f.Validate(); err != nil {
		return nil, false, err
	}
	if f.Type == TypeFileOffer {
		name, err := safeName(f.Name)
		if err != nil {
			return []AegisFrame{{Type: TypeFileReject, TransferID: f.TransferID}}, false, nil
		}
		if e.upload != nil || e.recv != nil {
			return []AegisFrame{{Type: TypeFileReject, TransferID: f.TransferID}}, false, nil
		}
		f.Name = name
		e.recv = &download{offer: f}
		e.state = "offer received"
		e.activity = time.Now()
		e.lastID = f.TransferID
		e.lastName = name
		e.lastTotal = f.Chunks
		return nil, false, nil
	}
	if f.Type == TypeFileSaved && f.TransferID == e.lastID && e.upload == nil && e.recv == nil {
		e.state = "saved by peer"
		return nil, false, nil
	}
	if e.upload != nil && f.TransferID == e.upload.ID {
		switch f.Type {
		case TypeFileAccept:
			if e.state != "waiting for acceptance" {
				return nil, false, fmt.Errorf("unexpected acceptance")
			}
			e.state = "sending"
			e.activity = time.Now()
			return nil, true, nil
		case TypeFileReject, TypeFileCancel:
			e.clearLocked()
			e.state = "rejected or cancelled"
			return nil, false, nil
		case TypeChunkAck:
			if e.state != "sending" || !e.waiting || f.Index != e.current {
				return nil, false, fmt.Errorf("unexpected chunk acknowledgement")
			}
			e.current++
			e.waiting = false
			e.activity = time.Now()
			if e.current == e.upload.Chunks {
				e.state = "transferred; waiting for verification"
				return []AegisFrame{{Type: TypeFileDone, TransferID: e.upload.ID, SHA256: e.upload.Hash}}, false, nil
			}
			return nil, true, nil
		case TypeFileVerified:
			if e.state != "transferred; waiting for verification" {
				return nil, false, fmt.Errorf("unexpected verification")
			}
			e.clearLocked()
			e.state = "verified by peer"
			return nil, false, nil
		}
	}
	if e.recv != nil && f.TransferID == e.recv.offer.TransferID {
		switch f.Type {
		case TypeFileCancel:
			e.clearLocked()
			e.state = "cancelled by peer"
			return nil, false, nil
		case TypeChunk:
			if !e.recv.accepted || e.recv.verified || f.Index != e.recv.chunks {
				return nil, false, fmt.Errorf("unexpected file chunk")
			}
			expected := int64(ChunkSize)
			remaining := e.recv.offer.Size - int64(len(e.recv.data))
			if remaining < expected {
				expected = remaining
			}
			if int64(len(f.Data)) != expected || expected <= 0 {
				return nil, false, fmt.Errorf("invalid chunk length")
			}
			e.recv.data = append(e.recv.data, f.Data...)
			_, _ = e.recv.digest.Write(f.Data)
			e.recv.chunks++
			e.activity = time.Now()
			return []AegisFrame{{Type: TypeChunkAck, TransferID: f.TransferID, Index: f.Index}}, false, nil
		case TypeFileDone:
			if !e.recv.accepted || e.recv.verified || e.recv.chunks != e.recv.offer.Chunks || int64(len(e.recv.data)) != e.recv.offer.Size {
				return nil, false, fmt.Errorf("incomplete transfer")
			}
			digest := e.recv.digest.Sum(nil)
			if hex.EncodeToString(digest) != e.recv.offer.SHA256 || f.SHA256 != e.recv.offer.SHA256 {
				id := f.TransferID
				e.clearLocked()
				e.state = "integrity check failed"
				return []AegisFrame{{Type: TypeFileCancel, TransferID: id}}, false, fmt.Errorf("file integrity check failed")
			}
			e.recv.verified = true
			e.state = "verified; not saved"
			return []AegisFrame{{Type: TypeFileVerified, TransferID: f.TransferID}}, false, nil
		}
	}
	// Late packets from a cancelled/completed transfer cannot affect its successor.
	return nil, false, nil
}
func (e *Engine) Save(path string, overwrite bool) (AegisFrame, error) {
	e.mu.Lock()
	if e.recv == nil || !e.recv.verified || e.saving {
		e.mu.Unlock()
		return AegisFrame{}, fmt.Errorf("no verified file available to save")
	}
	received := e.recv
	// The save owns a bounded copy so cancellation can clear the reception buffer.
	data := append([]byte(nil), received.data...)
	e.saving = true
	e.mu.Unlock()
	defer crypto.Bytes(data)
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		path = filepath.Join(path, received.offer.Name)
	}
	err := SaveFileWithOverwrite(path, data, overwrite)
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.recv == received {
		e.saving = false
		if err == nil {
			e.clearLocked()
			e.state = "saved locally"
		}
	}
	if err != nil {
		return AegisFrame{}, err
	}
	return AegisFrame{Type: TypeFileSaved, TransferID: received.offer.TransferID}, nil
}
func (e *Engine) Cancel() AegisFrame {
	e.mu.Lock()
	defer e.mu.Unlock()
	id := e.lastID
	e.clearLocked()
	e.state = "cancelled"
	return AegisFrame{Type: TypeFileCancel, TransferID: id}
}
func (e *Engine) Expire(now time.Time) (AegisFrame, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if (e.upload == nil && e.recv == nil) || (e.recv != nil && e.recv.verified) || now.Sub(e.activity) < Timeout {
		return AegisFrame{}, false
	}
	id := e.lastID
	e.clearLocked()
	e.state = "transfer timed out"
	return AegisFrame{Type: TypeFileCancel, TransferID: id}, true
}
func (e *Engine) clearLocked() {
	if e.upload != nil {
		e.upload.Close()
		e.upload = nil
	}
	if e.recv != nil {
		e.current = e.recv.chunks
		crypto.Bytes(e.recv.data)
		e.recv = nil
	}
	e.waiting = false
	e.saving = false
}
func (e *Engine) Close() { e.mu.Lock(); defer e.mu.Unlock(); e.clearLocked() }
