package traffic

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

var defaultCollector = NewCollector()

func NewCollector() *Collector {
	return &Collector{
		bucketSeconds: defaultBucketSeconds,
		flushInterval: defaultFlushInterval,
		retentionDays: defaultRetentionDays,
		now:           time.Now,
	}
}

func Init() {
	defaultCollector.Init()
}

func Record(sample Sample) {
	defaultCollector.Record(sample)
}

func QuerySummary(startTs int64, endTs int64) (Summary, error) {
	return defaultCollector.QuerySummary(startTs, endTs)
}

func QueryBreakdown(startTs int64, endTs int64, dimension string) ([]DimensionSummary, error) {
	return defaultCollector.QueryBreakdown(startTs, endTs, dimension)
}

func QuerySeries(startTs int64, endTs int64) ([]BucketSummary, error) {
	return defaultCollector.QuerySeries(startTs, endTs)
}

func ResetForTest() {
	defaultCollector = NewCollector()
}

func (c *Collector) Init() {
	if c == nil {
		return
	}
	c.startOnce.Do(func() {
		go c.flushLoop()
	})
}

func (c *Collector) Record(sample Sample) {
	if c == nil {
		return
	}
	sample.ModelName = strings.TrimSpace(sample.ModelName)
	sample.Group = strings.TrimSpace(sample.Group)
	if sample.ModelName == "" || sample.ChannelID <= 0 {
		return
	}
	if sample.Group == "" {
		sample.Group = "default"
	}
	if sample.RequestBytes < 0 {
		sample.RequestBytes = 0
	}
	if sample.ResponseBytes < 0 {
		sample.ResponseBytes = 0
	}

	key := bucketKey{
		modelName: sample.ModelName,
		group:     sample.Group,
		channelID: sample.ChannelID,
		proxyID:   sample.ProxyID,
		bucketTs:  c.bucketStart(c.now().Unix()),
	}
	actual, _ := c.buckets.LoadOrStore(key, &atomicBucket{})
	actual.(*atomicBucket).add(sample)
}

func (c *Collector) QuerySummary(startTs int64, endTs int64) (Summary, error) {
	rows, err := c.queryCounters(startTs, endTs)
	if err != nil {
		return Summary{}, err
	}
	total := Summary{}
	for _, value := range rows {
		total.RequestCount += value.requestCount
		total.RequestBytes += value.requestBytes
		total.ResponseBytes += value.responseBytes
		total.TotalBytes += value.totalBytes
	}
	return total, nil
}

func (c *Collector) QueryBreakdown(startTs int64, endTs int64, dimension string) ([]DimensionSummary, error) {
	rows, err := c.queryCounters(startTs, endTs)
	if err != nil {
		return nil, err
	}
	merged := map[string]DimensionSummary{}
	for key, value := range rows {
		if value.requestCount == 0 {
			continue
		}
		dimensionKey, dimensionID := trafficDimensionKey(key, dimension)
		item := merged[dimensionKey]
		item.DimensionID = dimensionID
		item.DimensionKey = dimensionKey
		item.RequestCount += value.requestCount
		item.RequestBytes += value.requestBytes
		item.ResponseBytes += value.responseBytes
		item.TotalBytes += value.totalBytes
		merged[dimensionKey] = item
	}

	out := make([]DimensionSummary, 0, len(merged))
	for _, item := range merged {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].TotalBytes > out[j].TotalBytes
	})
	return out, nil
}

func (c *Collector) QuerySeries(startTs int64, endTs int64) ([]BucketSummary, error) {
	rows, err := c.queryCounters(startTs, endTs)
	if err != nil {
		return nil, err
	}
	merged := map[int64]Summary{}
	for key, value := range rows {
		if value.requestCount == 0 {
			continue
		}
		item := merged[key.bucketTs]
		item.RequestCount += value.requestCount
		item.RequestBytes += value.requestBytes
		item.ResponseBytes += value.responseBytes
		item.TotalBytes += value.totalBytes
		merged[key.bucketTs] = item
	}
	out := make([]BucketSummary, 0, len(merged))
	for bucketTs, item := range merged {
		out = append(out, BucketSummary{
			BucketTs: bucketTs,
			Summary:  item,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].BucketTs < out[j].BucketTs
	})
	return out, nil
}

