package providerhttp

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

// audioReceiptObject rejects ambiguous duplicate keys before unmarshalling a
// paid audio receipt. Keeping RawMessage values preserves decimal quantities.
func audioReceiptObject(raw []byte) (map[string]json.RawMessage, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	opening, err := d.Token()
	if err != nil || opening != json.Delim('{') {
		return nil, errors.New("invalid audio receipt")
	}
	fields := map[string]json.RawMessage{}
	for d.More() {
		token, err := d.Token()
		if err != nil {
			return nil, err
		}
		name, ok := token.(string)
		if !ok || fields[name] != nil {
			return nil, errors.New("ambiguous audio receipt")
		}
		var value json.RawMessage
		if err := d.Decode(&value); err != nil {
			return nil, err
		}
		fields[name] = value
	}
	closing, err := d.Token()
	if err != nil || closing != json.Delim('}') {
		return nil, errors.New("invalid audio receipt")
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, errors.New("incomplete audio receipt")
	}
	return fields, nil
}
