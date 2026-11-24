package app

import (
	"math"
	"sync"
	"time"

	"github.com/R19988088/GameRoom/internal/core"
)

// A rewind is a row of states: the machine as it was, one capture every segment
// of the game, the newest at the end. The timeline is measured in tenths of a
// second, not in frames — a capture is 100ms of play (200ms, 300ms when the core
// is too big to be caught up with that often) — so the row reads the same however
// fast the machine runs. What is kept is bounded by bytes as well, so a heavy
// core is never asked to hold a gigabyte of DMA.
const (
	rewindBytes      = 128 << 20
	rewindSpan       = 2 * time.Second
	rewindSegment    = 100 * time.Millisecond
	rewindCostBudget = 2 * time.Millisecond
	rewindMaxEntries = 240
	// A core whose state is expensive still deserves a row long enough to step
	// through: ten segments of history, as far as the byte budget allows.
	rewindMinEntries = 10
	// Past these, a row would cost the game more than it gives back: a state
	// that takes a fifth of a second to write stops the machine visibly, and
	// one over half the byte budget leaves a row of two. Those cores still get
	// the nine save slots — the timeline is what is left out, not saving.
	rewindCostCeiling = 300 * time.Millisecond
	rewindMaxState    = rewindBytes / 2
)

// rewindEntry is one captured state and the frame it was taken on.
type rewindEntry struct {
	Frame int
	Data  []byte
}

// rewindPlan says how a core's states fit into the row: one capture every step
// frames and max of them — about two seconds of history, cut into segments of
// 100ms. A state that costs more than the budget cannot be taken that often, so
// the segment grows to the next tenth of a second rather than to whatever the
// cost works out to: the timeline always reads in round hundreds of
// milliseconds, and a PlayStation, whose state takes 10–15ms to write, steps back
// 200ms at a time instead of 133. When a state is so big that two seconds would
// be two captures, the row still keeps rewindMinEntries of them if the byte
// budget allows, so a heavy core has a timeline instead of a single step.
func rewindPlan(size int, fps float64, cost time.Duration) (step, max int) {
	if size <= 0 {
		return 0, 0
	}
	// A state that is enormous, or slow to be written, cannot fill a row that
	// anyone would step through: the whole row would be two snapshots, and each
	// one stops the machine. No row at all is the honest answer; saving still
	// works through the panel.
	if size > rewindMaxState || cost > rewindCostCeiling {
		return 0, 0
	}
	if fps <= 0 {
		fps = 60
	}
	// One segment is this many frames of this machine: 6 at 60fps, 5 at 50.
	seg := int(math.Round(fps * rewindSegment.Seconds()))
	if seg < 1 {
		seg = 1
	}
	step = seg
	if cost > rewindCostBudget {
		if byCost := int(math.Ceil(float64(cost) / float64(rewindCostBudget))); byCost > step {
			// Round up to whole segments, so every step of the timeline is a
			// round tenth of a second.
			step = int(math.Ceil(float64(byCost)/float64(seg))) * seg
		}
	}
	span := int(rewindSpan.Seconds() * fps)
	if span <= 0 {
		span = 60
	}
	max = span / step
	// A core that is slow to be caught (Citra, a Dreamcast) would otherwise keep
	// two captures and call it a timeline. Ten is the floor; the byte budget below
	// is what actually stops the row from growing when a state is enormous.
	if max < rewindMinEntries {
		max = rewindMinEntries
	}
	if byBytes := int(rewindBytes / int64(size)); byBytes < max {
		max = byBytes
	}
	if max > rewindMaxEntries {
		max = rewindMaxEntries
	}
	if max < 2 {
		// Two snapshots a second apart are not a timeline; the plan answers
		// "none" and the panel says why.
		return 0, 0
	}
	return step, max
}

// rewind is the row the panel steps back through. The core's own thread adds to
// it after every stepped frame; the window's thread reads it while the panel is
// open, which is why it carries a lock of its own.
type rewind struct {
	mu      sync.Mutex
	step    int     // frames between captures, 0 when the core has no state
	max     int     // how many captures are kept
	fps     float64 // the machine's rate, for reading frames as milliseconds
	planned bool
	last    int // the frame of the newest capture, -1 for none
	idx     int // the entry the picture is at, -1 for the live edge
	frozen  bool
	note    string
	// noState is a core that has no save state at all. A core whose state is
	// merely too big or too slow to keep a row is still savable: the panel's
	// slots work, only the timeline is left out.
	noState bool
	entries []rewindEntry
	cost    time.Duration
}

// reset empties the row and forgets the plan: a new game plans it again.
func (r *rewind) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.step, r.max, r.planned = 0, 0, false
	r.last, r.idx = -1, -1
	r.frozen = false
	r.entries = nil
	r.note = ""
	r.noState = false
	r.cost = 0
	r.fps = 0
}

// ms is the game's time for a number of its frames, as the timeline reads it.
func (r *rewind) ms(frames int) int {
	fps := r.fps
	if fps <= 0 {
		fps = 60
	}
	return int(math.Round(float64(frames) * 1000 / fps))
}

// freeze stops the row from growing while the panel is open, so the states it
// shows are the ones the game really had and not a row that moves under the
// pointer.
func (r *rewind) freeze(on bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.frozen = on
	r.idx = -1
}

