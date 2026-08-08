package cli

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestScrollbarThumb(t *testing.T) {
	if _, size := scrollbarThumb(10, 0, 5); size != 0 {
		t.Errorf("content within viewport should have no thumb, got size %d", size)
	}
	if start, _ := scrollbarThumb(10, 0, 100); start != 0 {
		t.Errorf("at top the thumb starts at row 0, got %d", start)
	}
	const h, total = 10, 100
	if start, size := scrollbarThumb(h, total-h, total); start+size != h {
		t.Errorf("at bottom the thumb reaches the last row: start=%d size=%d h=%d", start, size, h)
	}
}

func TestEdgeScrollDir(t *testing.T) {
	const h = 10
	if got := edgeScrollDir(0, h); got != -1 {
		t.Errorf("top edge dir = %d, want -1", got)
	}
	if got := edgeScrollDir(h-1, h); got != 1 {
		t.Errorf("bottom edge dir = %d, want 1", got)
	}
	if got := edgeScrollDir(h/2, h); got != 0 {
		t.Errorf("middle dir = %d, want 0", got)
	}
}

func TestSelSpan(t *testing.T) {
	start, end, cw := selPos{line: 1, col: 3}, selPos{line: 3, col: 5}, 20
	for _, tc := range []struct {
		idx         int
		wantOK      bool
		wantLo, wHi int
	}{
		{0, false, 0, 0}, // above
		{1, true, 3, cw}, // first line: anchor col → right edge
		{2, true, 0, cw}, // middle line: full width
		{3, true, 0, 5},  // last line: left edge → head col
		{4, false, 0, 0}, // below
	} {
		lo, hi, ok := selSpan(tc.idx, start, end, cw)
		if ok != tc.wantOK || (ok && (lo != tc.wantLo || hi != tc.wHi)) {
			t.Errorf("selSpan(%d) = (%d,%d,%v), want (%d,%d,%v)", tc.idx, lo, hi, ok, tc.wantLo, tc.wHi, tc.wantOK)
		}
	}
}

func TestSelectedTextMultiLine(t *testing.T) {
	m := newTestChatTUI()
	m.wrappedLines = []string{"hello world", "second line", "third row"}
	m.sel = selection{active: true, anchor: selPos{line: 0, col: 6}, head: selPos{line: 2, col: 5}}

	if got, want := m.selectedText(), "world\nsecond line\nthird"; got != want {
		t.Errorf("selectedText() = %q, want %q", got, want)
	}

	// A zero-width selection (plain click) copies nothing.
	m.sel = selection{active: true, anchor: selPos{line: 0, col: 3}, head: selPos{line: 0, col: 3}}
	if got := m.selectedText(); got != "" {
		t.Errorf("empty selection should yield no text, got %q", got)
	}
}

// TestCopySelectionCopiesAndClears proves copySelection returns a clipboard
// write command for the selected plain text and clears the highlight, so the
// transcript returns to normal input afterwards.
func TestCopySelectionCopiesAndClears(t *testing.T) {
	m := newTestChatTUI()
	m.wrappedLines = []string{"hello world", "second line"}
	m.sel = selection{active: true, anchor: selPos{line: 0, col: 6}, head: selPos{line: 1, col: 6}}

	got, cmd := m.copySelection()
	if cmd == nil {
		t.Fatal("copySelection should return a clipboard command")
	}
	if got.sel.active || !got.sel.empty() {
		t.Errorf("selection should be cleared after copy, got %+v", got.sel)
	}
	if msg := cmd(); msg != nil {
		t.Errorf("clipboard command should return nil msg, got %v", msg)
	}

	// An empty selection returns no command and stays cleared.
	m.sel = selection{active: true, anchor: selPos{line: 0, col: 3}, head: selPos{line: 0, col: 3}}
	got, cmd = m.copySelection()
	if cmd != nil {
		t.Errorf("empty selection should return no command, got %v", cmd)
	}
	if got.sel.active {
		t.Errorf("empty selection should be cleared, got %+v", got.sel)
	}
}

// TestYKeyCopiesSelectionAndClears proves pressing y while a transcript
// selection is live copies it (vim-style yank) instead of dismissing the
// selection or typing into the input.
func TestYKeyCopiesSelectionAndClears(t *testing.T) {
	m := newTestChatTUI()
	m.wrappedLines = []string{"hello world", "second line"}
	m.sel = selection{active: true, anchor: selPos{line: 0, col: 6}, head: selPos{line: 1, col: 6}}

	model, cmd := m.update(tea.KeyPressMsg{Code: 'y'})
	if cmd == nil {
		t.Fatal("y on a live selection should copy")
	}
	if next := model.(chatTUI); next.sel.active || !next.sel.empty() {
		t.Errorf("selection should be cleared after y, got %+v", next.sel)
	}

	// Any other key still dismisses the selection without copying.
	m.sel = selection{active: true, anchor: selPos{line: 0, col: 0}, head: selPos{line: 0, col: 5}}
	model, cmd = m.update(tea.KeyPressMsg{Code: 'x'})
	if cmd != nil {
		t.Errorf("non-copy key should not emit a command, got %v", cmd)
	}
	if next := model.(chatTUI); next.sel.active || !next.sel.empty() {
		t.Errorf("non-copy key should dismiss the selection, got %+v", next.sel)
	}
}
