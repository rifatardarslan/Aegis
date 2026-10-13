package transfer

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func upload(t *testing.T) *Upload {
	t.Helper()
	path := filepath.Join(t.TempDir(), "payload.dat")
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), ChunkSize+15), 0600); err != nil {
		t.Fatal(err)
	}
	u, err := Prepare(path)
	if err != nil {
		t.Fatal(err)
	}
	return u
}
func transferPair(t *testing.T, a, b *Engine) {
	t.Helper()
	offer, err := a.Offer(upload(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := b.Handle(offer); err != nil {
		t.Fatal(err)
	}
	accepted, err := b.Accept(true)
	if err != nil {
		t.Fatal(err)
	}
	_, next, err := a.Handle(accepted)
	if err != nil || !next {
		t.Fatal("no next chunk", err)
	}
	for next {
		chunk, err := a.NextChunk()
		if err != nil {
			t.Fatal(err)
		}
		responses, _, err := b.Handle(chunk)
		if err != nil {
			t.Fatal(err)
		}
		replies, again, err := a.Handle(responses[0])
		if err != nil {
			t.Fatal(err)
		}
		next = again
		if !next {
			if a.Snapshot().State == "verified by peer" {
				t.Fatal("sender marked verified prematurely")
			}
			verified, _, err := b.Handle(replies[0])
			if err != nil {
				t.Fatal(err)
			}
			if !b.Snapshot().Unsaved {
				t.Fatal("missing unsaved state")
			}
			if _, _, err := a.Handle(verified[0]); err != nil {
				t.Fatal(err)
			}
		}
	}
}
func TestRepeatedTransferAndSave(t *testing.T) {
	a, b := &Engine{}, &Engine{}
	defer a.Close()
	defer b.Close()
	for i := 0; i < 2; i++ {
		transferPair(t, a, b)
		path := filepath.Join(t.TempDir(), "saved.dat")
		saved, err := b.Save(path, false)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := a.Handle(saved); err != nil {
			t.Fatal(err)
		}
		if a.Snapshot().Busy || b.Snapshot().Busy {
			t.Fatal("completed transfer still busy")
		}
		info, _ := os.Stat(path)
		if info.Size() != ChunkSize+15 {
			t.Fatal("wrong file contents")
		}
	}
}
func TestBusyOfferAndCancel(t *testing.T) {
	a, b := &Engine{}, &Engine{}
	defer a.Close()
	defer b.Close()
	offer, _ := a.Offer(upload(t))
	_, _, _ = b.Handle(offer)
	responses, _, err := b.Handle(offer)
	if err != nil || len(responses) != 1 || responses[0].Type != TypeFileReject {
		t.Fatal("busy offer not rejected")
	}
	cancel := b.Cancel()
	_, _, _ = a.Handle(cancel)
	if a.Snapshot().Busy || b.Snapshot().Busy {
		t.Fatal("cancel did not reset")
	}
}
func TestTransferTimeoutAndRetry(t *testing.T) {
	e := &Engine{}
	defer e.Close()
	_, _ = e.Offer(upload(t))
	if _, expired := e.Expire(time.Now().Add(Timeout + time.Second)); !expired {
		t.Fatal("missing timeout")
	}
	if e.Snapshot().Busy {
		t.Fatal("timeout left busy")
	}
	if _, err := e.Offer(upload(t)); err != nil {
		t.Fatal(err)
	}
}
func TestTransferIntegrityFailure(t *testing.T) {
	a, b := &Engine{}, &Engine{}
	defer a.Close()
	defer b.Close()
	offer, _ := a.Offer(upload(t))
	_, _, _ = b.Handle(offer)
	accept, _ := b.Accept(true)
	_, _, _ = a.Handle(accept)
	for i := 0; i < offer.Chunks; i++ {
		chunk, _ := a.NextChunk()
		chunk.Data[0] ^= 1
		acks, _, err := b.Handle(chunk)
		if err != nil {
			t.Fatal(err)
		}
		done, _, _ := a.Handle(acks[0])
		if i == offer.Chunks-1 {
			cancel, _, err := b.Handle(done[0])
			if err == nil || len(cancel) != 1 {
				t.Fatal("corruption accepted")
			}
			_, _, _ = a.Handle(cancel[0])
		}
	}
	if b.Snapshot().Unsaved || b.Snapshot().Busy {
		t.Fatal("failed receive buffer retained")
	}
}
func TestSaveOverwriteAndPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file.dat")
	if err := os.WriteFile(path, []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := SaveFile(path, []byte("new")); !errors.Is(err, ErrExists) {
		t.Fatal("overwrite without confirmation", err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "original" {
		t.Fatal("original modified")
	}
	if err := SaveFileWithOverwrite(path, []byte("new"), true); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(path)
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatal("unsafe permissions", info.Mode())
	}
	files, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".aegis-save-*"))
	if len(files) != 0 {
		t.Fatal("staging file leaked")
	}
}
func TestSaveSymlinkAndRetry(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink privileges vary")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	_ = os.WriteFile(target, []byte("original"), 0600)
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := SaveFileWithOverwrite(link, []byte("new"), true); err == nil {
		t.Fatal("symlink accepted")
	}
	data, _ := os.ReadFile(target)
	if string(data) != "original" {
		t.Fatal("symlink target overwritten")
	}
	a, b := &Engine{}, &Engine{}
	defer a.Close()
	defer b.Close()
	transferPair(t, a, b)
	if _, err := b.Save(link, true); err == nil {
		t.Fatal("save unexpectedly succeeded")
	}
	if !b.Snapshot().Unsaved {
		t.Fatal("failed save lost buffer")
	}
	if _, err := b.Save(filepath.Join(dir, "retry"), false); err != nil {
		t.Fatal(err)
	}
}
func TestInvalidPackets(t *testing.T) {
	for _, f := range []AegisFrame{{Type: "unknown"}, {Type: TypeChunk, TransferID: "bad"}, {Type: TypeMsg, Index: -1}, {Type: TypeFileOffer, TransferID: "00000000000000000000000000000000", Size: 1, Chunks: 8}} {
		if f.Validate() == nil {
			t.Fatal("invalid packet accepted", f.Type)
		}
	}
}

func TestSaveAndSnapshotConcurrent(t *testing.T) {
	a, b := &Engine{}, &Engine{}
	defer a.Close()
	defer b.Close()
	transferPair(t, a, b)
	done := make(chan error, 1)
	go func() { _, err := b.Save(filepath.Join(t.TempDir(), "saved.dat"), false); done <- err }()
	for i := 0; i < 100; i++ {
		_ = b.Snapshot()
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if b.Snapshot().Current != b.Snapshot().Total {
		t.Fatal("completed progress was lost")
	}
}
