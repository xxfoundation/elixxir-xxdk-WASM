////////////////////////////////////////////////////////////////////////////////
// Copyright © 2022 xx foundation                                             //
//                                                                            //
// Use of this source code is governed by a license that can be found in the  //
// LICENSE file.                                                              //
////////////////////////////////////////////////////////////////////////////////

//go:build js && wasm

// This file contains several generic IndexedDB helper functions that
// may be useful for any IndexedDB implementations.

package impl

import (
	"context"
	"encoding/base64"
	"github.com/hack-pad/go-indexeddb/idb"
	"github.com/pkg/errors"
	jww "github.com/spf13/jwalterweatherman"
	"gitlab.com/elixxir/xxdk-wasm/utils"
	"syscall/js"
	"time"
)

const (
	// dbTimeout is the global timeout for operations with the storage
	// [context.Context].
	dbTimeout = time.Second

	// ErrDoesNotExist is an error string for got undefined on Get operations.
	ErrDoesNotExist = "result is undefined"

	// ErrUniqueConstraint is an error string for failed uniqueness inserts.
	ErrUniqueConstraint = "at least one key does not satisfy the uniqueness requirements"
)

// NewContext builds a context for indexedDb operations.
func NewContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), dbTimeout)
}

// EncodeBytes returns the proper IndexedDb encoding for a byte slice into js.Value.
func EncodeBytes(input []byte) js.Value {
	return js.ValueOf(base64.StdEncoding.EncodeToString(input))
}

// Get is a generic helper for getting values from the given [idb.ObjectStore].
// Only usable by primary key.
func Get(db *idb.Database, objectStoreName string, key js.Value) (js.Value, error) {
	parentErr := errors.Errorf("failed to Get %s", objectStoreName)

	// Prepare the Transaction
	txn, err := db.Transaction(idb.TransactionReadOnly, objectStoreName)
	if err != nil {
		return js.Undefined(), errors.WithMessagef(parentErr,
			"Unable to create Transaction: %+v", err)
	}
	store, err := txn.ObjectStore(objectStoreName)
	if err != nil {
		return js.Undefined(), errors.WithMessagef(parentErr,
			"Unable to get ObjectStore: %+v", err)
	}

	// Perform the operation
	getRequest, err := store.Get(key)
	if err != nil {
		return js.Undefined(), errors.WithMessagef(parentErr,
			"Unable to Get from ObjectStore: %+v", err)
	}

	// Wait for the operation to return
	ctx, cancel := NewContext()
	resultObj, err := getRequest.Await(ctx)
	cancel()
	if err != nil {
		return js.Undefined(), errors.WithMessagef(parentErr,
			"Unable to get from ObjectStore: %+v", err)
	} else if resultObj.IsUndefined() {
		return js.Undefined(), errors.WithMessagef(parentErr,
			"Unable to get from ObjectStore: %s", ErrDoesNotExist)
	}

	// Process result into string
	jww.DEBUG.Printf("Got from %s: %s",
		objectStoreName, utils.JsToJson(resultObj))
	return resultObj, nil
}

// GetAll is a generic helper for getting all values from the given [idb.ObjectStore].
func GetAll(db *idb.Database, objectStoreName string) ([]js.Value, error) {
	parentErr := errors.Errorf("failed to GetAll %s", objectStoreName)

	// Prepare the Transaction
	txn, err := db.Transaction(idb.TransactionReadWrite, objectStoreName)
	if err != nil {
		return nil, errors.WithMessagef(parentErr,
			"Unable to create Transaction: %+v", err)
	}
	store, err := txn.ObjectStore(objectStoreName)
	if err != nil {
		return nil, errors.WithMessagef(parentErr,
			"Unable to get ObjectStore: %+v", err)
	}

	// Perform the operation
	result := make([]js.Value, 0)
	cursorRequest, err := store.OpenCursor(idb.CursorNext)
	if err != nil {
		return nil, errors.WithMessagef(parentErr, "Unable to open Cursor: %+v", err)
	}
	ctx, cancel := NewContext()
	err = cursorRequest.Iter(ctx,
		func(cursor *idb.CursorWithValue) error {
			row, err := cursor.Value()
			if err != nil {
				return err
			}
			result = append(result, row)
			return nil
		})
	cancel()
	if err != nil {
		return nil, errors.WithMessagef(parentErr, err.Error())
	}
	return result, nil
}

// GetIndex is a generic helper for getting values from the given
// [idb.ObjectStore] using the given [idb.Index].
func GetIndex(db *idb.Database, objectStoreName,
	indexName string, key js.Value) (js.Value, error) {
	parentErr := errors.Errorf("failed to GetIndex %s/%s",
		objectStoreName, indexName)

	// Prepare the Transaction
	txn, err := db.Transaction(idb.TransactionReadOnly, objectStoreName)
	if err != nil {
		return js.Undefined(), errors.WithMessagef(parentErr,
			"Unable to create Transaction: %+v", err)
	}
	store, err := txn.ObjectStore(objectStoreName)
	if err != nil {
		return js.Undefined(), errors.WithMessagef(parentErr,
			"Unable to get ObjectStore: %+v", err)
	}
	idx, err := store.Index(indexName)
	if err != nil {
		return js.Undefined(), errors.WithMessagef(parentErr,
			"Unable to get Index: %+v", err)
	}

	// Perform the operation
	getRequest, err := idx.Get(key)
	if err != nil {
		return js.Undefined(), errors.WithMessagef(parentErr,
			"Unable to Get from ObjectStore: %+v", err)
	}

	// Wait for the operation to return
	ctx, cancel := NewContext()
	resultObj, err := getRequest.Await(ctx)
	cancel()
	if err != nil {
		return js.Undefined(), errors.WithMessagef(parentErr,
			"Unable to get from ObjectStore: %+v", err)
	} else if resultObj.IsUndefined() {
		return js.Undefined(), errors.WithMessagef(parentErr,
			"Unable to get from ObjectStore: %s", ErrDoesNotExist)
	}

	// Process result into string
	jww.DEBUG.Printf("Got from %s/%s: %s",
		objectStoreName, indexName, utils.JsToJson(resultObj))
	return resultObj, nil
}

