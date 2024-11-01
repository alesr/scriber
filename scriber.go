package scriber

import (
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

	convertToWav convertToWavFunc = func(r io.Reader, w io.Writer) error {
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

		cmd.Stdout = w
		stdin, err := cmd.StdinPipe()
		if err != nil {
			return fmt.Errorf("failed to create stdin pipe: %w", err)
		}
		cmd.Stderr = os.Stderr

		if err := cmd.Start(); err != nil {
			return fmt.Errorf("failed to start ffmpeg: %w", err)
		}

		go func() {
			defer stdin.Close()
			if _, err := io.Copy(stdin, r); err != nil {
				fmt.Fprintf(os.Stderr, "error copying to stdin: %v\n", err)
			}
		}()

		if err := cmd.Wait(); err != nil {
			return fmt.Errorf("ffmpeg failed: %w", err)
		}
		return nil
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
	convertToWavFunc func(r io.Reader, w io.Writer) error
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

	if err := in.validate(); err != nil {
		return fmt.Errorf("invalid input: %w", err)
	}

	// Create pipes for conversion.
	// The pipeWriter will be used for writing the audio data from the input to ffmpeg.
	// The pipeReader will be used for reading the converted audio from ffmpeg and transcribing it.
	// Basically, we are converting the audio and transcribing it at the same time.
	// This is done to avoid writing the converted audio to disk or holding it in memory.
	pipeReader, pipeWriter := io.Pipe()

	errCh := make(chan error, 1)

	// Start conversion in goroutine
	go func() {
		defer func() {
			if err := pipeWriter.Close(); err != nil {
				select {
				case errCh <- fmt.Errorf("error closing pipe writer: %w", err):
				default:
				}
			}
		}()

		if err := s.convertToWavFunc(in.Data, pipeWriter); err != nil {
			errCh <- fmt.Errorf("could not convert to wav: %w", err)
			return
		}
		close(errCh)
	}()

	defer func() {
		in.Data.Close()
		pipeReader.Close()
	}()

	text, err := s.transcribeAudio(ctx, pipeReader, in)
	if err != nil {
		return fmt.Errorf("could not transcribe audio: %w", err)
	}

	select {
	case err := <-errCh:
		if err != nil {
			return err
		}
	case <-ctx.Done():
		return ctx.Err()
	}

	select {
	case s.resultsCh <- Output{
		Name: generateOutputFileName(in.Name, in.OutputType),
		Text: text,
	}:
	case <-ctx.Done():
		return ctx.Err()
	}

	s.logger.Info("Processing complete", slog.String("file", in.Name))
	return nil
}

func (s *Scriber) Collect() <-chan Output {
	return s.resultsCh
}

func (s *Scriber) transcribeAudio(ctx context.Context, audioData io.Reader, in Input) ([]byte, error) {
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
