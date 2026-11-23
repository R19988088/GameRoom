package app

import (
	"testing"
	"time"
)

// TestRewindPlanFollowsTheStateItWrites: how often a state is taken is decided by
// what one costs to write and how big it is, never by the clock alone. A core
// whose state is 22KB can give one every frame; the 16MB of a PSX core can hold
// eight in the row's 128MB whatever the span says; a state that costs 6ms is taken
// once every three frames.
func TestRewindPlanFollowsTheStateItWrites(t *testing.T) {
	step, max := rewindPlan(22037, 60, 0)
	if step != 1 {
		t.Fatalf("a 22KB state is captured every %d frames, want every frame", step)
	}
	if max != 120 {
		t.Fatalf("the row holds %d captures at 60fps, want two seconds (120)", max)
	}

	step, max = rewindPlan(16<<20, 60, 0)
	if step != 1 || max != 8 {
		t.Fatalf("a 16MB state gives step %d and %d captures, want 1 and 8 for 128MB", step, max)
	}

	step, max = rewindPlan(826458, 60, 6*time.Millisecond)
	if step != 3 {
		t.Fatalf("a state that costs 6ms is captured every %d frames, want every 3", step)
	}
	if max != 40 {
		t.Fatalf("120 frames at a 3-frame step is %d captures, want 40", max)
	}

	if step, max = rewindPlan(0, 60, 0); step != 0 || max != 0 {
		t.Fatalf("a core with no state plans %d/%d, want 0/0", step, max)
	}
}

// TestRewindRowStepsBackThroughTheStates: the row keeps the newest captures, the
// timeline steps back one at a time and reports how far back it is in frames, and
// walking forward reaches the newest one and stops there.
func TestRewindRowStepsBackThroughTheStates(t *testing.T) {
	var r rewind
	r.reset()
	r.mu.Lock()
	r.max, r.planned = 3, true
	r.mu.Unlock()
	for i := 0; i < 5; i++ {
		r.mu.Lock()
		r.add(i*10, []byte{byte(i)})
		r.mu.Unlock()
	}
	v := r.view()
	if v.Count != 3 {
		t.Fatalf("the row keeps %d captures, want the plan's 3", v.Count)
	}
	if v.Span != 20 {
		t.Fatalf("the row covers %d frames, want 20 (frames 20 to 40)", v.Span)
	}
	if v.Idx != -1 || v.Back != 0 {
		t.Fatalf("at the live edge idx is %d and back %d, want -1 and 0", v.Idx, v.Back)
	}

	e, ok := r.back()
	if !ok || e.Frame != 40 {
		t.Fatalf("the first step back answers frame %d, want the newest, 40", e.Frame)
	}
	e, ok = r.back()
	if !ok || e.Frame != 30 {
		t.Fatalf("the second step back answers frame %d, want 30", e.Frame)
	}
	if v = r.view(); v.Back != 10 {
		t.Fatalf("two steps back is %d frames, want 10", v.Back)
	}
	e, ok = r.back()
	if !ok || e.Frame != 20 {
		t.Fatalf("the third step back answers frame %d, want the oldest, 20", e.Frame)
	}
	if v = r.view(); v.Back != 20 {
		t.Fatalf("three steps back is %d frames, want 20", v.Back)
	}
	if e, _ = r.forward(); e.Frame != 30 {
		t.Fatalf("forward answers frame %d, want 30", e.Frame)
	}
	for i := 0; i < 3; i++ {
		e, _ = r.forward()
	}
	if e.Frame != 40 {
		t.Fatalf("forward ends at frame %d, want the newest, 40", e.Frame)
	}
	if _, ok := r.jump(9); ok {
		t.Fatalf("jump past the row answered a state")
	}
	if e, ok := r.jump(1); !ok || e.Frame != 30 {
		t.Fatalf("jump to 1 answered frame %d, want 30", e.Frame)
	}
}

// TestRewindFreezeHoldsTheRow: while the panel is open the row does not grow, and
// closing it empties both the row and the place in it.
func TestRewindFreezeHoldsTheRow(t *testing.T) {
	var r rewind
	r.reset()
	r.mu.Lock()
	r.max, r.planned, r.step = 8, true, 1
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
