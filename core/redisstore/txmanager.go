package redisstore

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/metrics"
	"github.com/go-redis/redis/v8"
)

var (
	redisTxStoreTimer   = metrics.NewRegisteredTimer("redis/txstore", nil)
	redisTxErrorCounter = metrics.NewRegisteredCounter("redis/txerrors", nil)
	redisTxQueueSize    = metrics.NewRegisteredGauge("redis/txqueue", nil)
)

// StoredTransaction represents a transaction stored in Redis
type StoredTransaction struct {
	Hash        common.Hash     `json:"hash"`
	From        common.Address  `json:"from"`
	To          *common.Address `json:"to"`
	Value       *big.Int        `json:"value"`
	Gas         uint64          `json:"gas"`
	GasPrice    *big.Int        `json:"gasPrice"`
	Nonce       uint64          `json:"nonce"`
	Data        []byte          `json:"data"`
	BlockHash   common.Hash     `json:"blockHash"`
	BlockNumber uint64          `json:"blockNumber"`
	TxIndex     uint            `json:"transactionIndex"`
	RawData     string          `json:"rawData"`
	Timestamp   uint64          `json:"timestamp"`
	Status      uint64          `json:"status"`
}

// TxManager handles high-performance transaction storage
type TxManager struct {
	store  *RedisBlockStore
	client *redis.Client
	ctx    context.Context

	// Worker pool
	workers  int
	txQueue  chan *types.Transaction
	wg       sync.WaitGroup
	shutdown chan struct{}

	// Duplicate cache (simple map for now, could use Ristretto)
	dupCache map[common.Hash]bool
	dupMutex sync.RWMutex

	// Current blockchain number cache
	currentBlockNumber uint64
	blockNumberMutex   sync.RWMutex

	// Metrics
	processed uint64
	errors    uint64
}

// NewTxManager creates a new transaction manager
func NewTxManager(store *RedisBlockStore) *TxManager {
	// Set compression config for the transaction manager
	SetConfig(store.config)

	txManager := &TxManager{
		store:              store,
		client:             store.client,
		ctx:                store.ctx,
		workers:            10,                                  // Configurable worker pool size
		txQueue:            make(chan *types.Transaction, 1000), // Buffered channel
		shutdown:           make(chan struct{}),
		dupCache:           make(map[common.Hash]bool),
		currentBlockNumber: 0, // Initialize to 0, will be updated when blocks are processed
	}

	// Set the transaction manager reference in the store for block number updates
	store.SetTxManager(txManager)

	return txManager
}

// loadExistingTxHashes loads existing transaction hashes from Redis to prevent duplicates
func (tm *TxManager) loadExistingTxHashes() error {
	// Use SCAN to iterate through all tx:* keys
	iter := tm.client.Scan(tm.ctx, 0, "tx:*", 1000).Iterator()
	loaded := 0

	for iter.Next(tm.ctx) {
		key := iter.Val()
		// Extract hash from key (format: "tx:hash")
		if len(key) > 3 {
			hashStr := key[3:]      // Remove "tx:" prefix
			if len(hashStr) == 64 { // Valid hex hash length
				hash := common.HexToHash("0x" + hashStr)
				tm.dupMutex.Lock()
				tm.dupCache[hash] = true
				tm.dupMutex.Unlock()
				loaded++
			}
		}
	}

	if err := iter.Err(); err != nil {
		return fmt.Errorf("failed to scan transaction keys: %v", err)
	}

	return nil
}

// Init initializes the transaction manager
func (tm *TxManager) Init() error {
	// Test Redis connection
	if err := tm.client.Ping(tm.ctx).Err(); err != nil {
		log.Error("Redis connection test failed", "err", err)
		return fmt.Errorf("Redis connection failed: %v", err)
	}

	// Load existing transaction hashes from Redis to prevent duplicates
	if err := tm.loadExistingTxHashes(); err != nil {
		log.Warn("Failed to load existing transaction hashes", "err", err)
	}

	// Start worker goroutines
	for i := 0; i < tm.workers; i++ {
		tm.wg.Add(1)
		go tm.worker(i)
	}

	return nil
}

