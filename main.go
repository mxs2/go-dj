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
	return []string{"parado", "tocando", "pausado"}[s]
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
		return nil, fmt.Errorf("falha ao abrir arquivo %s: %w", filename, err)
	}
	streamer, _, err := wav.Decode(f)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("falha ao decodificar arquivo WAV %s: %w", filename, err)
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
		return fmt.Errorf("proporção de velocidade %.2f está fora do intervalo [%.2f, %.2f]", ratio, MinSpeedRatio, MaxSpeedRatio)
	}
	i.mu.Lock()
	i.speedRatio = ratio
	i.mu.Unlock()
	speaker.Lock()
	i.resampler.SetRatio(ratio)
	speaker.Unlock()
	currentBPM := BaseBPM * ratio
	log.Printf("🎹 Tempo para '%s' definido para %.1f BPM (%.2fx).", i.name, currentBPM, ratio)
	return nil
}

func (i *Instrument) Play() error {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.state == StatePlaying {
		return fmt.Errorf("instrumento '%s' já está tocando", i.name)
	}
	i.volume.Silent = false // Unmute the track
	i.ctrl.Paused = false
	i.state = StatePlaying
	log.Printf("▶️  %s começou a tocar.", i.name)
	return nil
}

func (i *Instrument) Replay() error {
	i.mu.Lock()
	defer i.mu.Unlock()
	if err := i.streamer.Seek(0); err != nil {
		return fmt.Errorf("falha ao reiniciar '%s': %w", i.name, err)
	}
	i.volume.Silent = false // Unmute the track
	i.ctrl.Paused = false
	i.state = StatePlaying
	log.Printf("🔄 %s tocando novamente desde o início.", i.name)
	return nil
}

func (i *Instrument) Pause() error {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.state != StatePlaying {
		return fmt.Errorf("instrumento '%s' não está tocando (estado atual: %s)", i.name, i.state)
	}
	i.ctrl.Paused = true
	i.state = StatePaused
	log.Printf("⏸️  %s pausado.", i.name)
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
	log.Printf("🔇 %s silenciado (parado).", i.name)
	return nil
}

func (i *Instrument) SetVolume(vol float64) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	if vol < MinVolume || vol > MaxVolume {
		return fmt.Errorf("volume %.2f está fora do intervalo permitido [%.2f, %.2f]", vol, MinVolume, MaxVolume)
	}
	i.volume.Volume = vol
	log.Printf("🔊 Volume de %s definido para %.2f.", i.name, vol)
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
		return fmt.Errorf("instrumento '%s' já existe", name)
	}
	inst, err := NewInstrument(name, filepath)
	if err != nil {
		return err
	}
	dj.instruments[name] = inst
	dj.mixer.Add(inst.volume)
	log.Printf("✅ Instrumento '%s' carregado com sucesso.", name)
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
	log.Println("Desligando todos os instrumentos...")
	for _, inst := range dj.instruments {
		_ = inst.Stop()
		_ = inst.Close()
	}
	dj.instruments = make(map[string]*Instrument)
}

// --- Main Application & Command Loop ---

func main() {
	log.SetFlags(0)
	log.Println("🎧 Mesa de DJ Inicializando...")

	shutdownChan := make(chan os.Signal, 1)
	signal.Notify(shutdownChan, os.Interrupt, syscall.SIGTERM)

	audioFiles, err := filepath.Glob(filepath.Join(AudioDir, "*.wav"))
	if err != nil || len(audioFiles) == 0 {
		log.Fatalf("❌ Nenhum arquivo WAV encontrado em '%s'. Erro: %v", AudioDir, err)
	}

	sampleRate, err := getSampleRateFromFile(audioFiles[0])
	if err != nil {
		log.Fatalf("❌ Não foi possível determinar a taxa de amostragem: %v", err)
	}

	if err := speaker.Init(sampleRate, sampleRate.N(time.Second/10)); err != nil {
		log.Fatalf("❌ Falha ao inicializar o alto-falante: %v", err)
	}
	defer speaker.Close()

	mixer := NewDJMixer()
	defer mixer.Close()

	for _, file := range audioFiles {
		instrumentName := strings.TrimSuffix(filepath.Base(file), ".wav")
		if err := mixer.AddInstrument(instrumentName, file); err != nil {
			log.Printf("⚠️  Não foi possível carregar '%s': %v", instrumentName, err)
		}
	}

	speaker.Play(&mixer.mixer)

	go runCommandLoop(mixer)

	<-shutdownChan

	log.Println("\n👋 Sinal de interrupção recebido. Desligando graciosamente...")
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
	log.Printf("🎵 Taxa de amostragem detectada %d Hz de '%s'.", format.SampleRate, filename)
	return format.SampleRate, nil
}

