/* SPDX-License-Identifier: MPL-2.0
 *
 * Copyright (c) 2024 Damian Peckett <damian@pecke.tt>
 *
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/.
 */

package gcs

import (
	"context"
	"errors"
	"io"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/dpeckett/objsync/provider"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

// Option is a functional option for configuring a GCS provider.
type Option func(context.Context, *Provider) error

// WithCredentialsFile specifies the path to the service account key file for authentication.
func WithCredentialsFile(ctx context.Context, credentialsFile string) Option {
	return func(ctx context.Context, p *Provider) error {
		var err error
		p.client, err = storage.NewClient(ctx, option.WithCredentialsFile(credentialsFile))
		if err != nil {
			return err
		}
		return nil
	}
}

// Provider is a GCS provider.
type Provider struct {
	client *storage.Client
}

// NewProvider initializes a new GCS provider.
func NewProvider(ctx context.Context, opts ...Option) (provider.Provider, error) {
	p := &Provider{}

	for _, opt := range opts {
		if err := opt(ctx, p); err != nil {
			return nil, err
		}
	}

	// If no client has been configured, use defaults.
	if p.client == nil {
		client, err := storage.NewClient(ctx)
		if err != nil {
			return nil, err
		}
		p.client = client
	}

	return p, nil
}

func (p *Provider) AtomicUpdateObject(ctx context.Context, bucket, key string, fn provider.UpdateObjectFunc) (string, error) {
	bkt := p.client.Bucket(bucket)

	obj := bkt.Object(key)
	attrs, err := obj.Attrs(ctx)
	if err != nil && !errors.Is(err, storage.ErrObjectNotExist) {
		return "", err
	}

	var currentGeneration int64
	if attrs != nil {
		currentGeneration = attrs.Generation
	}

	var currentETag string
	if attrs != nil {
		currentETag = strings.Trim(attrs.Etag, "\"")
	}

	reader, err := obj.NewReader(ctx)
	if err != nil && !errors.Is(err, storage.ErrObjectNotExist) {
		return "", err
	}

	var currentData []byte
	if reader != nil {
		defer reader.Close()
		currentData, err = io.ReadAll(reader)
		if err != nil {
			return "", err
		}
	}

	newData, err := fn(currentETag, currentData)
	if err != nil {
		return "", err
	}

	writer := obj.If(storage.Conditions{GenerationMatch: currentGeneration}).NewWriter(ctx)
	if _, err := writer.Write(newData); err != nil {
		return "", err
	}

	if err := writer.Close(); err != nil {
		var apiErr *googleapi.Error
		if errors.As(err, &apiErr) && apiErr.Code == 412 {
			return "", provider.ErrConflict
		}

		return "", err
	}

	newAttrs, err := obj.Attrs(ctx)
	if err != nil {
		return "", err
	}

	return strings.Trim(newAttrs.Etag, "\""), nil
}