// StoreTx stores a transaction (async if queue has space, sync if full)
func (tm *TxManager) StoreTx(tx *types.Transaction) error {
	// Check for duplicates
	tm.dupMutex.RLock()
	if tm.dupCache[tx.Hash()] {
		tm.dupMutex.RUnlock()
		return nil // Already processed
	}
	tm.dupMutex.RUnlock()

	// Try async first
	select {
	case tm.txQueue <- tx:
		// Successfully queued
		redisTxQueueSize.Update(int64(len(tm.txQueue)))
		return nil
	default:
		// Queue full, process synchronously
		return tm.storeTxSync(tx)
	}
}

// worker processes transactions from the queue
func (tm *TxManager) worker(id int) {
	defer tm.wg.Done()

	for {
		select {
		case tx := <-tm.txQueue:
			if err := tm.storeTxSync(tx); err != nil {
				log.Error("Worker failed to store transaction", "worker", id, "hash", tx.Hash(), "err", err)
			}
			redisTxQueueSize.Update(int64(len(tm.txQueue)))

		case <-tm.shutdown:
			return
		}
	}
}

// storeTxSync synchronously stores a transaction
func (tm *TxManager) storeTxSync(tx *types.Transaction) error {
	defer redisTxStoreTimer.UpdateSince(time.Now())

	// Mark as processed in duplicate cache
	tm.dupMutex.Lock()
	tm.dupCache[tx.Hash()] = true
	tm.dupMutex.Unlock()

	// Create stored transaction with proper rawdata encoding
	rawTxData, err := tx.MarshalBinary()
	if err != nil {
		redisTxErrorCounter.Inc(1)
		return fmt.Errorf("failed to marshal transaction: %v", err)
	}

	storedTx := &StoredTransaction{
		Hash:      tx.Hash(),
		Gas:       tx.Gas(),
		Nonce:     tx.Nonce(),
		Data:      tx.Data(),
		RawData:   fmt.Sprintf("0x%x", rawTxData),
		Timestamp: uint64(time.Now().Unix()),
		Status:    0, // Pending
	}

	// Handle transaction fields safely
	if tx.To() != nil {
		storedTx.To = tx.To()
	}

	storedTx.Value = tx.Value()
	storedTx.GasPrice = tx.GasPrice()

	// Get sender (this might fail for some transactions)
	chainID := tx.ChainId()
	if chainID != nil && chainID.Cmp(big.NewInt(0)) > 0 {
		if from, err := types.Sender(types.LatestSignerForChainID(chainID), tx); err == nil {
			storedTx.From = from
		}
	}

	txKey := fmt.Sprintf("tx:%x", tx.Hash())

	// Get current blockchain number from cache
	tm.blockNumberMutex.RLock()
	currentBlockNum := tm.currentBlockNumber
	tm.blockNumberMutex.RUnlock()

	// Create transaction hash with only required fields for pending transactions
	txFields := map[string]interface{}{
		"hash":              strings.ToLower(storedTx.Hash.Hex()),
		"nonce":             storedTx.Nonce,
		"from":              strings.ToLower(storedTx.From.Hex()),
		"rawdata":           storedTx.RawData,
		"gasprice":          storedTx.GasPrice.String(),
		"gas":               storedTx.Gas,
		"value":             storedTx.Value.String(),
		"type":              tx.Type(),       // Add transaction type (0=Legacy, 1=AccessList, 2=DynamicFee)
		"blockchain_number": currentBlockNum, // Add current blockchain number
	}

	// Add EIP-1559 fields for Type 2 transactions
	if tx.Type() == 2 {
		if tx.GasFeeCap() != nil {
			txFields["maxfeepergas"] = tx.GasFeeCap().String()
		}
		if tx.GasTipCap() != nil {
			txFields["maxpriorityfeepergas"] = tx.GasTipCap().String()
		}
	}

	// Add 'to' field if it exists
	if storedTx.To != nil {
		txFields["to"] = strings.ToLower(storedTx.To.Hex())
	} else {
		txFields["to"] = "" // Empty string for contract creation
	}

	// Store transaction header data as hash fields
	if err := tm.client.HMSet(tm.ctx, txKey, txFields).Err(); err != nil {
		redisTxErrorCounter.Inc(1)
		return fmt.Errorf("failed to store transaction header: %v", err)
	}

	// Store full transaction data as uncompressed JSON
	txData, err := json.Marshal(storedTx)
	if err != nil {
		redisTxErrorCounter.Inc(1)
		return fmt.Errorf("failed to encode transaction: %v", err)
	}

	// Store full transaction data as separate field
	if err := tm.client.HSet(tm.ctx, txKey, "full_data", txData).Err(); err != nil {
		redisTxErrorCounter.Inc(1)
		return fmt.Errorf("failed to store transaction full data: %v", err)
	}

	// Set TTL for transaction (10 days)
	if err := tm.client.Expire(tm.ctx, txKey, 10*24*time.Hour).Err(); err != nil {
		redisTxErrorCounter.Inc(1)
		return fmt.Errorf("failed to set transaction TTL: %v", err)
	}

	tm.processed++
	return nil
}

