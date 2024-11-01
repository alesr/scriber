package scriber

import (
	"bytes"
	"context"
	"io"
	"testing"

	"log/slog"

	"github.com/alesr/whisperclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockWhisperClient struct {
	transcribeAudioFunc func(ctx context.Context, in whisperclient.TranscribeAudioInput) ([]byte, error)
}

func (m *mockWhisperClient) TranscribeAudio(ctx context.Context, in whisperclient.TranscribeAudioInput) ([]byte, error) {
	return m.transcribeAudioFunc(ctx, in)
}

func TestNew(t *testing.T) {
	t.Parallel()

	logger := noopLogger()

	whisperCli := &mockWhisperClient{}

	scriber := New(logger, whisperCli)

	require.NotNil(t, scriber)
	assert.Equal(t, logger.WithGroup("scriber"), scriber.logger)
	assert.Equal(t, whisperCli, scriber.whisperClient)
	assert.NotNil(t, scriber.resultsCh)
}

func TestGenerateOutputFileName(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name          string
		givenFilename string
		givenOutType  string
		expected      string
	}{
		{
			name:          "subtitles",
			givenFilename: "foo.mp4",
			givenOutType:  string(OutputTypeSubtitles),
			expected:      "foo.srt",
		},
		{
			name:          "transcript",
			givenFilename: "bar.mp4",
			givenOutType:  string(OutputTypeTranscript),
			expected:      "bar.txt",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := generateOutputFileName(tc.givenFilename, OutputType(tc.givenOutType))
			assert.Equal(t, tc.expected, got)
		})
	}
}

func TestTranscribeAudio(t *testing.T) {
	t.Parallel()

	mockClient := &mockWhisperClient{
		transcribeAudioFunc: func(ctx context.Context, in whisperclient.TranscribeAudioInput) ([]byte, error) {
			return []byte("mock transcription"), nil
		},
	}

	scriber := New(noopLogger(), mockClient)
	scriber.convertToWavFunc = func(data []byte) (*bytes.Buffer, error) {
		return bytes.NewBufferString("mock audio data"), nil
	}

	audioData := bytes.NewBufferString("mock audio data")

	in := Input{
		Name:       "test.mp4",
		Language:   "en",
		OutputType: OutputTypeSubtitles,
	}

	ctx := context.Background()
	text, err := scriber.transcribeAudio(ctx, audioData, in)

	require.NoError(t, err)
	assert.Equal(t, []byte("mock transcription"), text)
	assert.Equal(t, "mock audio data", audioData.String())
}

func TestInputValidate(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		input   Input
		wantErr bool
	}{
		{
			name: "valid input",
			input: Input{
				Name:       "test.mp4",
				OutputType: OutputTypeSubtitles,
				Language:   "en",
				Data:       io.NopCloser(bytes.NewBufferString("mock data")),
			},
			wantErr: false,
		},
		{
			name: "missing name",
			input: Input{
				OutputType: OutputTypeSubtitles,
				Language:   "en",
				Data:       io.NopCloser(bytes.NewBufferString("mock data")),
			},
			wantErr: true,
		},
		{
			name: "missing extension",
			input: Input{
				Name:       "test",
				OutputType: OutputTypeSubtitles,
				Language:   "en",
				Data:       io.NopCloser(bytes.NewBufferString("mock data")),
			},
			wantErr: true,
		},
		{
			name: "unsupported output type",
			input: Input{
				Name:       "test.mp4",
				OutputType: "unsupported",
				Language:   "en",
				Data:       io.NopCloser(bytes.NewBufferString("mock data")),
			},
			wantErr: true,
		},
		{
			name: "missing language",
			input: Input{
				Name:       "test.mp4",
				OutputType: OutputTypeSubtitles,
				Data:       io.NopCloser(bytes.NewBufferString("mock data")),
			},
			wantErr: true,
		},
		{
			name: "missing data",
			input: Input{
				Name:       "test.mp4",
				OutputType: OutputTypeSubtitles,
				Language:   "en",
			},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.input.validate()
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestProcess(t *testing.T) {
	t.Parallel()

	mockClient := &mockWhisperClient{
		transcribeAudioFunc: func(ctx context.Context, in whisperclient.TranscribeAudioInput) ([]byte, error) {
			return []byte("mock transcription"), nil
		},
	}

	scriber := New(noopLogger(), mockClient)

	testCases := []struct {
		name    string
		input   Input
		wantErr bool
	}{
		{
			name: "valid input",
			input: Input{
				Name:       "test.mp4",
				OutputType: OutputTypeSubtitles,
				Language:   "en",
				Data:       io.NopCloser(bytes.NewBufferString("mock data")),
			},
			wantErr: false,
		},
		{
			name: "invalid input",
			input: Input{
				Name:       "",
				OutputType: OutputTypeSubtitles,
				Language:   "en",
				Data:       io.NopCloser(bytes.NewBufferString("mock data")),
			},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			err := scriber.Process(ctx, tc.input)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func noopLogger() *slog.Logger {
	return slog.New(
		slog.NewTextHandler(
			io.Discard,
			&slog.HandlerOptions{},
		))
}
