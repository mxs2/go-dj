package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/faiface/beep"
	"github.com/faiface/beep/effects"
	"github.com/faiface/beep/speaker"
	"github.com/faiface/beep/wav"
)

// --- Constants ---
const (
	AudioDir      = "./musics/"
	DefaultVolume = 0.0
	MaxVolume     = 2.0
	MinVolume     = -2.0
	BaseBPM       = 120.0
	MinSpeedRatio = 0.5
	MaxSpeedRatio = 2.0
)

// --- Type Definitions ---
type InstrumentState int

const (
	StateStopped InstrumentState = iota
	StatePlaying
	StatePaused
)

func (s InstrumentState) String() string {
	return []string{"stopped", "playing", "paused"}[s]
}

type Instrument struct {
	name       string
	streamer   beep.StreamSeekCloser
	ctrl       *beep.Ctrl
	volume     *effects.Volume
	resampler  *beep.Resampler
	state      InstrumentState
	speedRatio float64
	mu         sync.RWMutex
	file       *os.File
}

type DJMixer struct {
	instruments map[string]*Instrument
	mixer       beep.Mixer
	mu          sync.RWMutex
}

// --- Instrument Methods ---

func NewInstrument(name, filename string) (*Instrument, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", filename, err)
	}
	streamer, _, err := wav.Decode(f)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("failed to decode WAV file %s: %w", filename, err)
	}
	loopedStreamer := beep.Loop(-1, streamer)
	ctrl := &beep.Ctrl{Streamer: loopedStreamer, Paused: true}
	resampler := beep.ResampleRatio(4, 1.0, ctrl)
	volume := &effects.Volume{
		Streamer: resampler, // Volume now wraps the resampler directly
		Base:     2,
		Volume:   DefaultVolume,
		Silent:   true, // Start silently until played
	}
	return &Instrument{
		name:       name,
		streamer:   streamer,
		ctrl:       ctrl,
		volume:     volume,
		resampler:  resampler,
		state:      StateStopped,
		speedRatio: 1.0,
		file:       f,
	}, nil
}

func (i *Instrument) SetSpeed(ratio float64) error {
	if ratio < MinSpeedRatio || ratio > MaxSpeedRatio {
		return fmt.Errorf("speed ratio %.2f is out of range [%.2f, %.2f]", ratio, MinSpeedRatio, MaxSpeedRatio)
	}
	i.mu.Lock()
	i.speedRatio = ratio
	i.mu.Unlock()
	speaker.Lock()
	i.resampler.SetRatio(ratio)
	speaker.Unlock()
	currentBPM := BaseBPM * ratio
	log.Printf("🎹 Tempo for '%s' set to %.1f BPM (%.2fx).", i.name, currentBPM, ratio)
	return nil
}

func (i *Instrument) Play() error {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.state == StatePlaying {
		return fmt.Errorf("instrument '%s' is already playing", i.name)
	}
	i.volume.Silent = false // Unmute the track
	i.ctrl.Paused = false
	i.state = StatePlaying
	log.Printf("▶️  %s started playing.", i.name)
	return nil
}

func (i *Instrument) Replay() error {
	i.mu.Lock()
	defer i.mu.Unlock()
	if err := i.streamer.Seek(0); err != nil {
		return fmt.Errorf("failed to seek '%s' for replay: %w", i.name, err)
	}
	i.volume.Silent = false // Unmute the track
	i.ctrl.Paused = false
	i.state = StatePlaying
	log.Printf("🔄 %s replaying from the beginning.", i.name)
	return nil
}

func (i *Instrument) Pause() error {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.state != StatePlaying {
		return fmt.Errorf("instrument '%s' is not playing (current state: %s)", i.name, i.state)
	}
	i.ctrl.Paused = true
	i.state = StatePaused
	log.Printf("⏸️  %s paused.", i.name)
	return nil
}

func (i *Instrument) Stop() error {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.state == StateStopped {
		return nil
	}
	// Stop now mutes the track but lets it play silently in the background.
	i.volume.Silent = true
	i.state = StateStopped
	log.Printf("🔇 %s muted (stopped).", i.name)
	return nil
}

