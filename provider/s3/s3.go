/* SPDX-License-Identifier: MPL-2.0
 *
 * Copyright (c) 2024 Damian Peckett <damian@pecke.tt>
 *
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/.
 */

package s3

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsretry "github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	smithymiddleware "github.com/aws/smithy-go/middleware"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/bucket-sailor/objsync/provider"
)

type Provider struct {
	client *s3.Client
}

func NewProvider(ctx context.Context, endpointURL, region, accessKeyID, secretAccessKey string) (provider.Provider, error) {
	customResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...any) (aws.Endpoint, error) {
		return aws.Endpoint{
			URL:           endpointURL,
			SigningRegion: region,
		}, nil
	})

	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithEndpointResolverWithOptions(customResolver),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, "")),
		config.WithRegion(region))
	if err != nil {
		return nil, err
	}

	client := s3.NewFromConfig(cfg, func(options *s3.Options) {
		options.UsePathStyle = true
		options.Retryer = awsretry.AddWithMaxAttempts(awsretry.NewStandard(), 0)
	})

	return &Provider{client: client}, nil
}

func (p *Provider) AtomicUpdateObject(ctx context.Context, bucket, key string, fn provider.UpdateObjectFunc) (string, error) {
	getResp, err := p.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) && apiErr.ErrorCode() != "NoSuchKey" {
			return "", err
		}
	}

	var currentETag string
	if getResp != nil && getResp.ETag != nil {
		currentETag = strings.Trim(*getResp.ETag, "\"")
	}

	var currentData []byte
	if getResp != nil {
		defer getResp.Body.Close()

		currentData, err = io.ReadAll(getResp.Body)
		if err != nil {
			return "", err
		}
	}

	newData, err := fn(currentETag, currentData)
	if err != nil {
		return "", err
	}

	putResp, err := p.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(newData),
		ContentType: aws.String("application/json"),
	}, func(options *s3.Options) {
		options.APIOptions = []func(*smithymiddleware.Stack) error{
			func(stack *smithymiddleware.Stack) error {
				if currentETag != "" {
					// You might ask why the ETag is not wrapped in quotes like the spec says it should be.
					// This is because of a bug in Ceph's S3 API implementation: https://tracker.ceph.com/issues/64439
					// Other providers seem to be tolerant of this, so we'll just go with it.
					return smithyhttp.AddHeaderValue("If-Match", currentETag)(stack)
				}

				return nil
			},
		}
	})
	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) && apiErr.ErrorCode() == "PreconditionFailed" {
			return "", provider.ErrConflict
		}

		return "", err
	}

	return strings.Trim(*putResp.ETag, "\""), nil
}
