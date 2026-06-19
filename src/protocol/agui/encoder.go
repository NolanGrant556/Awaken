package agui

import (
	"encoding/json"
	"fmt"
)

type Encoder struct{}

func (Encoder) EncodeJSON(event Event) ([]byte, error) {
	return json.Marshal(event)
}

func (enc Encoder) EncodeSSE(event Event) ([]byte, error) {
	payload, err := enc.EncodeJSON(event)
	if err != nil {
		return nil, err
	}
	var frame []byte
	if event.Seq > 0 {
		frame = append(frame, []byte(fmt.Sprintf("id: %d\n", event.Seq))...)
	}
	frame = append(frame, []byte("data: ")...)
	frame = append(frame, payload...)
	frame = append(frame, '\n', '\n')
	return frame, nil
}