func (i *Instrument) SetVolume(vol float64) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	if vol < MinVolume || vol > MaxVolume {
		return fmt.Errorf("volume %.2f is out of the allowed range [%.2f, %.2f]", vol, MinVolume, MaxVolume)
	}
	i.volume.Volume = vol
	log.Printf("🔊 Set volume for %s to %.2f.", i.name, vol)
	return nil
}

func (i *Instrument) GetState() InstrumentState {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.state
}

func (i *Instrument) Close() error {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.file.Close()
}

// --- DJMixer Methods ---

func NewDJMixer() *DJMixer {
	return &DJMixer{
		instruments: make(map[string]*Instrument),
	}
}

func (dj *DJMixer) AddInstrument(name, filepath string) error {
	dj.mu.Lock()
	defer dj.mu.Unlock()
	if _, exists := dj.instruments[name]; exists {
		return fmt.Errorf("instrument '%s' already exists", name)
	}
	inst, err := NewInstrument(name, filepath)
	if err != nil {
		return err
	}
	dj.instruments[name] = inst
	dj.mixer.Add(inst.volume)
	log.Printf("✅ Instrument '%s' loaded successfully.", name)
	return nil
}

func (dj *DJMixer) GetInstrument(name string) (*Instrument, bool) {
	dj.mu.RLock()
	defer dj.mu.RUnlock()
	inst, ok := dj.instruments[name]
	return inst, ok
}

// GetAllInstrumentsSorted returns a slice of instruments sorted by name for consistent display.
func (dj *DJMixer) GetAllInstrumentsSorted() []*Instrument {
	dj.mu.RLock()
	defer dj.mu.RUnlock()
	keys := make([]string, 0, len(dj.instruments))
	for k := range dj.instruments {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	sortedInstruments := make([]*Instrument, len(keys))
	for i, key := range keys {
		sortedInstruments[i] = dj.instruments[key]
	}
	return sortedInstruments
}

func (dj *DJMixer) Close() {
	dj.mu.Lock()
	defer dj.mu.Unlock()
	log.Println("Shutting down all instruments...")
	for _, inst := range dj.instruments {
		_ = inst.Stop()
		_ = inst.Close()
	}
	dj.instruments = make(map[string]*Instrument)
}

// --- Main Application & Command Loop ---

func main() {
	log.SetFlags(0)
	log.Println("🎧 DJ Mixer Initializing...")

	shutdownChan := make(chan os.Signal, 1)
	signal.Notify(shutdownChan, os.Interrupt, syscall.SIGTERM)

	audioFiles, err := filepath.Glob(filepath.Join(AudioDir, "*.wav"))
	if err != nil || len(audioFiles) == 0 {
		log.Fatalf("❌ No WAV files found in '%s'. Error: %v", AudioDir, err)
	}

	sampleRate, err := getSampleRateFromFile(audioFiles[0])
	if err != nil {
		log.Fatalf("❌ Could not determine sample rate: %v", err)
	}

	if err := speaker.Init(sampleRate, sampleRate.N(time.Second/10)); err != nil {
		log.Fatalf("❌ Failed to initialize speaker: %v", err)
	}
	defer speaker.Close()

	mixer := NewDJMixer()
	defer mixer.Close()

	for _, file := range audioFiles {
		instrumentName := strings.TrimSuffix(filepath.Base(file), ".wav")
		if err := mixer.AddInstrument(instrumentName, file); err != nil {
			log.Printf("⚠️  Could not load '%s': %v", instrumentName, err)
		}
	}

	speaker.Play(&mixer.mixer)

	go runCommandLoop(mixer)

	<-shutdownChan

	log.Println("\n👋 Interrupt signal received. Shutting down gracefully...")
}

func getSampleRateFromFile(filename string) (beep.SampleRate, error) {
	f, err := os.Open(filename)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	_, format, err := wav.Decode(f)
	if err != nil {
		return 0, err
	}
	log.Printf("🎵 Detected sample rate %d Hz from '%s'.", format.SampleRate, filename)
	return format.SampleRate, nil
}

func runCommandLoop(dj *DJMixer) {
	scanner := bufio.NewScanner(os.Stdin)
	printHelp()
	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil && err != context.Canceled {
				log.Printf("❌ Error reading input: %v", err)
			}
			return
		}
		handleCommand(dj, scanner.Text())
	}
}