// UpdateTxStatus updates transaction status (mined/dropped)
func (tm *TxManager) UpdateTxStatus(hash common.Hash, blockHash common.Hash, blockNumber uint64, txIndex uint, status uint64) error {
	txKey := fmt.Sprintf("tx:%x", hash)

	// Check if transaction exists
	exists, err := tm.client.Exists(tm.ctx, txKey).Result()
	if err != nil {
		return fmt.Errorf("failed to check transaction existence: %v", err)
	}
	if exists == 0 {
		return nil // Transaction not found, ignore
	}

	// Update hash fields directly
	updateFields := map[string]interface{}{
		"block_hash":   strings.ToLower(blockHash.Hex()),
		"block_number": blockNumber,
		"tx_index":     txIndex,
		"status":       status,
	}

	if err := tm.client.HMSet(tm.ctx, txKey, updateFields).Err(); err != nil {
		return fmt.Errorf("failed to update transaction fields: %v", err)
	}

	// Also update the full data for complete consistency
	txData, err := tm.client.HGet(tm.ctx, txKey, "full_data").Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil // Full data not found, but hash fields updated
		}
		return fmt.Errorf("failed to get transaction full data: %v", err)
	}

	// Decode transaction (stored as uncompressed JSON)
	var storedTx StoredTransaction
	if err := json.Unmarshal(txData, &storedTx); err != nil {
		return fmt.Errorf("failed to decode transaction: %v", err)
	}

	// Update fields
	storedTx.BlockHash = blockHash
	storedTx.BlockNumber = blockNumber
	storedTx.TxIndex = txIndex
	storedTx.Status = status

	// Re-encode and store as uncompressed JSON
	updatedData, err := json.Marshal(storedTx)
	if err != nil {
		return fmt.Errorf("failed to encode updated transaction: %v", err)
	}

	return tm.client.HSet(tm.ctx, txKey, "full_data", updatedData).Err()
}

// GetTx retrieves a transaction from Redis hash structure
func (tm *TxManager) GetTx(hash common.Hash) (*StoredTransaction, error) {
	txKey := fmt.Sprintf("tx:%x", hash)

	// Check if transaction exists
	exists, err := tm.client.Exists(tm.ctx, txKey).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to check transaction existence: %v", err)
	}
	if exists == 0 {
		return nil, nil // Transaction not found
	}

	// Get full data from hash field
	txData, err := tm.client.HGet(tm.ctx, txKey, "full_data").Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil // Transaction full data not found
		}
		return nil, fmt.Errorf("failed to get transaction full data: %v", err)
	}

	// Decode transaction (stored as uncompressed JSON)
	var storedTx StoredTransaction
	if err := json.Unmarshal(txData, &storedTx); err != nil {
		return nil, fmt.Errorf("failed to decode transaction: %v", err)
	}

	return &storedTx, nil
}

