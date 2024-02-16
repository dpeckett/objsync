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
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/docker/docker/api/types/container"
	tc "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestMain(m *testing.M) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	ctr, err := startS3Server(ctx, logger)
	if err != nil {
		logger.Error("Failed to start S3 server", "error", err)

		if ctr != nil {
			if err := ctr.Terminate(ctx); err != nil {
				logger.Error("Failed to terminate S3 server", "error", err)
			}
		}

		os.Exit(1)
	}

	exitVal := m.Run()

	if err := ctr.Terminate(ctx); err != nil {
		logger.Error("Failed to terminate S3 server", "error", err)
		os.Exit(1)
	}

	os.Exit(exitVal)
}

func startS3Server(ctx context.Context, logger *slog.Logger) (tc.Container, error) {
	req := tc.ContainerRequest{
		Image:        "ghcr.io/bucket-sailor/picoceph:v0.2.1",
		ExposedPorts: []string{"7480/tcp"},
		WaitingFor:   wait.ForListeningPort("7480/tcp"),
		Privileged:   true,
		HostConfigModifier: func(hc *container.HostConfig) {
			hc.Binds = append(hc.Binds, "/dev:/dev")
			hc.Binds = append(hc.Binds, "/lib/modules:/lib/modules")
		},
	}

	ctr, err := tc.GenericContainer(ctx, tc.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start S3 server: %w", err)
	}

	ret, output, err := ctr.Exec(ctx, []string{"radosgw-admin", "user", "create", "--uid=admin", "--display-name=Admin User", "--caps=users=*;buckets=*;metadata=*;usage=*;zone=*"})
	if err != nil || ret != 0 {
		return ctr, fmt.Errorf("failed to configure S3 server: %w", err)
	}

	var createUser struct {
		Keys []struct {
			AccessKey string `json:"access_key"`
			SecretKey string `json:"secret_key"`
		} `json:"keys"`
	}

	// Skip some proceeding junk output.
	_, _ = io.CopyN(io.Discard, output, 8)

	if err := json.NewDecoder(output).Decode(&createUser); err != nil {
		return ctr, fmt.Errorf("failed to decode user creation output: %w", err)
	}

	if len(createUser.Keys) == 0 {
		return ctr, fmt.Errorf("no keys found in user creation output")
	}

	os.Setenv("AWS_ACCESS_KEY_ID", createUser.Keys[0].AccessKey)
	os.Setenv("AWS_SECRET_ACCESS_KEY", createUser.Keys[0].SecretKey)

	dockerHost, err := ctr.Host(ctx)
	if err != nil {
		return ctr, fmt.Errorf("failed to get S3 server host: %w", err)
	}

	s3Port, err := ctr.MappedPort(ctx, "7480")
	if err != nil {
		return ctr, fmt.Errorf("failed to get S3 server port: %w", err)
	}

	endpointURL := fmt.Sprintf("http://%s:%s", dockerHost, s3Port.Port())

	os.Setenv("AWS_ENDPOINT_URL_S3", endpointURL)

	customResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...any) (aws.Endpoint, error) {
		return aws.Endpoint{
			URL:           endpointURL,
			SigningRegion: region,
		}, nil
	})

	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithEndpointResolverWithOptions(customResolver),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(createUser.Keys[0].AccessKey, createUser.Keys[0].SecretKey, "")))
	if err != nil {
		return nil, err
	}

	client := s3.NewFromConfig(cfg, func(options *s3.Options) {
		options.UsePathStyle = true
	})

	_, err = client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String("test"),
	})
	if err != nil {
		return nil, err
	}

	os.Setenv("BUCKET", "test")

	return ctr, nil
}
