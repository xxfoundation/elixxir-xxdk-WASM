////////////////////////////////////////////////////////////////////////////////
// Copyright © 2022 xx foundation                                             //
//                                                                            //
// Use of this source code is governed by a license that can be found in the  //
// LICENSE file                                                               //
////////////////////////////////////////////////////////////////////////////////

//go:build js && wasm
// +build js,wasm

package indexedDb

import (
	"time"
)

const (
	// Text representation of primary key value (keyPath).
	pkeyName = "id"

	// Text representation of the names of the various [idb.ObjectStore].
	messageStoreName  = "messages"
	channelsStoreName = "channels"

	// Message index names.
	messageStoreChannelIndex   = "channel_id_index"
	messageStoreParentIndex    = "parent_message_id_index"
	messageStoreTimestampIndex = "timestamp_index"
	messageStorePinnedIndex    = "pinned_index"

	// Message keyPath names (must match json struct tags).
	messageStoreChannel   = "channel_id"
	messageStoreParent    = "parent_message_id"
	messageStoreTimestamp = "timestamp"
	messageStorePinned    = "pinned"
)

// Message defines the IndexedDb representation of a single Message.
// A Message belongs to one Channel.
// A Message may belong to one Message (Parent).
type Message struct {
	Id              []byte        `json:"id"` // Matches pkeyName
	SenderUsername  string        `json:"sender_username"`
	ChannelId       []byte        `json:"channel_id"`        // Index
	ParentMessageId []byte        `json:"parent_message_id"` // Index
	Timestamp       time.Time     `json:"timestamp"`         // Index
	Lease           time.Duration `json:"lease"`
	Status          uint8         `json:"status"`
	Hidden          bool          `json:"hidden"`
	Pinned          bool          `json:"pinned"` // Index
	Text            string        `json:"text"`
}

// Channel defines the IndexedDb representation of a single Channel
// A Channel has many Message.
type Channel struct {
	Id          []byte `json:"id"` // Matches pkeyName
	Name        string `json:"name"`
	Description string `json:"description"`
}