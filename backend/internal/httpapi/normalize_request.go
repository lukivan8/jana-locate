package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
)

// Keep pointer fields until validation so missing/null addresses cannot become
// valid empty strings. Both envelopes use the same strict record decoder.
type normalizeRecord struct {
	Id         interface{} `json:"id"`
	RawAddress *string     `json:"raw_address"`
}

type normalizeRequest struct {
	Records []normalizeRecord `json:"records"`
	array   bool
}

func (req *normalizeRequest) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || (data[0] != '[' && data[0] != '{') {
		return errors.New("expected a records object or array")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	d.UseNumber()
	if data[0] == '[' {
		req.array = true
		return d.Decode(&req.Records)
	}
	// An alias avoids recursively invoking UnmarshalJSON.
	type objectRequest normalizeRequest
	return d.Decode((*objectRequest)(req))
}
