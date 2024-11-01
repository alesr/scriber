package scriber

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/alesr/whisperclient"
)

const (
	sampleRate                      = "5200"
	OutputTypeSubtitles  OutputType = "subtitles"
	OutputTypeTranscript OutputType = "transcript"
)

var (
	supportedOutputTypes = map[OutputType]struct{}{OutputTypeSubtitles: {}, OutputTypeTranscript: {}}

	convertToWav convertToWavFunc = func(data []byte) (*bytes.Buffer, error) {
		cmd := exec.Command(
			"ffmpeg", "-y",
			"-i", "pipe:0",
			"-vn",
			"-acodec", "pcm_s16le",
			"-ar", sampleRate,
			"-ac", "2",
			"-b:a", "32k",
			"-f", "wav",
			"pipe:1",
		)

		var outBuf bytes.Buffer
		cmd.Stdout = &outBuf
		cmd.Stdin = bytes.NewReader(data)
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("ffmpeg failed: %w", err)
		}
		return &outBuf, nil
	}
)

type (

	// whisperClient is a client for the whisper service.
	whisperClient interface {
		TranscribeAudio(ctx context.Context, in whisperclient.TranscribeAudioInput) ([]byte, error)
	}

	// OutputType represents the type of output to generate.
	OutputType string

	// Output represents the result of processing an input file.
	Output struct {
		Name string
		Text []byte
	}

	// convertToWavFunc is a function that converts audio data to wav format.
	convertToWavFunc func(data []byte) (*bytes.Buffer, error)
)

// Input represents an input file to be processed.
type Input struct {
	Name       string
	OutputType OutputType
	Language   string
	Data       io.ReadCloser
}

func (i *Input) validate() error {
	if i.Name == "" {
		return errNameRequired
	}

	if filepath.Ext(i.Name) == "" {
		return errExtRequired
	}

	if _, ok := supportedOutputTypes[i.OutputType]; !ok {
		return errorOutputType
	}

	if i.Language == "" {
		return errorLanguage
	}

	if i.Data == nil {
		return errorData
	}
	return nil
}

// Scriber is a service that processes
// audio files and transcribes them.
type Scriber struct {
	logger           *slog.Logger
	convertToWavFunc convertToWavFunc
	whisperClient    whisperClient
	resultsCh        chan Output
}

func New(logger *slog.Logger, whisperCli whisperClient) *Scriber {
	return &Scriber{
		logger:           logger.WithGroup("scriber"),
		convertToWavFunc: convertToWav,
		whisperClient:    whisperCli,
		resultsCh:        make(chan Output, 10),
	}
}

func (s *Scriber) Process(ctx context.Context, in Input) error {
	s.logger.Info("Processing file", slog.String("name", in.Name))

	data, err := io.ReadAll(in.Data)
	if err != nil {
		return fmt.Errorf("reading input: %w", err)
	}
	defer in.Data.Close()

	audioData, err := convertToWav(data)
	if err != nil {
		return fmt.Errorf("could not convert to wav: %w", err)
	}

	text, err := s.transcribeAudio(ctx, audioData, in)
	if err != nil {
		return fmt.Errorf("could not transcribe audio: %w", err)
	}

	s.resultsCh <- Output{
		Name: generateOutputFileName(in.Name, in.OutputType), // foo.mp4 -> foo.srt
		Text: text,
	}

	s.logger.Info("Processing complete", slog.String("file", in.Name))
	return nil
}

func (s *Scriber) Collect() <-chan Output {
	return s.resultsCh
}

func (s *Scriber) transcribeAudio(ctx context.Context, audioData *bytes.Buffer, in Input) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	s.logger.Debug("Transcribing audio", slog.String("file", in.Name))

	text, err := s.whisperClient.TranscribeAudio(ctx, whisperclient.TranscribeAudioInput{
		Name:     in.Name,
		Language: in.Language,
		Format:   string(in.OutputType),
		Data:     audioData,
	})
	if err != nil {
		return nil, fmt.Errorf("transcription failed: %w", err)
	}
	return text, nil
}

func generateOutputFileName(filename string, outType OutputType) string {
	ext := ".srt"
	if outType == OutputTypeTranscript {
		ext = ".txt"
	}
	return strings.Replace(filename, filepath.Ext(filename), ext, 1)
}
