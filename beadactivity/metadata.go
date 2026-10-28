package beadactivity

import (
	"bytes"
	"encoding/json"
	"errors"
)

// decodeMetadataObject accepts historical object values and the single JSON
// string layer emitted by bd --set-metadata. Never recursively unwrap strings.
func decodeMetadataObject(raw json.RawMessage, dst any) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) > 0 && raw[0] == '"' {
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return err
		}
		raw = bytes.TrimSpace([]byte(text))
	}
	if len(raw) == 0 || raw[0] != '{' {
		return errors.New("metadata must contain a JSON object")
	}
	return json.Unmarshal(raw, dst)
}
