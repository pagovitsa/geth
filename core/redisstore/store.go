package redisstore

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/metrics"
	"github.com/go-redis/redis/v8"
)

var (
	redisBlockStoreTimer = metrics.NewRegisteredTimer("redis/blockstore", nil)
	redisErrorCounter    = metrics.NewRegisteredCounter("redis/errors", nil)
)

// RedisBlockStore handles storage of blocks and logs in Redis
type RedisBlockStore struct {
	client    *redis.Client
	config    *Config
	ctx       context.Context
	txManager *TxManager
}

// NewRedisStore creates a new Redis block store
func NewRedisStore(cfg *Config) (*RedisBlockStore, error) {
	if !cfg.IsEnabled() {
		return nil, fmt.Errorf("redis storage is disabled")
	}

	// Set compression config
	SetConfig(cfg)

	client := redis.NewClient(&redis.Options{
		Network:         cfg.Network,
		Addr:            cfg.Address,
		Username:        cfg.Username,
		Password:        cfg.Password,
		DB:              cfg.DB,
		PoolSize:        cfg.PoolSize,
		MinIdleConns:    cfg.MinIdle,
		MaxRetries:      cfg.MaxRetries,
		MinRetryBackoff: cfg.RetryDelay,
		MaxRetryBackoff: cfg.RetryDelay * 2,
	})

	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis connection failed: %v", err)
	}

	store := &RedisBlockStore{
		client: client,
		config: cfg,
		ctx:    ctx,
	}

	return store, nil
}

// SetTxManager sets the transaction manager reference for block number updates
func (s *RedisBlockStore) SetTxManager(txManager *TxManager) {
	s.txManager = txManager
}

// StoreBlock stores a block and its logs in Redis using hash structure
func (s *RedisBlockStore) StoreBlock(block *types.Block, logs []*types.Log) error {
	defer redisBlockStoreTimer.UpdateSince(time.Now())

	blockKey := fmt.Sprintf("block:%x", block.Hash())

	// Use atomic SET operation with NX (Not eXists) to prevent race conditions
	// This creates a lock key that prevents duplicate processing of the same block
	lockKey := fmt.Sprintf("lock:%x", block.Hash())
	set, err := s.client.SetNX(s.ctx, lockKey, "1", 5*time.Second).Result()
	if err != nil {
		redisErrorCounter.Inc(1)
		return fmt.Errorf("failed to acquire block lock: %v", err)
	}
	if !set {
		// Another process is already storing this block, skip to prevent duplicates
		return nil
	}

	// Ensure lock is cleaned up even if function exits early
	defer s.client.Del(s.ctx, lockKey)

	// Extract transaction hashes from block
	txHashes := make([]string, len(block.Transactions()))
	for i, tx := range block.Transactions() {
		txHashes[i] = strings.ToLower(tx.Hash().Hex())
	}
	txHashesJSON, _ := json.Marshal(txHashes)

	// Get block gas price (base fee or 0 if not available)
	var blockGasPrice string
	if block.BaseFee() != nil {
		blockGasPrice = block.BaseFee().String()
	} else {
		blockGasPrice = "0"
	}

	// Convert logs to JSON format
	logsData, err := json.Marshal(logs)
	if err != nil {
		redisErrorCounter.Inc(1)
		return fmt.Errorf("failed to encode logs: %v", err)
	}

	// Create block hash with all fields including logs (single HSET operation)
	blockFields := map[string]interface{}{
		"blocknumber":   block.NumberU64(),
		"blockgasprice": blockGasPrice,
		"txshashes":     string(txHashesJSON),
		"txslogs":       logsData,
	}

	// Store all block data in a single atomic operation
	if err := s.client.HMSet(s.ctx, blockKey, blockFields).Err(); err != nil {
		redisErrorCounter.Inc(1)
		return fmt.Errorf("failed to store block data: %v", err)
	}

	// Set TTL for block (60 seconds)
	if err := s.client.Expire(s.ctx, blockKey, 60*time.Second).Err(); err != nil {
		redisErrorCounter.Inc(1)
		return fmt.Errorf("failed to set block TTL: %v", err)
	}

	// Update current blockchain number in transaction manager if available
	if s.txManager != nil {
		s.txManager.UpdateCurrentBlockNumber(block.NumberU64())
	}

	return nil
}

// GetBlock retrieves a block from Redis hash structure
func (s *RedisBlockStore) GetBlock(hash common.Hash) (*types.Block, error) {
	blockKey := fmt.Sprintf("block:%x", hash)

	// Check if block exists
	exists, err := s.client.Exists(s.ctx, blockKey).Result()
	if err != nil {
		redisErrorCounter.Inc(1)
		return nil, fmt.Errorf("failed to check block existence: %v", err)
	}
	if exists == 0 {
		return nil, nil // Block not found
	}

	// Since we no longer store RLP data, we cannot reconstruct the full block
	// This method now returns nil to indicate blocks should be retrieved from other sources
	return nil, fmt.Errorf("block reconstruction not available - RLP data not stored")
}

// GetLogs retrieves logs for a block from Redis hash structure
func (s *RedisBlockStore) GetLogs(hash common.Hash) ([]*types.Log, error) {
	blockKey := fmt.Sprintf("block:%x", hash)

	// Get logs data from hash field
	logsData, err := s.client.HGet(s.ctx, blockKey, "txslogs").Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil // Logs not found
		}
		redisErrorCounter.Inc(1)
		return nil, fmt.Errorf("failed to get logs: %v", err)
	}

	// Decode logs (stored as uncompressed JSON)
	var logs []*types.Log
	if err := json.Unmarshal(logsData, &logs); err != nil {
		redisErrorCounter.Inc(1)
		return nil, fmt.Errorf("failed to decode logs: %v", err)
	}

	return logs, nil
}

// GetBlockFields retrieves specific block fields from Redis hash
func (s *RedisBlockStore) GetBlockFields(hash common.Hash, fields ...string) (map[string]string, error) {
	blockKey := fmt.Sprintf("block:%x", hash)

	if len(fields) == 0 {
		// Get all fields
		result, err := s.client.HGetAll(s.ctx, blockKey).Result()
		if err != nil {
			redisErrorCounter.Inc(1)
			return nil, fmt.Errorf("failed to get block fields: %v", err)
		}
		return result, nil
	}

	// Get specific fields
	result, err := s.client.HMGet(s.ctx, blockKey, fields...).Result()
	if err != nil {
		redisErrorCounter.Inc(1)
		return nil, fmt.Errorf("failed to get block fields: %v", err)
	}

	fieldMap := make(map[string]string)
	for i, field := range fields {
		if result[i] != nil {
			fieldMap[field] = result[i].(string)
		}
	}

	return fieldMap, nil
}

// Close closes the Redis connection
func (s *RedisBlockStore) Close() error {
	return s.client.Close()
}
