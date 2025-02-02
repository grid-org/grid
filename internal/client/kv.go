package client

import (
	"context"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

func (c *Client) EnsureKV(cfg jetstream.KeyValueConfig) (jetstream.KeyValue, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	bucket, err := c.js.CreateKeyValue(ctx, cfg)
	if err != nil {
		return nil, err
	}

	return bucket, nil
}

func (c *Client) GetKV(bucket, key string) (jetstream.KeyValueEntry, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	bucketObj, err := c.js.KeyValue(ctx, bucket)
	if err != nil {
		return nil, err
	}

	entry, err := bucketObj.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	return entry, nil
}

func (c *Client) PutKV(bucket, key string, value []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	bucketObj, err := c.js.KeyValue(ctx, bucket)
	if err != nil {
		return err
	}

	_, err = bucketObj.Put(ctx, key, value)
	if err != nil {
		return err
	}

	return nil
}

func (c *Client) DeleteKV(bucket, key string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	bucketObj, err := c.js.KeyValue(ctx, bucket)
	if err != nil {
		return err
	}

	err = bucketObj.Delete(ctx, key)
	if err != nil {
		return err
	}

	return nil
}

func (c *Client) ListKV(bucket string) ([]jetstream.KeyValueEntry, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	bucketObj, err := c.js.KeyValue(ctx, bucket)
	if err != nil {
		return nil, err
	}

	lister, err := bucketObj.ListKeys(ctx)
	if err != nil {
		return nil, err
	}

	var entries []jetstream.KeyValueEntry
	for name := range lister.Keys() {
		entry, err := bucketObj.Get(ctx, name)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}

	return entries, nil
}
