package domain

import "encoding/json"

// MaxDocumentBytes leaves room for the snapshot envelope within the 16 MiB
// downlink limit. Admission measures the same escaped JSON sent to clients.
const MaxDocumentBytes = 8 << 20

// ValidateDocumentCapacity prevents accepting graphs that cannot be reopened.
func ValidateDocumentCapacity(document Document) error {
	body, err := json.Marshal(document)
	if err != nil {
		return err
	}
	if len(body) > MaxDocumentBytes {
		return operationError("canvas_capacity_exceeded")
	}
	return nil
}