// trim drops the oldest captures until the row fits the plan.
func (r *rewind) trim() {
	for len(r.entries) > r.max && len(r.entries) > 0 {
		r.entries = r.entries[1:]
		if r.idx > 0 {
			r.idx--
		}
	}
}

// add puts one state at the end of the row.
func (r *rewind) add(frame int, data []byte) {
	r.entries = append(r.entries, rewindEntry{Frame: frame, Data: data})
	r.trim()
}

// view is what the panel draws: how many states there are, which one the picture
// is at, how far back that is in milliseconds of play, and whether the row is live.
type rewindView struct {
	Count int
	Idx   int
	Back  int // milliseconds behind the live edge, 0 while live
	Span  int // milliseconds the whole row covers
	Seg   int // milliseconds of game in one step of the timeline
	Step  int // frames of game in one segment
	Note  string
}

func (r *rewind) view() rewindView {
	r.mu.Lock()
	defer r.mu.Unlock()
	v := rewindView{Count: len(r.entries), Idx: r.idx, Step: r.step, Note: r.note}
	if r.step > 0 {
		v.Seg = r.ms(r.step)
	}
	if len(r.entries) > 0 {
		if r.idx >= 0 && r.idx < len(r.entries) {
			v.Back = r.ms(r.entries[len(r.entries)-1].Frame - r.entries[r.idx].Frame)
		}
		// The row is counted in whole segments: twenty of them are two seconds of
		// play, even though the oldest and the newest are nineteen apart.
		if r.step > 0 {
			v.Span = r.ms(len(r.entries) * r.step)
		} else if len(r.entries) > 1 {
			v.Span = r.ms(r.entries[len(r.entries)-1].Frame - r.entries[0].Frame)
		}
	}
	return v
}

// jump answers the state at i and makes the picture sit there.
func (r *rewind) jump(i int) (rewindEntry, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if i < 0 || i >= len(r.entries) {
		return rewindEntry{}, false
	}
	r.idx = i
	return r.entries[i], true
}

// back is one step towards the past: from the live edge it goes to the newest
// capture, and from there one capture further each time.
func (r *rewind) back() (rewindEntry, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.entries) == 0 {
		return rewindEntry{}, false
	}
	switch {
	case r.idx < 0:
		r.idx = len(r.entries) - 1
	case r.idx > 0:
		r.idx--
	}
	return r.entries[r.idx], true
}

// forward is one step back towards the live edge.
func (r *rewind) forward() (rewindEntry, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.idx < 0 || len(r.entries) == 0 {
		return rewindEntry{}, false
	}
	if r.idx < len(r.entries)-1 {
		r.idx++
	}
	return r.entries[r.idx], true
}

// captureRewind keeps the row. It runs on the core's own thread, right after a
// frame — the one moment the core's state is whole — and the first capture also
// pays for the measurement: what a state costs decides how often one is taken.
func (a *App) captureRewind(s *core.Session) {
	if s == nil {
		return
	}
	r := &a.rew
	n := s.Frames()
	r.mu.Lock()
	if r.frozen {
		r.mu.Unlock()
		return
	}
	if r.planned && (r.step <= 0 || n-r.last < r.step) {
		r.mu.Unlock()
		return
	}
	// Before the first snapshot, the core's own declaration says whether a row
	// is possible at all: a hundred-megabyte state would be paid for once and
	// then never kept, so it is not even written. Dolphin is the case here.
	if !r.planned {
		if sz := s.StateSizeNow(); sz > rewindMaxState {
			r.planned, r.step, r.max = true, 0, 0
			r.note = "这个核心一份存档太大，时间轴没有开；存档面板照旧可用"
			r.mu.Unlock()
			return
		}
	}
	r.last = n
	r.mu.Unlock()

	start := time.Now()
	data, err := s.SaveStateNow()
	cost := time.Since(start)

	r.mu.Lock()
	defer r.mu.Unlock()
	if err != nil {
		// A core that answered 0 has no state to give: the panel says so instead
		// of offering a row that can never be filled.
		r.step, r.max, r.planned, r.note, r.noState = 0, 0, true, err.Error(), true
		return
	}
	if !r.planned || cost > 2*rewindCostBudget {
		step, max := rewindPlan(len(data), s.FPS(), cost)
		r.step, r.max, r.fps = step, max, s.FPS()
	}
	r.planned = true
	if r.step == 0 || r.max == 0 {
		// The plan measured the state and refused it: too large to keep a row,
		// or so slow to write that every capture would be a visible stop. Saving
		// itself still works, so this is not a core without a state.
		r.note = "这个核心一份存档太慢太大，时间轴没有开；存档面板照旧可用"
		return
	}
	r.cost = cost
	r.add(n, data)
}

// rewindTo puts the game at the entry i of the row: the state goes back in, and
// one frame is let through so the glass shows it.
func (a *App) rewindTo(i int) {
	if a.core == nil {
		return
	}
	e, ok := a.rew.jump(i)
	if !ok {
		return
	}
	if err := a.core.LoadState(e.Data); err != nil {
		a.rew.mu.Lock()
		a.rew.note = err.Error()
		a.rew.mu.Unlock()
		return
	}
	a.core.Step()
}
