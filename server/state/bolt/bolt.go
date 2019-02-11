// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package bolt implements an AccessTracker using a file-backed database.
package bolt

import (
	"context"
	"encoding/binary"
	"errors"
	"io/ioutil"
	"os"
	"path"
	"time"

	"github.com/tsavola/gate/server"
	"github.com/tsavola/gate/server/state"
	bolt "go.etcd.io/bbolt"
)

const maxBatchDelay = time.Millisecond

var (
	nonceBucketName  = []byte("nonce")
	expireBucketName = []byte("expire")
)

var errNonceExists = errors.New("nonce already exists")

func init() {
	state.Register("bolt", driver{})
}

type config struct {
	Filename string
}

type driver struct{}

func (driver) NewConfig() interface{} {
	return new(config)
}

func (driver) Open(ctx context.Context, conf interface{}) (db state.DB, err error) {
	return open(conf.(*config).Filename)
}

type DB struct {
	state.AccessTrackerBase
	db *bolt.DB
}

// open or create a database in the given file.  If filename is empty, a
// temporary file is created.
func open(filename string) (accessTracker state.DB, err error) {
	if filename == "" {
		var dir string

		dir, err = ioutil.TempDir("", "")
		if err != nil {
			return
		}

		filename = path.Join(dir, "db")

		defer func() {
			os.Remove(filename)
			os.Remove(dir)
		}()
	}

	db, err := bolt.Open(filename, 0600, nil)
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			db.Close()
		}
	}()

	db.MaxBatchDelay = maxBatchDelay

	err = db.Update(func(tx *bolt.Tx) (err error) {
		_, err = tx.CreateBucketIfNotExists(nonceBucketName)
		if err != nil {
			return
		}

		_, err = tx.CreateBucketIfNotExists(expireBucketName)
		if err != nil {
			return
		}

		return
	})
	if err != nil {
		return
	}

	accessTracker = &DB{db: db}
	return
}

// Close the database.
func (accessTracker *DB) Close() error {
	return accessTracker.db.Close()
}

func (accessTracker *DB) TrackNonce(ctx context.Context, pri *server.PrincipalKey, nonce string, expire time.Time) error {
	var nonceKey []byte
	nonceKey = append(nonceKey, pri.KeyBytes()...)
	nonceKey = append(nonceKey, nonce...)

	expireKey := make([]byte, 8+8)
	binary.LittleEndian.PutUint64(expireKey, uint64(expire.Unix()))
	// Second half is filled inside transaction.

	return accessTracker.db.Batch(func(tx *bolt.Tx) error {
		now := uint64(time.Now().Unix())

		nonceBucket := tx.Bucket(nonceBucketName)

		if value := nonceBucket.Get(nonceKey); value != nil {
			if binary.LittleEndian.Uint64(value) >= now {
				return errNonceExists
			}
		}

		expireTime := expireKey[:8]

		if err := nonceBucket.Put(nonceKey, expireTime); err != nil {
			return err
		}

		expireBucket := tx.Bucket(expireBucketName)

		seq, err := expireBucket.NextSequence()
		if err != nil {
			return err
		}

		binary.LittleEndian.PutUint64(expireKey[8:], seq)

		if err := expireBucket.Put(expireKey, nonceKey); err != nil {
			return err
		}

		expireCursor := expireBucket.Cursor()

		for expireKey, nonceKey := expireCursor.First(); expireKey != nil; expireKey, nonceKey = expireCursor.Next() {
			if binary.LittleEndian.Uint64(expireKey) >= now {
				break
			}

			if err := nonceBucket.Delete(nonceKey); err != nil {
				return err
			}

			if err := expireCursor.Delete(); err != nil {
				return err
			}
		}

		return nil
	})
}
