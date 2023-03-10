////////////////////////////////////////////////////////////////////////////////
// Copyright © 2022 xx foundation                                             //
//                                                                            //
// Use of this source code is governed by a license that can be found in the  //
// LICENSE file.                                                              //
////////////////////////////////////////////////////////////////////////////////

//go:build js && wasm

package channels

import (
	"crypto/ed25519"
	"encoding/json"
	"github.com/pkg/errors"
	"time"

	jww "github.com/spf13/jwalterweatherman"
	"gitlab.com/elixxir/client/v4/channels"

	cryptoChannel "gitlab.com/elixxir/crypto/channel"
	"gitlab.com/elixxir/crypto/message"
	"gitlab.com/elixxir/xxdk-wasm/storage"
	"gitlab.com/elixxir/xxdk-wasm/worker"
	"gitlab.com/xx_network/primitives/id"
)

// MessageReceivedCallback is called any time a message is received or updated.
//
// update is true if the row is old and was edited.
type MessageReceivedCallback func(uuid uint64, channelID *id.ID, update bool)

// DeletedMessageCallback is called any time a message is deleted.
type DeletedMessageCallback func(messageID message.ID)

// MutedUserCallback is called any time a user is muted or unmuted. unmute is
// true if the user has been unmuted and false if they have been muted.
type MutedUserCallback func(
	channelID *id.ID, pubKey ed25519.PublicKey, unmute bool)

// NewWASMEventModelBuilder returns an EventModelBuilder which allows
// the channel manager to define the path but the callback is the same
// across the board.
func NewWASMEventModelBuilder(wasmJsPath string,
	encryption cryptoChannel.Cipher, messageReceivedCB MessageReceivedCallback,
	deletedMessageCB DeletedMessageCallback,
	mutedUserCB MutedUserCallback) channels.EventModelBuilder {
	fn := func(path string) (channels.EventModel, error) {
		return NewWASMEventModel(path, wasmJsPath, encryption,
			messageReceivedCB, deletedMessageCB, mutedUserCB)
	}
	return fn
}

// NewWASMEventModelMessage is JSON marshalled and sent to the worker for
// [NewWASMEventModel].
type NewWASMEventModelMessage struct {
	Path           string `json:"path"`
	EncryptionJSON string `json:"encryptionJSON"`
}

// NewWASMEventModel returns a [channels.EventModel] backed by a wasmModel.
// The name should be a base64 encoding of the users public key.
func NewWASMEventModel(path, wasmJsPath string, encryption cryptoChannel.Cipher,
	messageReceivedCB MessageReceivedCallback,
	deletedMessageCB DeletedMessageCallback, mutedUserCB MutedUserCallback) (
	channels.EventModel, error) {

	wm, err := worker.NewManager(wasmJsPath, "channelsIndexedDb", true)
	if err != nil {
		return nil, err
	}

	// Register handler to manage messages for the MessageReceivedCallback
	wm.RegisterCallback(MessageReceivedCallbackTag,
		messageReceivedCallbackHandler(messageReceivedCB))

	// Register handler to manage messages for the DeletedMessageCallback
	wm.RegisterCallback(DeletedMessageCallbackTag,
		deletedMessageCallbackHandler(deletedMessageCB))

	// Register handler to manage messages for the MutedUserCallback
	wm.RegisterCallback(MutedUserCallbackTag,
		mutedUserCallbackHandler(mutedUserCB))

	// Register handler to manage checking encryption status from local storage
	wm.RegisterCallback(EncryptionStatusTag, checkDbEncryptionStatusHandler(wm))

	// Register handler to manage the storage of the database name
	wm.RegisterCallback(StoreDatabaseNameTag, storeDatabaseNameHandler(wm))

	encryptionJSON, err := json.Marshal(encryption)
	if err != nil {
		return nil, err
	}

	msg := NewWASMEventModelMessage{
		Path:           path,
		EncryptionJSON: string(encryptionJSON),
	}

	payload, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}

	errChan := make(chan string)
	wm.SendMessage(NewWASMEventModelTag, payload,
		func(data []byte) { errChan <- string(data) })

	select {
	case workerErr := <-errChan:
		if workerErr != "" {
			return nil, errors.New(workerErr)
		}
	case <-time.After(worker.ResponseTimeout):
		return nil, errors.Errorf("timed out after %s waiting for indexedDB "+
			"database in worker to initialize", worker.ResponseTimeout)
	}

	return &wasmModel{wm}, nil
}

