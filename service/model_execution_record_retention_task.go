package service

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"

	"github.com/bytedance/gopkg/util/gopool"
)

const (
	modelExecutionRecordRetentionDefaultHours           = 48
	modelExecutionRecordRetentionDefaultIntervalMinutes = 10
	modelExecutionRecordRetentionDefaultBatchSize       = 500
	modelExecutionRecordRetentionDefaultMaxBatches      = 3
	modelExecutionRecordRetentionMaxBatchSize           = 5000
	modelExecutionRecordRetentionMaxBatches             = 20
)

type modelExecutionRecordRetentionConfig struct {
	Enabled         bool
	RetentionHours  int
	IntervalMinutes int
	BatchSize       int
	MaxBatches      int
}

var (
	modelExecutionRecordRetentionOnce    sync.Once
	modelExecutionRecordRetentionRunning atomic.Bool
)

func StartModelExecutionRecordRetentionTask() {
	modelExecutionRecordRetentionOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}
		config := loadModelExecutionRecordRetentionConfig()
		if !config.Enabled {
			return
		}
		gopool.Go(func() {
			logger.LogInfo(context.Background(), fmt.Sprintf(
				"model execution record retention task started: retention=%dh interval=%dm batch_size=%d max_batches=%d",
				config.RetentionHours,
				config.IntervalMinutes,
				config.BatchSize,
				config.MaxBatches,
			))
			ticker := time.NewTicker(time.Duration(config.IntervalMinutes) * time.Minute)
			defer ticker.Stop()

			for range ticker.C {
				runModelExecutionRecordRetentionOnce()
			}
		})
	})
}

func runModelExecutionRecordRetentionOnce() {
	if !modelExecutionRecordRetentionRunning.CompareAndSwap(false, true) {
		return
	}
	defer modelExecutionRecordRetentionRunning.Store(false)

	config := loadModelExecutionRecordRetentionConfig()
	if !config.Enabled {
		return
	}
	cutoff := common.GetTimestamp() - int64(config.RetentionHours*3600)
	if cutoff <= 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.IntervalMinutes)*time.Minute)
	defer cancel()

	var total int64
	for i := 0; i < config.MaxBatches; i++ {
		deleted, err := model.DeleteOldModelExecutionRecords(ctx, cutoff, config.BatchSize)
		if err != nil {
			common.SysLog(fmt.Sprintf("failed to cleanup old model execution records: %s", err.Error()))
			return
		}
		total += deleted
		if deleted < int64(config.BatchSize) {
			break
		}
	}
	if total > 0 {
		common.SysLog(fmt.Sprintf("cleanup old model execution records: deleted=%d retention_hours=%d", total, config.RetentionHours))
	}
}

func loadModelExecutionRecordRetentionConfig() modelExecutionRecordRetentionConfig {
	config := modelExecutionRecordRetentionConfig{
		Enabled:         common.GetEnvOrDefaultBool("MODEL_EXECUTION_RECORD_RETENTION_ENABLED", false),
		RetentionHours:  common.GetEnvOrDefault("MODEL_EXECUTION_RECORD_RETENTION_HOURS", modelExecutionRecordRetentionDefaultHours),
		IntervalMinutes: common.GetEnvOrDefault("MODEL_EXECUTION_RECORD_CLEANUP_INTERVAL_MINUTES", modelExecutionRecordRetentionDefaultIntervalMinutes),
		BatchSize:       common.GetEnvOrDefault("MODEL_EXECUTION_RECORD_CLEANUP_BATCH_SIZE", modelExecutionRecordRetentionDefaultBatchSize),
		MaxBatches:      common.GetEnvOrDefault("MODEL_EXECUTION_RECORD_CLEANUP_MAX_BATCHES", modelExecutionRecordRetentionDefaultMaxBatches),
	}
	if config.RetentionHours <= 0 {
		config.Enabled = false
		config.RetentionHours = modelExecutionRecordRetentionDefaultHours
	}
	if config.IntervalMinutes <= 0 {
		config.IntervalMinutes = modelExecutionRecordRetentionDefaultIntervalMinutes
	}
	if config.BatchSize <= 0 {
		config.BatchSize = modelExecutionRecordRetentionDefaultBatchSize
	}
	if config.BatchSize > modelExecutionRecordRetentionMaxBatchSize {
		config.BatchSize = modelExecutionRecordRetentionMaxBatchSize
	}
	if config.MaxBatches <= 0 {
		config.MaxBatches = modelExecutionRecordRetentionDefaultMaxBatches
	}
	if config.MaxBatches > modelExecutionRecordRetentionMaxBatches {
		config.MaxBatches = modelExecutionRecordRetentionMaxBatches
	}
	return config
}
