package audio

import (
	"fmt"
	"os"
	"time"
)

// Duration returns the playing time of an MP3 by summing its frame lengths.
// A leading Xing/Info/VBRI tag frame is not audio and is skipped.
func Duration(path string) (time.Duration, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	pos := id3v2Len(data)
	first := -1
	for i := pos; i < len(data)-4 && i < pos+4096; i++ {
		if f, ok := parseFrame(data[i:]); ok && i+f.length+4 <= len(data) {
			if _, ok2 := parseFrame(data[i+f.length:]); ok2 {
				first = i
				break
			}
		}
	}
	if first < 0 {
		return 0, fmt.Errorf("%s: no MP3 frames found", path)
	}
	pos = first
	var samples int64
	sr := 0
	for pos+4 <= len(data) {
		f, ok := parseFrame(data[pos:])
		if !ok || pos+f.length > len(data) {
			break
		}
		if pos == first {
			sr = sampleRateOf(f)
			tagOff := pos + 4 + f.sideInfo()
			if tagOff+4 <= len(data) {
				if s := string(data[tagOff : tagOff+4]); s == "Xing" || s == "Info" {
					pos += f.length
					continue
				}
			}
		}
		samples += int64(f.samples)
		pos += f.length
	}
	if sr == 0 || samples == 0 {
		return 0, fmt.Errorf("%s: cannot determine duration", path)
	}
	return time.Duration(samples) * time.Second / time.Duration(sr), nil
}
