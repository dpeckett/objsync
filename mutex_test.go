/* SPDX-License-Identifier: MPL-2.0
 *
 * Copyright (c) 2024 Damian Peckett <damian@pecke.tt>
 *
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/.
 */

package objsync_test

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bucket-sailor/objsync"
	"github.com/bucket-sailor/objsync/provider/s3"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

func TestMutex(t *testing.T) {
	endpointURL := os.Getenv("AWS_ENDPOINT_URL_S3")
	accessKeyID := os.Getenv("AWS_ACCESS_KEY_ID")
	secretAccessKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	bucket := os.Getenv("BUCKET")

	key := fmt.Sprintf("test-%d.lock", time.Now().UnixNano())

	ctx := context.Background()
	p, err := s3.NewProvider(ctx, endpointURL, "", accessKeyID, secretAccessKey)
	require.NoError(t, err)

	// Make sure the lock object exists.
	_, err = p.AtomicUpdateObject(ctx, bucket, key, func(_ string, _ []byte) ([]byte, error) {
		return []byte("{}"), nil
	})
	require.NoError(t, err)

	// Two threads competing for the mutex.
	var lockCounter int32
	var lastFencingToken int64
	g, ctx := errgroup.WithContext(ctx)
	for i := 0; i < 3; i++ {
		g.Go(func() error {
			mu, err := objsync.NewMutex(p, bucket, key)
			if err != nil {
				return err
			}

			for j := 0; j < 5; j++ {
				fencingToken, err := mu.Lock(ctx, 5*time.Second)
				if err != nil {
					return fmt.Errorf("lock: %w", err)
				}

				// Verify the mutex is held by only one goroutine.
				if n := atomic.AddInt32(&lockCounter, 1); n > 1 {
					return fmt.Errorf("lock is held by %d goroutines (fencingToken: %d)", n, fencingToken)
				}

				// Is the fencing token monotonically increasing?
				if fencingToken <= lastFencingToken {
					return fmt.Errorf("fencing token is not monotonically increasing: %d <= %d", fencingToken, lastFencingToken)
				}
				lastFencingToken = fencingToken

				// Simulate some work.
				time.Sleep(time.Millisecond*10 + time.Duration(rand.Intn(5))*time.Millisecond)

				atomic.AddInt32(&lockCounter, -1)

				if err := mu.Unlock(ctx); err != nil {
					return fmt.Errorf("unlock: %w", err)
				}
			}

			return nil
		})
	}

	require.NoError(t, g.Wait())
}
