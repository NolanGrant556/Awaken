package structuredio

import (
	"bytes"
	"encoding/json"
)

type LineReaderOption func(*LineReader)

func EmitKeepAlive() LineReaderOption {
	return func(reader *LineReader) {
		reader.emitKeepAlive = true
	}
}

type LineReader struct {
	buffer        []byte
	prepended     [][]byte
	emitKeepAlive bool
}

type ReadResult struct {
	Envelopes   []Envelope
	Diagnostics []Diagnostic
}

func NewLineReader(options ...LineReaderOption) *LineReader {
	reader := &LineReader{}
	for _, option := range options {
		option(reader)
	}
	return reader
}

func (r *LineReader) Push(chunk []byte) ReadResult {
	if len(chunk) > 0 {
		r.buffer = append(r.buffer, chunk...)
	}
	return r.drain(false)
}

func (r *LineReader) Close() ReadResult {
	return r.drain(true)
}

func (r *LineReader) PrependUserMessage(content string) error {
	message := UserMessage{
		Type:            EnvelopeUser,
		SessionID:       "",
		ParentToolUseID: nil,
	}
	message.Message.Role = "user"
	message.Message.Content = content

	line, err := NDJSONSafeMarshal(message)
	if err != nil {
		return err
	}
	line = append(line, '\n')
	r.prepended = append(r.prepended, append([]byte(nil), line...))
	return nil
}

type UserMessage struct {
	Type      EnvelopeType `json:"type"`
	SessionID string       `json:"session_id"`
	Message   struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"message"`
	ParentToolUseID *string `json:"parent_tool_use_id"`
}

func (r *LineReader) drain(final bool) ReadResult {
	r.applyPrepended()

	var result ReadResult
	for {
		newline := bytes.IndexByte(r.buffer, '\n')
		if newline == -1 {
			break
		}
		line := r.buffer[:newline]
		r.buffer = r.buffer[newline+1:]
		r.processLine(line, &result)
		r.applyPrepended()
	}

	if final {
		if len(bytes.TrimSpace(r.buffer)) > 0 {
			r.processLine(r.buffer, &result)
		}
		r.buffer = nil
	}

	return result
}

func (r *LineReader) processLine(line []byte, result *ReadResult) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return
	}

	envelope, diagnostic := ParseEnvelope(line)
	if diagnostic != nil {
		result.Diagnostics = append(result.Diagnostics, *diagnostic)
		return
	}
	if envelope.IsKeepAlive() && !r.emitKeepAlive {
		return
	}
	result.Envelopes = append(result.Envelopes, envelope)
}

func (r *LineReader) applyPrepended() {
	if len(r.prepended) == 0 {
		return
	}

	var prefix []byte
	for _, line := range r.prepended {
		prefix = append(prefix, line...)
	}
	r.buffer = append(prefix, r.buffer...)
	r.prepended = nil
}

func EnvelopeMessageContent(envelope Envelope) (string, bool) {
	messageRaw, ok := envelope.Fields["message"]
	if !ok {
		return "", false
	}
	var message struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(messageRaw, &message); err != nil {
		return "", false
	}
	return message.Content, true
}