func handleCommand(dj *DJMixer, input string) {
	input = strings.TrimSpace(input)
	if input == "" {
		return
	}
	parts := strings.Fields(strings.ToLower(input))
	cmd := parts[0]
	var err error
	switch cmd {
	case "play", "start", "pause", "stop", "replay":
		target := ""
		if len(parts) > 1 {
			target = parts[1]
		}
		var action func(i *Instrument) error
		if cmd == "play" || cmd == "start" {
			action = func(i *Instrument) error { return i.Play() }
		} else if cmd == "pause" {
			action = func(i *Instrument) error { return i.Pause() }
		} else if cmd == "stop" {
			action = func(i *Instrument) error { return i.Stop() }
		} else if cmd == "replay" {
			action = func(i *Instrument) error { return i.Replay() }
		}
		dj.mu.RLock()
		if target != "" {
			if inst, ok := dj.instruments[target]; ok {
				err = action(inst)
			} else {
				err = fmt.Errorf("instrument '%s' not found", target)
			}
		} else {
			for _, inst := range dj.instruments {
				if e := action(inst); e != nil {
					log.Printf("⚠️  Skipping error on bulk operation for '%s': %v", inst.name, e)
				}
			}
		}
		dj.mu.RUnlock()
	case "volume", "vol":
		if len(parts) < 3 {
			log.Println("❌ Usage: volume <instrument> <value>")
			return
		}
		target, valStr := parts[1], parts[2]
		vol, parseErr := strconv.ParseFloat(valStr, 64)
		if parseErr != nil {
			log.Printf("❌ Invalid volume value: %s", valStr)
			return
		}
		if inst, ok := dj.GetInstrument(target); ok {
			err = inst.SetVolume(vol)
		} else {
			err = fmt.Errorf("instrument '%s' not found", target)
		}
	case "bpm":
		if len(parts) < 3 {
			log.Println("❌ Usage: bpm <instrument> <value>")
			return
		}
		target, valStr := parts[1], parts[2]
		targetBPM, parseErr := strconv.ParseFloat(valStr, 64)
		if parseErr != nil || targetBPM <= 0 {
			log.Printf("❌ Invalid BPM value: %s", valStr)
			return
		}
		if inst, ok := dj.GetInstrument(target); ok {
			ratio := targetBPM / BaseBPM
			err = inst.SetSpeed(ratio)
		} else {
			err = fmt.Errorf("instrument '%s' not found", target)
		}
	case "list", "ls":
		listInstruments(dj)
	case "help", "h":
		printHelp()
	case "quit", "exit", "q":
		log.Println("Use Ctrl+C to exit.")
		p, _ := os.FindProcess(os.Getpid())
		_ = p.Signal(os.Interrupt)
	default:
		log.Printf("❓ Unknown command: '%s'. Type 'help' for options.", cmd)
	}
	if err != nil {
		log.Printf("❌ Error: %v", err)
	}
}

func listInstruments(dj *DJMixer) {
	fmt.Println("--- Instruments ---")
	for _, inst := range dj.GetAllInstrumentsSorted() {
		state := inst.GetState()
		icon := "🔇" // Default to muted/stopped icon
		if state == StatePlaying {
			icon = "▶️"
		} else if state == StatePaused {
			icon = "⏸️"
		}
		currentBPM := BaseBPM * inst.speedRatio
		fmt.Printf(" %s %-10s (State: %-7s, Vol: %+.2f, BPM: %.1f)\n", icon, inst.name, state, inst.volume.Volume, currentBPM)
	}
	fmt.Println("-------------------")
}

func printHelp() {
	fmt.Println("\n--- DJ Mixer Commands ---")
	fmt.Println("  play [name]      - Plays or resumes an instrument (or all).")
	fmt.Println("  replay [name]    - Restarts an instrument from the beginning (or all).")
	fmt.Println("  pause [name]     - Pauses an instrument at its current position (or all).")
	fmt.Println("  stop [name]      - Stops an instrument by muting it (or all).")
	fmt.Println("  volume <name> <v> - Sets instrument volume (-2.0 to 2.0).")
	fmt.Println("  bpm <name> <v>   - Sets instrument BPM (e.g., 'bpm drums 140').")
	fmt.Println("  list             - Shows the status of all instruments.")
	fmt.Println("  help             - Shows this help message.")
	fmt.Println("  quit             - Exits the program (or use Ctrl+C).")
	fmt.Println("-------------------------")
}
