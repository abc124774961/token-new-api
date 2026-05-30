package traffic

import (
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultBucketSeconds = int64(3600)
	defaultFlushInterval = 5 * time.Minute
	defaultRetentionDays = 90
)

type Sample struct {
	ModelName     string
	Group         string
	ChannelID     int
	ProxyID       int
	RequestBytes  int64
	ResponseBytes int64
}

type Summary struct {
	RequestCount  int64 `json:"request_count"`
	RequestBytes  int64 `json:"request_bytes"`
	ResponseBytes int64 `json:"response_bytes"`
	TotalBytes    int64 `json:"total_bytes"`
}

type DimensionSummary struct {
	DimensionID  int    `json:"dimension_id"`
	DimensionKey string `json:"dimension_key"`
	Summary
}

type BucketSummary struct {
	BucketTs int64 `json:"bucket_ts"`
	Summary
}

type Collector struct {
	buckets       sync.Map
	startOnce     sync.Once
	bucketSeconds int64
	flushInterval time.Duration
	retentionDays int
	now           func() time.Time
}

type bucketKey struct {
	modelName string
	group     string
	channelID int
	proxyID   int
	bucketTs  int64
}

type counters struct {
	requestCount  int64
	requestBytes  int64
	responseBytes int64
	totalBytes    int64
}

type atomicBucket struct {
	requestCount  atomic.Int64
	requestBytes  atomic.Int64
	responseBytes atomic.Int64
	totalBytes    atomic.Int64
}

func (b *atomicBucket) add(sample Sample) {
	b.requestCount.Add(1)
	if sample.RequestBytes > 0 {
		b.requestBytes.Add(sample.RequestBytes)
	}
	if sample.ResponseBytes > 0 {
		b.responseBytes.Add(sample.ResponseBytes)
	}
	total := sample.RequestBytes + sample.ResponseBytes
	if total > 0 {
		b.totalBytes.Add(total)
	}
}

func (b *atomicBucket) snapshot() counters {
	return counters{
		requestCount:  b.requestCount.Load(),
		requestBytes:  b.requestBytes.Load(),
		responseBytes: b.responseBytes.Load(),
		totalBytes:    b.totalBytes.Load(),
	}
}

func (b *atomicBucket) drain() counters {
	return counters{
		requestCount:  b.requestCount.Swap(0),
		requestBytes:  b.requestBytes.Swap(0),
		responseBytes: b.responseBytes.Swap(0),
		totalBytes:    b.totalBytes.Swap(0),
	}
}

func (b *atomicBucket) addCounters(value counters) {
	if value.requestCount != 0 {
		b.requestCount.Add(value.requestCount)
	}
	if value.requestBytes != 0 {
		b.requestBytes.Add(value.requestBytes)
	}
	if value.responseBytes != 0 {
		b.responseBytes.Add(value.responseBytes)
	}
	if value.totalBytes != 0 {
		b.totalBytes.Add(value.totalBytes)
	}
}