func (c *Collector) queryCounters(startTs int64, endTs int64) (map[bucketKey]counters, error) {
	result := map[bucketKey]counters{}
	if c == nil {
		return result, nil
	}
	if endTs <= 0 {
		endTs = c.now().Unix()
	}
	if startTs <= 0 || startTs > endTs {
		startTs = endTs - 86400
	}
	dbStart := c.bucketStart(startTs)
	rows, err := model.ListModelGatewayTrafficMetrics(dbStart, endTs)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		key := bucketKey{
			modelName: row.ModelName,
			group:     row.Group,
			channelID: row.ChannelID,
			proxyID:   row.ProxyID,
			bucketTs:  row.BucketTs,
		}
		mergeCounters(result, key, counters{
			requestCount:  row.RequestCount,
			requestBytes:  row.RequestBytes,
			responseBytes: row.ResponseBytes,
			totalBytes:    row.TotalBytes,
		})
	}

	c.buckets.Range(func(key any, value any) bool {
		k := key.(bucketKey)
		if k.bucketTs < dbStart || k.bucketTs > endTs {
			return true
		}
		mergeCounters(result, k, value.(*atomicBucket).snapshot())
		return true
	})
	return result, nil
}

func (c *Collector) flushLoop() {
	for {
		time.Sleep(c.flushInterval)
		c.flushCompletedBuckets()
		c.cleanupExpiredMetrics()
	}
}

func (c *Collector) flushCompletedBuckets() {
	currentBucket := c.bucketStart(c.now().Unix())
	c.buckets.Range(func(key any, value any) bool {
		k := key.(bucketKey)
		if k.bucketTs >= currentBucket {
			return true
		}
		bucket := value.(*atomicBucket)
		drained := bucket.drain()
		if drained.requestCount == 0 {
			c.deleteOldEmptyBucket(k, key)
			return true
		}
		err := model.UpsertModelGatewayTrafficMetric(&model.ModelGatewayTrafficMetric{
			ModelName:     k.modelName,
			Group:         k.group,
			ChannelID:     k.channelID,
			ProxyID:       k.proxyID,
			BucketTs:      k.bucketTs,
			RequestCount:  drained.requestCount,
			RequestBytes:  drained.requestBytes,
			ResponseBytes: drained.responseBytes,
			TotalBytes:    drained.totalBytes,
		})
		if err != nil {
			bucket.addCounters(drained)
			common.SysError(fmt.Sprintf("failed to flush model gateway traffic bucket model=%s group=%s channel=%d bucket=%d: %s", k.modelName, k.group, k.channelID, k.bucketTs, err.Error()))
			return true
		}
		c.deleteOldEmptyBucket(k, key)
		return true
	})
}

func (c *Collector) cleanupExpiredMetrics() {
	if c.retentionDays <= 0 {
		return
	}
	cutoff := c.now().Add(-time.Duration(c.retentionDays) * 24 * time.Hour).Unix()
	if err := model.DeleteModelGatewayTrafficMetricsBefore(cutoff); err != nil {
		common.SysError("failed to cleanup model gateway traffic metrics: " + err.Error())
	}
}

func (c *Collector) deleteOldEmptyBucket(k bucketKey, rawKey any) {
	if k.bucketTs < c.bucketStart(c.now().Add(-24*time.Hour).Unix()) {
		c.buckets.Delete(rawKey)
	}
}

func (c *Collector) bucketStart(ts int64) int64 {
	if c == nil || c.bucketSeconds <= 0 {
		return ts - (ts % defaultBucketSeconds)
	}
	return ts - (ts % c.bucketSeconds)
}

func mergeCounters(target map[bucketKey]counters, key bucketKey, value counters) {
	if value.requestCount == 0 {
		return
	}
	current := target[key]
	current.requestCount += value.requestCount
	current.requestBytes += value.requestBytes
	current.responseBytes += value.responseBytes
	current.totalBytes += value.totalBytes
	target[key] = current
}

func trafficDimensionKey(key bucketKey, dimension string) (string, int) {
	switch strings.ToLower(strings.TrimSpace(dimension)) {
	case "channel":
		return fmt.Sprintf("%d", key.channelID), key.channelID
	case "model":
		return key.modelName, 0
	case "proxy":
		return fmt.Sprintf("%d", key.proxyID), key.proxyID
	default:
		return key.group, 0
	}
}