// MessageReceivedCallbackMessage is JSON marshalled and received from the
// worker for the [MessageReceivedCallback] callback.
type MessageReceivedCallbackMessage struct {
	UUID      uint64 `json:"uuid"`
	ChannelID *id.ID `json:"channelID"`
	Update    bool   `json:"update"`
}

// messageReceivedCallbackHandler returns a handler to manage messages for the
// MessageReceivedCallback.
func messageReceivedCallbackHandler(cb MessageReceivedCallback) func(data []byte) {
	return func(data []byte) {
		var msg MessageReceivedCallbackMessage
		err := json.Unmarshal(data, &msg)
		if err != nil {
			jww.ERROR.Printf(
				"Failed to JSON unmarshal %T from worker: %+v", msg, err)
			return
		}

		cb(msg.UUID, msg.ChannelID, msg.Update)
	}
}

// deletedMessageCallbackHandler returns a handler to manage messages for the
// DeletedMessageCallback.
func deletedMessageCallbackHandler(cb DeletedMessageCallback) func(data []byte) {
	return func(data []byte) {
		messageID, err := message.UnmarshalID(data)
		if err != nil {
			jww.ERROR.Printf(
				"Failed to JSON unmarshal message ID from worker: %+v", err)
		}

		cb(messageID)
	}
}

// mutedUserCallbackHandler returns a handler to manage messages for the
// MutedUserCallback.
func mutedUserCallbackHandler(cb MutedUserCallback) func(data []byte) {
	return func(data []byte) {
		var msg MuteUserMessage
		err := json.Unmarshal(data, &msg)
		if err != nil {
			jww.ERROR.Printf(
				"Failed to JSON unmarshal %T from worker: %+v", msg, err)
			return
		}

		cb(msg.ChannelID, msg.PubKey, msg.Unmute)
	}
}

// EncryptionStatusMessage is JSON marshalled and received from the worker when
// the database checks if it is encrypted.
type EncryptionStatusMessage struct {
	DatabaseName     string `json:"databaseName"`
	EncryptionStatus bool   `json:"encryptionStatus"`
}

// EncryptionStatusReply is JSON marshalled and sent to the worker is response
// to the [EncryptionStatusMessage].
type EncryptionStatusReply struct {
	EncryptionStatus bool   `json:"encryptionStatus"`
	Error            string `json:"error"`
}

// checkDbEncryptionStatusHandler returns a handler to manage checking
// encryption status from local storage.
func checkDbEncryptionStatusHandler(
	wh *worker.Manager) func(data []byte) {
	return func(data []byte) {
		// Unmarshal received message
		var msg EncryptionStatusMessage
		err := json.Unmarshal(data, &msg)
		if err != nil {
			jww.ERROR.Printf("Failed to JSON unmarshal "+
				"EncryptionStatusMessage message from worker: %+v", err)
			return
		}

		// Pass message values to storage
		loadedEncryptionStatus, err := storage.StoreIndexedDbEncryptionStatus(
			msg.DatabaseName, msg.EncryptionStatus)
		var reply EncryptionStatusReply
		if err != nil {
			reply.Error = err.Error()
		} else {
			reply.EncryptionStatus = loadedEncryptionStatus
		}

		// Return response
		statusData, err := json.Marshal(reply)
		if err != nil {
			jww.ERROR.Printf(
				"Failed to JSON marshal EncryptionStatusReply: %+v", err)
			return
		}

		wh.SendMessage(EncryptionStatusTag, statusData, nil)
	}
}

// storeDatabaseNameHandler returns a handler that stores the database name to
// storage when it is received from the worker.
func storeDatabaseNameHandler(
	wh *worker.Manager) func(data []byte) {
	return func(data []byte) {
		var returnData []byte

		// Get the database name and save it to storage
		if err := storage.StoreIndexedDb(string(data)); err != nil {
			returnData = []byte(err.Error())
		}

		wh.SendMessage(StoreDatabaseNameTag, returnData, nil)
	}
}
