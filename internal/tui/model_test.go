package tui

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"github.com/aegis-p2p/aegis/internal/transfer"
	tea "github.com/charmbracelet/bubbletea"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func rootModel(t *testing.T, m tea.Model) Model {
	t.Helper()
	switch v := m.(type) {
	case Model:
		return v
	case *Model:
		return *v
	default:
		t.Fatalf("unexpected model %T", m)
		return Model{}
	}
}
func TestNameEntryAndAdvancedLength(t *testing.T) {
	m := NewModel("", "", false)
	m.dashboard.input.SetValue("ab")
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = rootModel(t, next)
	if cmd != nil || m.dashboard.errorText == "" {
		t.Fatal("invalid name initialized")
	}
	m.dashboard.phase = phaseIdle
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyF3})
	m = rootModel(t, next)
	long := string(bytes.Repeat([]byte("x"), 500))
	m.dashboard.input.SetValue(long)
	if m.dashboard.input.Value() != long {
		t.Fatal("advanced address truncated")
	}
}
func TestSearchCancellationIgnoresLateResult(t *testing.T) {
	m := NewModel("", "", false)
	ctx, cancel := context.WithCancel(context.Background())
	m.operationCancel = cancel
	m.operation = 2
	m.dashboard.phase = phaseSearching
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = rootModel(t, next)
	if ctx.Err() == nil || m.dashboard.phase != phaseIdle {
		t.Fatal("search not cancelled")
	}
	next, _ = m.Update(searchResult{Operation: 2, Err: context.DeadlineExceeded})
	m = rootModel(t, next)
	if m.dashboard.errorText != "" {
		t.Fatal("stale search changed dashboard")
	}
}
func TestHistoryBoundAndScrollPosition(t *testing.T) {
	chat := NewChat("", nil)
	chat, _ = chat.UpdateSize(80, 25)
	for i := 0; i < 1100; i++ {
		chat.addSystem("history message")
	}
	if len(chat.history) != 1000 {
		t.Fatal("unbounded history")
	}
	chat.viewport.SetYOffset(5)
	chat.addSystem("new message")
	if chat.viewport.YOffset != 5 {
		t.Fatal("new message interrupted reading history")
	}
}
func verifiedEngine(t *testing.T) *transfer.Engine {
	t.Helper()
	e := &transfer.Engine{}
	data := []byte("unsaved file")
	hash := sha256.Sum256(data)
	id := "00112233445566778899aabbccddeeff"
	offer := transfer.AegisFrame{Type: transfer.TypeFileOffer, TransferID: id, Name: "test.dat", Size: int64(len(data)), Chunks: 1, SHA256: hex.EncodeToString(hash[:])}
	if _, _, err := e.Handle(offer); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Accept(true); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.Handle(transfer.AegisFrame{Type: transfer.TypeChunk, TransferID: id, Data: data}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.Handle(transfer.AegisFrame{Type: transfer.TypeFileDone, TransferID: id, SHA256: offer.SHA256}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(e.Close)
	return e
}
func TestUnsavedCloseRequiresChoice(t *testing.T) {
	m := NewModel("", "", false)
	m.screen = ScreenChat
	m.chat = NewChat("", nil)
	m.chat.engine = verifiedEngine(t)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = rootModel(t, next)
	if cmd != nil || !m.chat.closePrompt || !m.chat.engine.Snapshot().Unsaved {
		t.Fatal("unsaved file discarded without choice")
	}
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd == nil {
		t.Fatal("discard did not close")
	}
	m = rootModel(t, next)
	if m.chat.engine.Snapshot().Unsaved {
		t.Fatal("discard retained file")
	}
}
func TestUnsavedStayAndSaveRetry(t *testing.T) {
	m := NewModel("", "", false)
	m.screen = ScreenChat
	m.chat = NewChat("", nil)
	m.chat.engine = verifiedEngine(t)
	next, _ := m.Update(closeRequestMsg{})
	m = rootModel(t, next)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = rootModel(t, next)
	if m.closing || m.chat.closePrompt {
		t.Fatal("stay did not cancel close")
	}
	m.chat.finishAfterSave = true
	var cmd tea.Cmd
	m.chat, cmd = m.chat.Update(saveResult{Err: os.ErrPermission})
	if cmd != nil || !m.chat.closePrompt || !m.chat.engine.Snapshot().Unsaved {
		t.Fatal("save failure lost pending file")
	}
}
func TestStalePreparationClosesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file")
	_ = os.WriteFile(path, []byte("payload"), 0600)
	u, err := transfer.Prepare(path)
	if err != nil {
		t.Fatal(err)
	}
	chat := NewChat("", nil)
	chat.prepareGeneration = 2
	chat, _ = chat.Update(preparedFile{Generation: 1, Upload: u})
	if chat.engine.Snapshot().Busy {
		t.Fatal("stale preparation started transfer")
	}
}
func TestNoDiskLogs(t *testing.T) {
	t.Chdir(t.TempDir())
	SetDebug(false)
	LogDebug("transfer failed", "private-file")
	SetDebug(true)
	LogDebug("transfer failed", "private-file")
	SetDebug(false)
	if _, err := os.Stat("tmp/debug.log"); !os.IsNotExist(err) {
		t.Fatal("disk log created")
	}
}
func TestTransferTimeoutReturnsToIdle(t *testing.T) {
	chat := NewChat("", nil)
	offer := transfer.AegisFrame{Type: transfer.TypeFileOffer, TransferID: "00112233445566778899aabbccddeeff", Name: "x", Size: 1, Chunks: 1, SHA256: hex.EncodeToString(make([]byte, 32))}
	_, _, _ = chat.engine.Handle(offer)
	_, expired := chat.engine.Expire(time.Now().Add(transfer.Timeout + time.Second))
	if !expired || chat.engine.Snapshot().Busy {
		t.Fatal("offer did not expire")
	}
}
