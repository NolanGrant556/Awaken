package agui

import "encoding/json"

type Encoder struct{}

func (Encoder) EncodeJSON(event Event) ([]byte, error) {
	return json.Marshal(event)
}

func (enc Encoder) EncodeSSE(event Event) ([]byte, error) {
	payload, err := enc.EncodeJSON(event)
	if err != nil {
		return nil, err
	}
	frame := append([]byte("data: "), payload...)
	frame = append(frame, '\n', '\n')
	return frame, nil
}
