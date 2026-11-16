package query

import (
	"bytes"
	"encoding/json"
)

// UnmarshalJSON restores typed arrays required by predicate validation from the
// language-neutral corpus representation. Build performs semantic validation.
func (c *Catalyst) UnmarshalJSON(data []byte) error {
	type catalyst Catalyst
	var decoded catalyst
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if e := decoder.Decode(&decoded); e != nil {
		return e
	}
	var raw struct{ Value json.RawMessage }
	if e := json.Unmarshal(data, &raw); e != nil {
		return e
	}
	if len(raw.Value) > 0 {
		switch decoded.Operator {
		case FTScore, FTContainsAll, NotFTContainsAll, FTContainsAny, NotFTContainsAny:
			var values []string
			if e := json.Unmarshal(raw.Value, &values); e != nil {
				return e
			}
			decoded.Value = values
		case VectorDistance:
			var values []float64
			if e := json.Unmarshal(raw.Value, &values); e != nil {
				return e
			}
			decoded.Value = values
		}
	}
	*c = Catalyst(decoded)
	return nil
}
