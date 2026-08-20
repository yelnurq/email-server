package emailapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

// decodeStrict parses JSON with unknown fields rejected.
func decodeStrict(body []byte, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return errors.New("invalid JSON body")
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return errors.New("request body must contain a single JSON value")
	}
	return nil
}
