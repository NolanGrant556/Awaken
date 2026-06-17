package server

import (
	"net/http"

	"awaken/src/protocol/agui"
)

type SSEStream struct {
	writer  http.ResponseWriter
	flusher http.Flusher
	encoder agui.Encoder
}

func NewSSEStream(w http.ResponseWriter) (*SSEStream, bool) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, false
	}

	headers := w.Header()
	headers.Set("Content-Type", "text/event-stream")
	headers.Set("Cache-Control", "no-cache")
	headers.Set("Connection", "keep-alive")
	headers.Set("X-Accel-Buffering", "no")

	return &SSEStream{
		writer:  w,
		flusher: flusher,
		encoder: agui.Encoder{},
	}, true
}

func (s *SSEStream) WriteEvent(event agui.Event) error {
	frame, err := s.encoder.EncodeSSE(event)
	if err != nil {
		return err
	}
	if _, err := s.writer.Write(frame); err != nil {
		return err
	}
	s.flusher.Flush()
	return nil
}
