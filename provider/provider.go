/* SPDX-License-Identifier: MPL-2.0
 *
 * Copyright (c) 2024 Damian Peckett <damian@pecke.tt>
 *
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/.
 */

package provider

import (
	"context"
	"fmt"
)

// ErrConflict is returned when a write conflict is detected,
// this is expected when multiple clients are racing to acquire a mutex.
var ErrConflict = fmt.Errorf("write conflict")

type UpdateObjectFunc func(string, []byte) ([]byte, error)

type Provider interface {
	AtomicUpdateObject(ctx context.Context, bucket, key string, fn UpdateObjectFunc) (string, error)
}
