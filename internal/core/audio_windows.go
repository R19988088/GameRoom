package core

import (
	"io"
	"sync"
	"time"
)

// AudioPipe carries the sound a core makes into the app's audio output. The
// core hands over samples as fast as it runs; the output asks for them on its
// own clock, so the samples wait in a short queue and the output never blocks
// on the core for longer than a frame.
//
// It is a mygo audio.Decoder: the app hands it to audio.NewPlayer and the
// mixer plays it like any other sound an app generates.
type AudioPipe struct {
	mu      sync.Mutex
	samples []float32 // interleaved left, right
	rate    int
	closed  bool
	pushed  int64
	dropped int64
	reads   int64
	loudest float32
}

// Pushed is how many samples the core has made, so a person can tell a silent
// game from a silent output.
func (p *AudioPipe) Pushed() int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.pushed
}

// Dropped is how many samples were thrown away because the output fell far
// behind; a run where this keeps growing is one where the sound is being cut
// short to keep up.
func (p *AudioPipe) Dropped() int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.dropped
}

// Reads is how often the output has asked for sound.
func (p *AudioPipe) Reads() int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.reads
}

// Loudest is the largest sample the core has made since the last call, so the
// difference between a game with no sound and an output with none is visible.
func (p *AudioPipe) Loudest() float32 {
	p.mu.Lock()
	defer p.mu.Unlock()
	v := p.loudest
	p.loudest = 0
	return v
}

// NewAudioPipe is a pipe for a core that plays at rate frames per second.
func NewAudioPipe(rate int) *AudioPipe {
	if rate < 8000 || rate > 192000 {
		rate = 44100
	}
	return &AudioPipe{rate: rate}
}

// SampleRate is the frames per second the core plays at.
func (p *AudioPipe) SampleRate() int { return p.rate }

// Channels is two: every core this app runs plays stereo.
func (p *AudioPipe) Channels() int { return 2 }

// Len is unknown: the sound has no end while the game runs.
func (p *AudioPipe) Len() int64 { return -1 }

// SetPosition is a seek, which a running game has nothing to do with.
func (p *AudioPipe) SetPosition(int64) error { return nil }

// push takes a batch of samples as the core gives them: signed 16-bit,
// interleaved left and right.
func (p *AudioPipe) push(samples []int16) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	for _, v := range samples {
		f := float32(v) / 32768
		p.samples = append(p.samples, f)
		if f < 0 {
			f = -f
		}
		if f > p.loudest {
			p.loudest = f
		}
	}
	p.pushed += int64(len(samples))
	// A slow output must not let the queue grow without bound: a quarter of a
	// second of sound rides out a stutter, and anything older than that is
	// already late enough that the ear hears it as a lag behind the picture.
	// Two values per frame, so a quarter second is rate/2 of them.
	if max := p.rate / 2; len(p.samples) > max {
		p.dropped += int64(len(p.samples) - max)
		p.samples = p.samples[len(p.samples)-max:]
	}
}

// Read hands the next samples to the output. When the core has not made any
// yet it waits a few milliseconds, then fills with silence so the output keeps
// its own clock and the game is never held up by the sound.
func (p *AudioPipe) Read(buf []float32) (int, error) {
	p.mu.Lock()
	p.reads++
	p.mu.Unlock()
	for attempt := 0; attempt < 4; attempt++ {
		p.mu.Lock()
		if n := len(p.samples); n > 0 {
			copied := copy(buf, p.samples)
			p.samples = p.samples[copied:]
			p.mu.Unlock()
			return copied, nil
		}
		closed := p.closed
		p.mu.Unlock()
		if closed {
			return 0, io.EOF
		}
		time.Sleep(5 * time.Millisecond)
	}
	for i := range buf {
		buf[i] = 0
	}
	return len(buf), nil
}

// close ends the stream, so the player stops asking for more.
func (p *AudioPipe) close() {
	p.mu.Lock()
	p.closed = true
	p.samples = nil
	p.mu.Unlock()
}
