# Scriber

Scriber is a Go service that processes audio files and transcribes them into subtitles or transcripts using the Whisper API.

## Features

- Convert audio files to WAV format using `ffmpeg`
- Transcribe audio files into subtitles (`.srt`) or transcripts (`.txt`)
- Supports multiple languages

## Installation

1. Clone the repository:

    ```sh
    git clone https://github.com/alesr/scriber.git
    cd scriber
    ```

2. Install dependencies:

    ```sh
    go mod download
    ```

3. Ensure `ffmpeg` is installed on your system:

    ```sh
    # On macOS
    brew install ffmpeg

    # On Ubuntu
    sudo apt-get install ffmpeg
    ```

## Usage

### Example

```go
package main

import (
    "context"
    "os"
    "log/slog"

    "github.com/alesr/scriber"
    "github.com/alesr/whisperclient"
)

func main() {
    logger := slog.Default()
    whisperCli := whisperclient.NewClient("your-api-key")

    s := scriber.New(logger, whisperCli)

    inputFile, err := os.Open("path/to/example.mp4")
    if err != nil {
        logger.Error("Failed to open input file", slog.Error(err))
        os.Exit(1)
    }
    defer inputFile.Close()

    input := scriber.Input{
        Name:       "example.mp4",
        OutputType: scriber.OutputTypeSubtitles,
        Language:   "en",
        Data:       inputFile,
    }

    ctx := context.Background()
    if err := s.Process(ctx, input); err != nil {
        logger.Error("Failed to process file", slog.Error(err))
        os.Exit(1)
    }

    for result := range s.Collect() {
        logger.Info("Transcription result", slog.String("file", result.Name), slog.String("text", string(result.Text)))
    }
}

```

## Testing

Run the tests:

```sh
go test ./...
```

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
