package core

import (
	"fmt"
	"os"
	"syscall"
	"time"
	"unsafe"
)

var debug = os.Getenv("GAMEROOM_DEBUG") != ""

// A game's sound is played through winmm's wave output: the samples the core
// makes are queued a few frames ahead of when they are heard, which is all a
// game needs and what any Windows machine has.

var (
	winmm             = syscall.NewLazyDLL("winmm.dll")
	procWaveOutOpen   = winmm.NewProc("waveOutOpen")
	procWaveOutWrite  = winmm.NewProc("waveOutWrite")
	procWaveOutPrep   = winmm.NewProc("waveOutPrepareHeader")
	procWaveOutUnprep = winmm.NewProc("waveOutUnprepareHeader")
	procWaveOutReset  = winmm.NewProc("waveOutReset")
	procWaveOutClose  = winmm.NewProc("waveOutClose")
)

type waveFormatEx struct {
	FormatTag      uint16
	Channels       uint16
	SamplesPerSec  uint32
	AvgBytesPerSec uint32
	BlockAlign     uint16
	BitsPerSample  uint16
	Size           uint16
}

type waveHdr struct {
	Data     *byte
	Length   uint32
	Recorded uint32
	User     uintptr
	Flags    uint32
	Loops    uint32
	Next     *waveHdr
	Reserved uintptr
}

const (
	waveMapper = 0xFFFFFFFF
	whdrDone   = 0x00000001

	// The buffers are short, so that sound follows the picture closely, and
	// there are several, so that one being refilled never leaves a gap. A
	// machine that stalls for a moment (a filter, a scene change) still has
	// room to catch up before the output runs dry.
	soundFrames  = 1024
	soundBuffers = 8
)

// StartSound plays the core's sound until Close. A core that makes none plays
// on, silent.
func (s *Session) StartSound() {
	if s.audio == nil {
		return
	}
	// Diagnostic: GAMEROOM_NOAUDIO leaves the core's audio_ready unstarted, so a
	// crash that only appears with sound can be told apart from a drawing fault.
	if os.Getenv("GAMEROOM_NOAUDIO") != "" {
		return
	}
	s.sndOnce.Do(func() { go s.soundLoop() })
}

func (s *Session) soundLoop() {
	defer close(s.sndDone)

	format := waveFormatEx{
		FormatTag:      1, // PCM
		Channels:       2,
		SamplesPerSec:  uint32(s.audio.SampleRate()),
		AvgBytesPerSec: uint32(s.audio.SampleRate() * 4),
		BlockAlign:     4,
		BitsPerSample:  16,
	}
	var hwo uintptr
	if r, _, _ := procWaveOutOpen.Call(uintptr(unsafe.Pointer(&hwo)), waveMapper,
		uintptr(unsafe.Pointer(&format)), 0, 0, 0); r != 0 {
		if debug {
			fmt.Fprintf(os.Stderr, "sound: waveOutOpen failed err=%d\n", r)
		}
		return // no output device: the game plays on, silent
	}
	if debug {
		fmt.Fprintf(os.Stderr, "sound: waveOut open rate=%d\n", format.SamplesPerSec)
	}
	defer procWaveOutClose.Call(hwo)

	bufs := make([][]byte, soundBuffers)
	for i := range bufs {
		bufs[i] = make([]byte, soundFrames*4)
	}
	hdrs := make([]waveHdr, soundBuffers)
	samples := make([]float32, soundFrames*2)
	hdrSize := uintptr(unsafe.Sizeof(hdrs[0]))

	fill := func(i int) {
		n := 0
		for n < len(samples) {
			c, err := s.audio.Read(samples[n:])
			n += c
			if err != nil {
				break
			}
		}
		buf := bufs[i]
		for f := 0; f < soundFrames; f++ {
			for c := 0; c < 2; c++ {
				v := samples[f*2+c]
				if v > 0.999 {
					v = 0.999
				} else if v < -0.999 {
					v = -0.999
				}
				p := int16(v * 32767)
				buf[f*4+c*2] = byte(p)
				buf[f*4+c*2+1] = byte(p >> 8)
			}
		}
	}

	queue := func(i int) bool {
		hdrs[i] = waveHdr{Data: &bufs[i][0], Length: soundFrames * 4}
		if r, _, _ := procWaveOutPrep.Call(hwo, uintptr(unsafe.Pointer(&hdrs[i])), hdrSize); r != 0 {
			if debug {
				fmt.Fprintf(os.Stderr, "sound: prepare failed err=%d\n", r)
			}
			return false
		}
		if r, _, _ := procWaveOutWrite.Call(hwo, uintptr(unsafe.Pointer(&hdrs[i])), hdrSize); r != 0 {
			if debug {
				fmt.Fprintf(os.Stderr, "sound: write failed err=%d\n", r)
			}
			return false
		}
		return true
	}

	for i := 0; i < soundBuffers; i++ {
		fill(i)
		if !queue(i) {
			return
		}
	}

	// Every time a buffer has been heard, it is filled with the sound made
	// since and queued behind the others: five always stand between the core
	// and the speakers.
	t0 := time.Now()
	for i := 0; ; i = (i + 1) % soundBuffers {
		for {
			select {
			case <-s.quit:
				procWaveOutReset.Call(hwo)
				return
			default:
			}
			if hdrs[i].Flags&whdrDone != 0 {
				break
			}
			time.Sleep(2 * time.Millisecond)
		}
		procWaveOutUnprep.Call(hwo, uintptr(unsafe.Pointer(&hdrs[i])), hdrSize)
		fill(i)
		if !queue(i) {
			return
		}
		if debug {
			played++
			if played%200 == 0 {
				el := time.Since(t0)
				fmt.Fprintf(os.Stderr, "sound: buffers played=%d in %dms (%.1f/s, %.0f frames/s) dropped=%d\n",
					played, el.Milliseconds(), float64(played)/el.Seconds(),
					float64(played)*float64(soundFrames)/el.Seconds(), s.audio.Dropped())
			}
		}
	}
}

var played int