// Close shuts down the transaction manager
func (tm *TxManager) Close() error {
	close(tm.shutdown)
	tm.wg.Wait()

	log.Info("Redis transaction manager closed",
		"processed", tm.processed,
		"errors", tm.errors,
		"queue_remaining", len(tm.txQueue))

	return nil
}

// UpdateCurrentBlockNumber updates the cached current blockchain number
func (tm *TxManager) UpdateCurrentBlockNumber(blockNumber uint64) {
	tm.blockNumberMutex.Lock()
	if blockNumber > tm.currentBlockNumber {
		tm.currentBlockNumber = blockNumber
		log.Debug("Updated current blockchain number", "number", blockNumber)
	}
	tm.blockNumberMutex.Unlock()
}

// GetCurrentBlockNumber returns the cached current blockchain number
func (tm *TxManager) GetCurrentBlockNumber() uint64 {
	tm.blockNumberMutex.RLock()
	defer tm.blockNumberMutex.RUnlock()
	return tm.currentBlockNumber
}

// Stats returns transaction manager statistics
func (tm *TxManager) Stats() map[string]interface{} {
	tm.dupMutex.RLock()
	cacheSize := len(tm.dupCache)
	tm.dupMutex.RUnlock()

	tm.blockNumberMutex.RLock()
	currentBlock := tm.currentBlockNumber
	tm.blockNumberMutex.RUnlock()

	return map[string]interface{}{
		"processed":            tm.processed,
		"errors":               tm.errors,
		"queue_size":           len(tm.txQueue),
		"cache_size":           cacheSize,
		"workers":              tm.workers,
		"current_block_number": currentBlock,
	}
}

// RemoveTx removes a transaction from Redis
func (tm *TxManager) RemoveTx(hash common.Hash) error {
	txKey := fmt.Sprintf("tx:%x", hash)

	// Remove from duplicate cache
	tm.dupMutex.Lock()
	delete(tm.dupCache, hash)
	tm.dupMutex.Unlock()

	// Remove from Redis
	if err := tm.client.Del(tm.ctx, txKey).Err(); err != nil {
		log.Error("Failed to remove transaction from Redis", "hash", hash.Hex(), "key", txKey, "err", err)
		return fmt.Errorf("failed to remove transaction from Redis: %v", err)
	}

	return nil
}

// RemoveTxs removes multiple transactions from Redis (batch operation)
func (tm *TxManager) RemoveTxs(hashes []common.Hash) error {
	if len(hashes) == 0 {
		return nil
	}

	// Remove from duplicate cache
	tm.dupMutex.Lock()
	for _, hash := range hashes {
		delete(tm.dupCache, hash)
	}
	tm.dupMutex.Unlock()

	// Prepare keys for batch deletion
	keys := make([]string, len(hashes))
	for i, hash := range hashes {
		keys[i] = fmt.Sprintf("tx:%x", hash)
	}

	// Batch remove from Redis
	_, err := tm.client.Del(tm.ctx, keys...).Result()
	if err != nil {
		log.Error("Failed to batch remove transactions from Redis", "count", len(hashes), "err", err)
		return fmt.Errorf("failed to batch remove transactions from Redis: %v", err)
	}

	return nil
}

// ListRedisTransactions returns all transaction hashes currently in Redis (for debugging)
func (tm *TxManager) ListRedisTransactions() ([]string, error) {
	var keys []string
	iter := tm.client.Scan(tm.ctx, 0, "tx:*", 1000).Iterator()

	for iter.Next(tm.ctx) {
		keys = append(keys, iter.Val())
	}

	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("failed to scan Redis keys: %v", err)
	}

	return keys, nil
}