// Put is a generic helper for putting values into the given [idb.ObjectStore].
// Equivalent to insert if not exists else update.
func Put(db *idb.Database, objectStoreName string, value js.Value) (js.Value, error) {
	// Prepare the Transaction
	txn, err := db.Transaction(idb.TransactionReadWrite, objectStoreName)
	if err != nil {
		return js.Undefined(), errors.Errorf("Unable to create Transaction: %+v", err)
	}
	store, err := txn.ObjectStore(objectStoreName)
	if err != nil {
		return js.Undefined(), errors.Errorf("Unable to get ObjectStore: %+v", err)
	}

	// Perform the operation
	request, err := store.Put(value)
	if err != nil {
		return js.Undefined(), errors.Errorf("Unable to Put: %+v", err)
	}

	// Wait for the operation to return
	ctx, cancel := NewContext()
	result, err := request.Await(ctx)
	cancel()
	if err != nil {
		return js.Undefined(), errors.Errorf("Putting value failed: %+v", err)
	}
	jww.DEBUG.Printf("Successfully put value in %s: %s",
		objectStoreName, utils.JsToJson(value))
	return result, nil
}

// Delete is a generic helper for removing values from the given
// [idb.ObjectStore]. Only usable by primary key.
func Delete(db *idb.Database, objectStoreName string, key js.Value) error {
	parentErr := errors.Errorf("failed to Delete %s", objectStoreName)

	// Prepare the Transaction
	txn, err := db.Transaction(idb.TransactionReadWrite, objectStoreName)
	if err != nil {
		return errors.WithMessagef(parentErr,
			"Unable to create Transaction: %+v", err)
	}
	store, err := txn.ObjectStore(objectStoreName)
	if err != nil {
		return errors.WithMessagef(parentErr,
			"Unable to get ObjectStore: %+v", err)
	}

	// Perform the operation
	_, err = store.Delete(key)
	if err != nil {
		return errors.WithMessagef(parentErr,
			"Unable to Delete from ObjectStore: %+v", err)
	}

	// Wait for the operation to return
	ctx, cancel := NewContext()
	err = txn.Await(ctx)
	cancel()
	if err != nil {
		return errors.WithMessagef(parentErr,
			"Unable to Delete from ObjectStore: %+v", err)
	}
	jww.DEBUG.Printf("Successfully deleted value at %s/%s",
		objectStoreName, utils.JsToJson(key))
	return nil
}

// DeleteIndex is a generic helper for removing values from the
// given [idb.ObjectStore] using the given [idb.Index]. Requires passing
// in the name of the primary key for the store.
func DeleteIndex(db *idb.Database, objectStoreName,
	indexName, pkeyName string, key js.Value) error {
	parentErr := errors.Errorf("failed to DeleteIndex %s/%s", objectStoreName, key)

	value, err := GetIndex(db, objectStoreName, indexName, key)
	if err != nil {
		return errors.WithMessagef(parentErr, "%+v", err)
	}

	err = Delete(db, objectStoreName, value.Get(pkeyName))
	if err != nil {
		return errors.WithMessagef(parentErr, "%+v", err)
	}

	jww.DEBUG.Printf("Successfully deleted value at %s/%s/%s",
		objectStoreName, indexName, utils.JsToJson(key))
	return nil
}

// Dump returns the given [idb.ObjectStore] contents to string slice for
// testing and debugging purposes.
func Dump(db *idb.Database, objectStoreName string) ([]string, error) {
	parentErr := errors.Errorf("failed to Dump %s", objectStoreName)

	txn, err := db.Transaction(idb.TransactionReadOnly, objectStoreName)
	if err != nil {
		return nil, errors.WithMessagef(parentErr,
			"Unable to create Transaction: %+v", err)
	}
	store, err := txn.ObjectStore(objectStoreName)
	if err != nil {
		return nil, errors.WithMessagef(parentErr,
			"Unable to get ObjectStore: %+v", err)
	}
	cursorRequest, err := store.OpenCursor(idb.CursorNext)
	if err != nil {
		return nil, errors.WithMessagef(parentErr,
			"Unable to open Cursor: %+v", err)
	}

	// Run the query
	jww.DEBUG.Printf("%s values:", objectStoreName)
	results := make([]string, 0)
	ctx, cancel := NewContext()
	err = cursorRequest.Iter(ctx,
		func(cursor *idb.CursorWithValue) error {
			value, err := cursor.Value()
			if err != nil {
				return err
			}
			valueStr := utils.JsToJson(value)
			results = append(results, valueStr)
			jww.DEBUG.Printf("- %v", valueStr)
			return nil
		})
	cancel()
	if err != nil {
		return nil, errors.WithMessagef(parentErr,
			"Unable to dump ObjectStore: %+v", err)
	}
	return results, nil
}
