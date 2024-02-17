package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/bucket-sailor/objsync"
	"github.com/bucket-sailor/objsync/provider/s3"
)

func main() {
	endpointURL := os.Getenv("AWS_ENDPOINT_URL_S3")
	accessKeyID := os.Getenv("AWS_ACCESS_KEY_ID")
	secretAccessKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	bucket := os.Getenv("BUCKET")

	key := fmt.Sprintf("test-%d.lock", time.Now().UnixNano())

	ctx := context.Background()
	p, err := s3.NewProvider(ctx, endpointURL, "", accessKeyID, secretAccessKey)
	if err != nil {
		panic(err)
	}

	// Create a mutex.
	mu := objsync.NewMutex(p, bucket, key)

	// Lock the mutex.
	fencingToken, err := mu.Lock(ctx, 5*time.Second)
	if err != nil {
		panic(err)
	}

	fmt.Println("Acquired lock with fence token:", fencingToken)

	// Do something with the mutex, pass along the fence token.

	// Unlock the mutex.
	if err := mu.Unlock(ctx); err != nil {
		panic(err)
	}
}
