package app

import (
	"testing"
	"time"
)

// TestRewindPlanCutsInTenthsOfASecond: one capture is 100ms of the game, not one
// frame — 6 frames at 60fps, 5 at 50 — and the row covers two seconds of them. A
// state that costs too much to be taken every tenth of a second grows the segment
// to the next tenth (200ms), never to a ragged 133. A state big enough to fill the
// row's memory is held to what fits instead.
func TestRewindPlanCutsInTenthsOfASecond(t *testing.T) {
	step, max := rewindPlan(22037, 60, 0)
	if step != 6 {
		t.Fatalf("a 60fps core steps in %d frames, want 6 (100ms)", step)
	}
	if max != 20 {
		t.Fatalf("the row holds %d captures, want two seconds of 100ms (20)", max)
	}

	if step, max = rewindPlan(22037, 50, 0); step != 5 || max != 20 {
		t.Fatalf("a 50fps core steps in %d frames for %d captures, want 5 and 20", step, max)
	}

	step, max = rewindPlan(16<<20, 60, 0)
	if step != 6 || max != 8 {
		t.Fatalf("a 16MB state gives step %d and %d captures, want 6 and 8 for 128MB", step, max)
	}

	// 6ms is over the budget but still under one segment: the step stays 100ms.
	if step, max = rewindPlan(826458, 60, 6*time.Millisecond); step != 6 || max != 20 {
		t.Fatalf("a state that costs 6ms stepped %d frames for %d captures, want the 100ms segment", step, max)
	}

	// 15ms cannot be written every 100ms, so the segment is 200ms: 12 frames, and
	// the two seconds hold ten of them.
	if step, max = rewindPlan(3751935, 60, 15*time.Millisecond); step != 12 || max != 10 {
		t.Fatalf("a state that costs 15ms stepped %d frames for %d captures, want 12 and 10", step, max)
	}

	if step, max = rewindPlan(0, 60, 0); step != 0 || max != 0 {
		t.Fatalf("a core with no state plans %d/%d, want 0/0", step, max)
	}
}

// TestRewindRowStepsBackThroughTheStates: the row keeps the newest captures, the
// timeline steps back one segment at a time and reads how far back it is in
// milliseconds, and walking forward reaches the newest one and stops there.
func TestRewindRowStepsBackThroughTheStates(t *testing.T) {
	var r rewind
	r.reset()
	r.mu.Lock()
	r.max, r.planned, r.step, r.fps = 3, true, 6, 60
	r.mu.Unlock()
	for i := 1; i <= 3; i++ {
		r.mu.Lock()
		r.add(i*6, []byte{byte(i)})
		r.mu.Unlock()
	}
	v := r.view()
	if v.Count != 3 {
		t.Fatalf("the row keeps %d captures, want the plan's 3", v.Count)
	}
	if v.Seg != 100 {
		t.Fatalf("one segment is %d ms, want 100", v.Seg)
	}
	if v.Span != 300 {
		t.Fatalf("the row covers %d ms, want 300 (three segments of 100)", v.Span)
	}
	if v.Idx != -1 || v.Back != 0 {
		t.Fatalf("at the live edge idx is %d and back %d ms, want -1 and 0", v.Idx, v.Back)
	}

	e, ok := r.back()
	if !ok || e.Frame != 18 {
		t.Fatalf("the first step back answers frame %d, want the newest, 18", e.Frame)
	}
	if e, ok = r.back(); !ok || e.Frame != 12 {
		t.Fatalf("the second step back answers frame %d, want 12", e.Frame)
	}
	if v = r.view(); v.Back != 100 {
		t.Fatalf("one segment back is %d ms, want 100", v.Back)
	}
	if e, ok = r.back(); !ok || e.Frame != 6 {
		t.Fatalf("the third step back answers frame %d, want the oldest, 6", e.Frame)
	}
	if v = r.view(); v.Back != 200 {
		t.Fatalf("two segments back is %d ms, want 200", v.Back)
	}
	if e, _ = r.forward(); e.Frame != 12 {
		t.Fatalf("forward answers frame %d, want 12", e.Frame)
	}
	for i := 0; i < 3; i++ {
		e, _ = r.forward()
	}
	if e.Frame != 18 {
		t.Fatalf("forward ends at frame %d, want the newest, 18", e.Frame)
	}
	if _, ok := r.jump(9); ok {
		t.Fatalf("jump past the row answered a state")
	}
	if e, ok := r.jump(1); !ok || e.Frame != 12 {
		t.Fatalf("jump to 1 answered frame %d, want 12", e.Frame)
	}
}

// TestRewindFreezeHoldsTheRow: while the panel is open the row does not grow, and
// closing it empties both the row and the place in it.
func TestRewindFreezeHoldsTheRow(t *testing.T) {
	var r rewind
	r.reset()
	r.mu.Lock()
	r.max, r.planned, r.step = 8, true, 6
	r.add(10, []byte{1})
	r.mu.Unlock()
	r.freeze(true)
	r.mu.Lock()
	frozen := r.frozen
	r.mu.Unlock()
	if !frozen {
		t.Fatalf("freeze left the row open")
	}
	r.reset()
	r.mu.Lock()
	frozen, count := r.frozen, len(r.entries)
	r.mu.Unlock()
	if frozen || count != 0 {
		t.Fatalf("reset left %d captures and frozen=%v, want an empty open row", count, frozen)
	}
}