func runCommandLoop(dj *DJMixer) {
	scanner := bufio.NewScanner(os.Stdin)
	printHelp()
	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil && err != context.Canceled {
				log.Printf("❌ Erro ao ler entrada: %v", err)
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
				err = fmt.Errorf("instrumento '%s' não encontrado", target)
			}
		} else {
			for _, inst := range dj.instruments {
				if e := action(inst); e != nil {
					log.Printf("⚠️  Ignorando erro na operação em lote para '%s': %v", inst.name, e)
				}
			}
		}
		dj.mu.RUnlock()
	case "volume", "vol":
		if len(parts) < 3 {
			log.Println("❌ Uso: volume <instrumento> <valor>")
			return
		}
		target, valStr := parts[1], parts[2]
		vol, parseErr := strconv.ParseFloat(valStr, 64)
		if parseErr != nil {
			log.Printf("❌ Valor de volume inválido: %s", valStr)
			return
		}
		if inst, ok := dj.GetInstrument(target); ok {
			err = inst.SetVolume(vol)
		} else {
			err = fmt.Errorf("instrumento '%s' não encontrado", target)
		}
	case "bpm":
		if len(parts) < 3 {
			log.Println("❌ Uso: bpm <instrumento> <valor>")
			return
		}
		target, valStr := parts[1], parts[2]
		targetBPM, parseErr := strconv.ParseFloat(valStr, 64)
		if parseErr != nil || targetBPM <= 0 {
			log.Printf("❌ Valor de BPM inválido: %s", valStr)
			return
		}
		if inst, ok := dj.GetInstrument(target); ok {
			ratio := targetBPM / BaseBPM
			err = inst.SetSpeed(ratio)
		} else {
			err = fmt.Errorf("instrumento '%s' não encontrado", target)
		}
	case "list", "ls":
		listInstruments(dj)
	case "help", "h":
		printHelp()
	case "quit", "exit", "q":
		log.Println("Use Ctrl+C para sair.")
		p, _ := os.FindProcess(os.Getpid())
		_ = p.Signal(os.Interrupt)
	default:
		log.Printf("❓ Comando desconhecido: '%s'. Digite 'help' para ver as opções.", cmd)
	}
	if err != nil {
		log.Printf("❌ Erro: %v", err)
	}
}

func listInstruments(dj *DJMixer) {
	fmt.Println("--- Instrumentos ---")
	for _, inst := range dj.GetAllInstrumentsSorted() {
		state := inst.GetState()
		icon := "🔇" // Default to muted/stopped icon
		if state == StatePlaying {
			icon = "▶️"
		} else if state == StatePaused {
			icon = "⏸️"
		}
		currentBPM := BaseBPM * inst.speedRatio
		fmt.Printf(" %s %-10s (Estado: %-7s, Vol: %+.2f, BPM: %.1f)\n", icon, inst.name, state, inst.volume.Volume, currentBPM)
	}
	fmt.Println("--------------------")
}

func printHelp() {
	fmt.Println("\n--- Comandos da Mesa de DJ ---")
	fmt.Println("  play [nome]       - Toca ou retoma um instrumento (ou todos).")
	fmt.Println("  replay [nome]     - Reinicia um instrumento do início (ou todos).")
	fmt.Println("  pause [nome]      - Pausa um instrumento na posição atual (ou todos).")
	fmt.Println("  stop [nome]       - Para um instrumento silenciando-o (ou todos).")
	fmt.Println("  volume <nome> <v> - Define o volume do instrumento (-2.0 a 2.0).")
	fmt.Println("  bpm <nome> <v>    - Define o BPM do instrumento (ex: 'bpm bateria 140').")
	fmt.Println("  list             - Mostra o status de todos os instrumentos.")
	fmt.Println("  help             - Mostra esta mensagem de ajuda.")
	fmt.Println("  quit             - Sai do programa (ou use Ctrl+C).")
	fmt.Println("------------------------------")
}
