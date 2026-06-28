package voiceinput

import "strings"

type StreamState string

const (
	StreamStateOpen    StreamState = "open"
	StreamStateClosing StreamState = "closing"
	StreamStateClosed  StreamState = "closed"
)

type OutboundEventKind string

const (
	OutboundEventKeepAlive        OutboundEventKind = "KeepAlive"
	OutboundEventCloseStream      OutboundEventKind = "CloseStream"
	OutboundEventBinaryAudioFrame OutboundEventKind = "BinaryAudioFrame"
)

type OutboundEvent struct {
	Kind    OutboundEventKind
	Payload []byte
	Dropped bool
	Reason  string
}

type FinalizeSource string

const (
	FinalizeSourceEndpoint      FinalizeSource = "endpoint"
	FinalizeSourceNoDataTimeout FinalizeSource = "no_data_timeout"
	FinalizeSourceSafetyTimeout FinalizeSource = "safety_timeout"
	FinalizeSourceWSClose       FinalizeSource = "ws_close"
	FinalizeSourceAlreadyClosed FinalizeSource = "already_closed"
)

type FinalizeResult struct {
	Source     FinalizeSource
	Transcript string
}

type TranscriptUpdate struct {
	Accepted   bool
	Final      bool
	Text       string
	Transcript string
}

type Session struct {
	state           StreamState
	closeStreamSent bool
	finalized       bool
	endpointed      bool
	finalParts      []string
	partial         string
	errors          []ClassifiedError
}

func NewSession() *Session {
	return &Session{state: StreamStateOpen}
}

func (s *Session) State() StreamState {
	return s.state
}

func (s *Session) CloseStreamSent() bool {
	return s.closeStreamSent
}

func (s *Session) Endpointed() bool {
	return s.endpointed
}

func (s *Session) KeepAlive() OutboundEvent {
	if s.state == StreamStateClosed {
		return OutboundEvent{
			Kind:    OutboundEventKeepAlive,
			Dropped: true,
			Reason:  "stream_closed",
		}
	}
	return OutboundEvent{Kind: OutboundEventKeepAlive}
}

func (s *Session) SendAudioFrame(frame []byte) OutboundEvent {
	event := OutboundEvent{Kind: OutboundEventBinaryAudioFrame}
	if s.state == StreamStateClosed {
		event.Dropped = true
		event.Reason = "stream_closed"
		return event
	}
	if s.closeStreamSent {
		event.Dropped = true
		event.Reason = "close_stream_sent"
		return event
	}
	event.Payload = append([]byte(nil), frame...)
	return event
}

func (s *Session) CloseStream() OutboundEvent {
	event := OutboundEvent{Kind: OutboundEventCloseStream}
	if s.closeStreamSent || s.finalized || s.state == StreamStateClosed {
		event.Dropped = true
		event.Reason = string(FinalizeSourceAlreadyClosed)
		return event
	}
	s.closeStreamSent = true
	s.state = StreamStateClosing
	return event
}

func (s *Session) AddTranscriptText(text string, final bool) TranscriptUpdate {
	text = strings.TrimSpace(text)
	if text == "" || s.endpointed || s.finalized {
		return TranscriptUpdate{
			Accepted:   false,
			Final:      final,
			Text:       text,
			Transcript: s.TranscriptText(),
		}
	}

	if final {
		s.commitTranscript(text)
		s.partial = ""
	} else {
		s.partial = text
	}

	return TranscriptUpdate{
		Accepted:   true,
		Final:      final,
		Text:       text,
		Transcript: s.TranscriptText(),
	}
}

func (s *Session) Endpoint() TranscriptUpdate {
	if s.endpointed {
		return TranscriptUpdate{
			Accepted:   false,
			Final:      true,
			Transcript: s.TranscriptText(),
		}
	}

	if s.partial != "" {
		s.commitTranscript(s.partial)
		s.partial = ""
	}
	s.endpointed = true

	return TranscriptUpdate{
		Accepted:   true,
		Final:      true,
		Transcript: s.TranscriptText(),
	}
}

func (s *Session) Finalize(source FinalizeSource) FinalizeResult {
	if s.finalized {
		return FinalizeResult{
			Source:     FinalizeSourceAlreadyClosed,
			Transcript: s.TranscriptText(),
		}
	}

	if source == "" {
		source = FinalizeSourceSafetyTimeout
	}

	if source == FinalizeSourceEndpoint {
		s.Endpoint()
	} else if s.partial != "" {
		s.commitTranscript(s.partial)
		s.partial = ""
	}

	s.closeStreamSent = true
	s.finalized = true
	s.state = StreamStateClosed

	return FinalizeResult{
		Source:     source,
		Transcript: s.TranscriptText(),
	}
}

func (s *Session) TranscriptText() string {
	parts := make([]string, 0, len(s.finalParts)+1)
	parts = append(parts, s.finalParts...)
	if s.partial != "" {
		parts = append(parts, s.partial)
	}
	return strings.Join(parts, " ")
}

func (s *Session) FinalTranscriptText() string {
	return strings.Join(s.finalParts, " ")
}

func (s *Session) RecordError(event ErrorEvent) ClassifiedError {
	err := ClassifyError(event)
	if !err.Ignored {
		s.errors = append(s.errors, err)
	}
	return err
}

func (s *Session) Errors() []ClassifiedError {
	return append([]ClassifiedError(nil), s.errors...)
}

func (s *Session) commitTranscript(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	s.finalParts = append(s.finalParts, text)
}
