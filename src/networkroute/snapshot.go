package networkroute

import "encoding/json"

func ParseSystemSnapshot(data []byte) (SystemSnapshot, error) {
	var snapshot SystemSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return SystemSnapshot{}, err
	}
	return normalizeSnapshot(snapshot)
}
