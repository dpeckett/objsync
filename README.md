# Objsync

A distributed synchronization library for Golang built on top of object storage.

Why? See my whitepaper: [All You Need Is S3](https://www.bucket-sailor.com/posts/all-you-need-is-s3/).

## Features

* Shared, multi-process, multi-host locks.
* No additional infrastructure required.
* Automatic expiration in the event of a failure.
* Fencing token support, to prevent stale locks.

## Limitations

* Not all object Storage providers support conditional PUTs which are required to implement locking (e.g. AWS S3).
* Due to object mutation rate limits, it's currently limited to 1 lock per second on most providers (but not Ceph RGW). This makes it more useful for leader election than for fine-grained locking.

## Supported Providers

* Cloudflare R2.
* Ceph RGW (and any providers built on top of it).
* Google Cloud Storage.

*Note: This is far from an exhaustive list, and I'm happy to accept PRs for more providers/documentation.*

## Usage

```go
func main() {
	p, err := s3.NewProvider(ctx, endpointURL, "", accessKeyID, secretAccessKey)
	if err != nil {
		panic(err)
	}

	mu, err := objsync.NewMutex(p, bucket, key)
	if err != nil {
		panic(err)
	}

	fencingToken, err := mu.Lock(ctx, 5*time.Second)
	if err != nil {
		panic(err)
	}

	// Do something with the mutex, remember to pass along the fencing token.

	if err := mu.Unlock(ctx); err != nil {
		panic(err)
	}
}
```

## Contribution Ideas

* Add support for more object storage providers.
* Increase the lock rate limit by using a separate object for each locking attempt (and the necessary cleanup etc).

## License

Objsync is licensed under the Mozilla Public License 2.0, see [LICENSE](LICENSE).
