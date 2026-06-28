package streamaccumulator

const (
	EventMessageStart      = "message_start"
	EventContentBlockDelta = "content_block_delta"
	DeltaText              = "text_delta"
)

type StreamEvent struct {
	UUID            string `json:"uuid"`
	SessionID       string `json:"session_id"`
	ParentToolUseID string `json:"parent_tool_use_id,omitempty"`
	Event           Event  `json:"event"`
}

type Event struct {
	Type         string                 `json:"type"`
	Message      *Message               `json:"message,omitempty"`
	ContentBlock map[string]interface{} `json:"content_block,omitempty"`
	Index        int                    `json:"index,omitempty"`
	Delta        *Delta                 `json:"delta,omitempty"`
	Usage        map[string]int         `json:"usage,omitempty"`
	Error        map[string]interface{} `json:"error,omitempty"`
}

type Message struct {
	ID    string         `json:"id"`
	Usage map[string]int `json:"usage,omitempty"`
}

type Delta struct {
	Type        string `json:"type,omitempty"`
	Text        string `json:"text,omitempty"`
	Thinking    string `json:"thinking,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
	StopReason  string `json:"stop_reason,omitempty"`
}

type Accumulator struct {
	byMessage      map[string]map[int]*blockState
	scopeToMessage map[string]string
}

type blockState struct {
	text string
}

func New() *Accumulator {
	return &Accumulator{
		byMessage:      make(map[string]map[int]*blockState),
		scopeToMessage: make(map[string]string),
	}
}

func (a *Accumulator) Accumulate(events []StreamEvent) []StreamEvent {
	out := make([]StreamEvent, 0, len(events))
	touched := make(map[*blockState]int)

	for _, event := range events {
		switch event.Event.Type {
		case EventMessageStart:
			if event.Event.Message == nil || event.Event.Message.ID == "" {
				out = append(out, event)
				continue
			}
			messageID := event.Event.Message.ID
			scope := scopeKey(event.SessionID, event.ParentToolUseID)
			if previousID, ok := a.scopeToMessage[scope]; ok {
				delete(a.byMessage, previousID)
			}
			a.scopeToMessage[scope] = messageID
			a.byMessage[messageID] = make(map[int]*blockState)
			out = append(out, event)

		case EventContentBlockDelta:
			if event.Event.Delta == nil || event.Event.Delta.Type != DeltaText {
				out = append(out, event)
				continue
			}

			messageID, ok := a.scopeToMessage[scopeKey(event.SessionID, event.ParentToolUseID)]
			if !ok {
				out = append(out, event)
				continue
			}

			blocks, ok := a.byMessage[messageID]
			if !ok {
				out = append(out, event)
				continue
			}

			block := blocks[event.Event.Index]
			if block == nil {
				block = &blockState{}
				blocks[event.Event.Index] = block
			}
			block.text += event.Event.Delta.Text

			if outIndex, ok := touched[block]; ok {
				out[outIndex].Event.Delta.Text = block.text
				continue
			}

			snapshot := event
			snapshot.Event = Event{
				Type:  EventContentBlockDelta,
				Index: event.Event.Index,
				Delta: &Delta{
					Type: DeltaText,
					Text: block.text,
				},
			}
			touched[block] = len(out)
			out = append(out, snapshot)

		default:
			out = append(out, event)
		}
	}

	return out
}

func (a *Accumulator) ClearCompleted(sessionID, parentToolUseID, messageID string) {
	delete(a.byMessage, messageID)

	scope := scopeKey(sessionID, parentToolUseID)
	if activeID, ok := a.scopeToMessage[scope]; ok && activeID == messageID {
		delete(a.scopeToMessage, scope)
	}
}

func scopeKey(sessionID, parentToolUseID string) string {
	return sessionID + "\x00" + parentToolUseID
}
