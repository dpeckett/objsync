/* SPDX-License-Identifier: MPL-2.0
 *
 * Copyright (c) 2024 Damian Peckett <damian@pecke.tt>
 *
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/.
 */

package objsync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/bucket-sailor/objsync/provider"
	"github.com/google/uuid"
)

// Mutex is a distributed mutex.
type Mutex struct {
	provider provider.Provider
	bucket   string
	key      string
	id       string
	etag     string
}

// The JSON content of the mutex object.
type mutexContent struct {
	ID      string     `json:"id,omitempty"`
	Expires *time.Time `json:"expires,omitempty"`
	Fence   int64      `json:"fence,omitempty"`
}

// NewMutex creates a new distributed mutex.
func NewMutex(p provider.Provider, bucket, key string) *Mutex {
	return &Mutex{
		provider: p,
		bucket:   bucket,
		key:      key,
		id:       uuid.New().String(),
	}
}

// Lock acquires the mutex. It blocks until the mutex is available.
// Length is the maximum duration the lock will be held for.
func (mu *Mutex) Lock(ctx context.Context, length time.Duration) (int64, error) {
	var fencingToken int64

	err := retry.Do(
		func() error {
			var ok bool
			var err error
			ok, fencingToken, err = mu.TryLock(ctx, length)
			if err != nil {
				return retry.Unrecoverable(err)
			}

			if ok {
				return nil
			}

			return fmt.Errorf("failed to acquire lock")
		},
		retry.Context(ctx),
		retry.Attempts(0),
	)
	if err != nil {
		return -1, err
	}

	return fencingToken, nil
}

// Unlock releases the mutex (if held).
func (mu *Mutex) Unlock(ctx context.Context) error {
	if mu.etag != "" {
		err := retry.Do(
			func() error {
				_, err := mu.provider.AtomicUpdateObject(ctx, mu.bucket, mu.key, func(currentETag string, currentData []byte) ([]byte, error) {
					if currentETag != mu.etag {
						return nil, provider.ErrConflict
					}

					var content mutexContent
					if len(currentData) > 0 {
						if err := json.Unmarshal(currentData, &content); err != nil {
							return nil, err
						}

						// Clear the lock.
						content.ID = ""
						content.Expires = nil
					}

					return json.Marshal(content)
				})
				if err != nil {
					if errors.Is(err, provider.ErrConflict) {
						return nil // someone else acquired the lock in the meantime.
					}

					return retry.Unrecoverable(err)
				}

				return nil
			},
			retry.Context(ctx),
			retry.Attempts(0),
		)
		if err != nil {
			return err
		}

		mu.etag = ""
	}

	return nil
}

// TryLock attempts to acquire the mutex without blocking.
func (mu *Mutex) TryLock(ctx context.Context, expiresIn time.Duration) (bool, int64, error) {
	var errLockHeld = fmt.Errorf("lock is held")

	var newFencingToken int64
	newETag, err := mu.provider.AtomicUpdateObject(ctx, mu.bucket, mu.key, func(_ string, currentData []byte) ([]byte, error) {
		var content mutexContent
		if len(currentData) > 0 {
			if err := json.Unmarshal(currentData, &content); err != nil {
				return nil, err
			}

			if content.Expires != nil && !time.Now().After(*content.Expires) {
				return nil, errLockHeld
			}
		}

		expires := time.Now().Add(expiresIn).UTC()
		content.Expires = &expires
		content.ID = mu.id
		content.Fence++

		newFencingToken = content.Fence

		return json.Marshal(content)
	})
	if err != nil {
		if errors.Is(err, errLockHeld) || errors.Is(err, provider.ErrConflict) {
			return false, -1, nil // Lock is held.
		}

		return false, -1, err
	}

	mu.etag = newETag

	return true, newFencingToken, nil
}
