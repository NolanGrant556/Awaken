package remotesse

import (
	"strconv"
	"strings"
)

// Frame is one parsed Server-Sent Events frame.
type Frame struct {
	Event   string
	ID      string
	Data    string
	Comment bool
}

// SequenceNum parses the SSE id field as a sequence number.
func (f Frame) SequenceNum() (int64, bool) {
	if f.ID == "" {
		return 0, false
	}
	seq, err := strconv.ParseInt(f.ID, 10, 64)
	if err != nil {
		return 0, false
	}
	return seq, true
}

// ParseFrames incrementally parses complete SSE frames from buffer.
//
// The returned remaining string contains any trailing incomplete frame and can
// be prepended to the next stream chunk.
func ParseFrames(buffer string) (frames []Frame, remaining string) {
	pos := 0
	for {
		idx, delimLen := nextFrameDelimiter(buffer[pos:])
		if idx == -1 {
			return frames, buffer[pos:]
		}

		raw := buffer[pos : pos+idx]
		pos += idx + delimLen

		frame, ok := parseFrame(raw)
		if ok {
			frames = append(frames, frame)
		}
	}
}

func nextFrameDelimiter(s string) (idx int, delimLen int) {
	lf := strings.Index(s, "\n\n")
	crlf := strings.Index(s, "\r\n\r\n")

	switch {
	case lf == -1 && crlf == -1:
		return -1, 0
	case lf == -1:
		return crlf, len("\r\n\r\n")
	case crlf == -1:
		return lf, len("\n\n")
	case crlf < lf:
		return crlf, len("\r\n\r\n")
	default:
		return lf, len("\n\n")
	}
}

func parseFrame(raw string) (Frame, bool) {
	if strings.TrimSpace(raw) == "" {
		return Frame{}, false
	}

	var frame Frame
	var dataLines []string

	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if strings.HasPrefix(line, ":") {
			frame.Comment = true
			continue
		}

		colonIdx := strings.IndexByte(line, ':')
		if colonIdx == -1 {
			continue
		}

		field := line[:colonIdx]
		value := line[colonIdx+1:]
		if strings.HasPrefix(value, " ") {
			value = value[1:]
		}

		switch field {
		case "event":
			frame.Event = value
		case "id":
			frame.ID = value
		case "data":
			dataLines = append(dataLines, value)
		}
	}

	if len(dataLines) > 0 {
		frame.Data = strings.Join(dataLines, "\n")
	}
	if frame.Data == "" && !frame.Comment {
		return Frame{}, false
	}
	return frame, true
}
