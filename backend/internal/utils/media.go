package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
)

type MediaInfo struct {
	Width  int
	Height int
	Format string
}

type ffprobeOutput struct {
	Streams []struct {
		Width  int    `json:"width"`
		Height int    `json:"height"`
		Codec  string `json:"codec_name"`
		Type   string `json:"codec_type"`
	} `json:"streams"`
}

// ProbeFile runs ffprobe on the given file path to extract media information.
func ProbeFile(path string) (*MediaInfo, error) {
	cmd := exec.Command("ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_streams",
		path,
	)

	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffprobe failed: %w", err)
	}

	var data ffprobeOutput
	if err := json.Unmarshal(out.Bytes(), &data); err != nil {
		return nil, fmt.Errorf("failed to parse ffprobe output: %w", err)
	}

	for _, stream := range data.Streams {
		if stream.Type == "video" {
			return &MediaInfo{
				Width:  stream.Width,
				Height: stream.Height,
				Format: stream.Codec,
			}, nil
		}
	}

	return nil, fmt.Errorf("no video stream found")
}
