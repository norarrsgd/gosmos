package repository

import "time"

// Entity is the minimal document contract.
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
