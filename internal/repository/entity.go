package repository

import (
	"bytes"
	"encoding/json"
	"time"
)

// Entity is the minimal document contract. GetID must match the single
// top-level string id emitted by JSON marshaling for full-document writes.
type Entity interface{ GetID() string }
type IDSetter interface{ SetID(string) }
type Stamper interface {
	StampCreate(time.Time)
	StampModify(time.Time)
}

// PartitionKeyer overrides partition key tags for document writes.
type PartitionKeyer interface{ PartitionKeyValues() []string }

// Base provides the standard document fields and optional mutation capabilities.
type Base struct {
	ID             string     `json:"id"`
	AddedOn        time.Time  `json:"addedOn"`
	ModifiedOn     *time.Time `json:"modifiedOn,omitempty"`
	CountAggregate int64      `json:"countAggregate,omitempty"`
}

func (b Base) GetID() string              { return b.ID }
func (b *Base) SetID(id string)           { b.ID = id }
func (b *Base) StampCreate(now time.Time) { b.AddedOn = now }
func (b *Base) StampModify(now time.Time) { b.ModifiedOn = &now }

// marshalEntity binds the method-backed identity to the document identity
// consumed by Cosmos body-only write operations.
func marshalEntity[T Entity](item T) ([]byte, string, error) {
	id := item.GetID()
	body, err := json.Marshal(item)
	if err != nil {
		return nil, "", err
	}

	decoder := json.NewDecoder(bytes.NewReader(body))
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return nil, "", validation("entity JSON must be an object")
	}
	count := 0
	serializedID := ""
	for decoder.More() {
		name, err := decoder.Token()
		if err != nil {
			return nil, "", validation("entity JSON must be an object")
		}
		var value json.RawMessage
		if err = decoder.Decode(&value); err != nil {
			return nil, "", validation("entity JSON must be an object")
		}
		if name != "id" {
			continue
		}
		count++
		if count > 1 {
			return nil, "", validation("entity JSON must contain exactly one top-level id")
		}
		var decoded any
		if err = json.Unmarshal(value, &decoded); err != nil {
			return nil, "", validation("entity JSON id must be a string")
		}
		var ok bool
		serializedID, ok = decoded.(string)
		if !ok {
			return nil, "", validation("entity JSON id must be a string")
		}
	}
	if count != 1 {
		return nil, "", validation("entity JSON must contain exactly one top-level id")
	}
	if serializedID != id {
		return nil, "", validation("entity JSON id must match GetID")
	}
	return body, id, nil
}
