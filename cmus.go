package main

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// TrackInfo holds parsed data from `cmus-remote -Q` output.
type TrackInfo struct {
	Artist   string
	Title    string
	Duration int
	Current  int
}

// ParseCmusOutput parses the text produced by `cmus-remote -Q` and returns a TrackInfo.
// It is resilient: missing fields are left zero-valued.
func ParseCmusOutput(output string) (TrackInfo, error) {
	var info TrackInfo
	if strings.TrimSpace(output) == "" {
		return info, fmt.Errorf("empty output")
	}

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Example tag lines: "tag artist Radiohead"
		if strings.HasPrefix(line, "tag artist ") {
			info.Artist = strings.TrimSpace(line[len("tag artist "):])
			continue
		}
		if strings.HasPrefix(line, "tag title ") {
			info.Title = strings.TrimSpace(line[len("tag title "):])
			continue
		}

		// duration <seconds>
		if strings.HasPrefix(line, "duration ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				if v, err := strconv.Atoi(parts[1]); err == nil {
					info.Duration = v
				}
			}
			continue
		}

		// position <seconds>  (the currently played position)
		if strings.HasPrefix(line, "position ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				if v, err := strconv.Atoi(parts[1]); err == nil {
					info.Current = v
				}
			}
			continue
		}
	}

	return info, nil
}

// QueryCmus runs `cmus-remote -Q` and parses the result. Returns an error if the command fails.
func QueryCmus() (TrackInfo, error) {
	out, err := exec.Command("cmus-remote", "-Q").CombinedOutput()
	if err != nil {
		return TrackInfo{}, fmt.Errorf("running cmus-remote: %w; output: %s", err, strings.TrimSpace(string(out)))
	}
	return ParseCmusOutput(string(out))
}
