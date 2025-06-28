package redisstore

import (
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestCompression(t *testing.T) {
	// Test compression and decompression
	testData := []byte("Hello, Redis! This is a test of zlib compression in geth.")

	// Test with compression enabled
	cfg := DefaultConfig()
	cfg.CompressEnabled = true
	SetConfig(cfg)

	compressed, err := Compress(testData)
	if err != nil {
		t.Fatalf("Compression failed: %v", err)
	}

	if len(compressed) >= len(testData) {
		t.Logf("Warning: Compressed data (%d bytes) is not smaller than original (%d bytes)", len(compressed), len(testData))
	}

	decompressed, err := Decompress(compressed)
	if err != nil {
		t.Fatalf("Decompression failed: %v", err)
	}

	if string(decompressed) != string(testData) {
		t.Fatalf("Decompressed data doesn't match original. Got: %s, Expected: %s", string(decompressed), string(testData))
	}

	ratio := CompressRatio(testData, compressed)
	t.Logf("Compression enabled - ratio: %.2f", ratio)

	// Test with compression disabled
	cfg.CompressEnabled = false
	SetConfig(cfg)

	uncompressed, err := Compress(testData)
	if err != nil {
		t.Fatalf("Compression (disabled) failed: %v", err)
	}

	if string(testData) != string(uncompressed) {
		t.Fatalf("Data should be unchanged when compression is disabled")
	}

	unchanged, err := Decompress(uncompressed)
	if err != nil {
		t.Fatalf("Decompression (disabled) failed: %v", err)
	}

	if string(testData) != string(unchanged) {
		t.Fatalf("Data should be unchanged when compression is disabled")
	}

	t.Log("Compression disabled test passed")
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if !config.IsEnabled() {
		t.Fatal("Default config should be enabled")
	}

	if config.Network != "unix" {
		t.Fatalf("Expected network 'unix', got '%s'", config.Network)
	}

	if config.Address != "/media/redis/local.sock" {
		t.Fatalf("Expected address '/media/redis/local.sock', got '%s'", config.Address)
	}

	if config.Username != "root" || config.Password != "root" {
		t.Fatalf("Expected username/password 'root/root', got '%s/%s'", config.Username, config.Password)
	}

	if config.CompressEnabled {
		t.Fatal("Default config should have compression disabled")
	}
}

func createTestBlock() *types.Block {
	// Create a simple test block without transactions to avoid DeriveSha issues
	header := &types.Header{
		Number:     big.NewInt(1),
		Time:       uint64(time.Now().Unix()),
		Difficulty: big.NewInt(1000),
		GasLimit:   8000000,
		GasUsed:    0,
		Coinbase:   common.HexToAddress("0x1234567890123456789012345678901234567890"),
		Root:       common.HexToHash("0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"),
		TxHash:     types.EmptyTxsHash,
		ParentHash: common.Hash{},
	}

	// Create block with empty body
	block := types.NewBlock(header, &types.Body{}, nil, nil)

	return block
}

func createTestLogs() []*types.Log {
	return []*types.Log{
		{
			Address: common.HexToAddress("0x1234567890123456789012345678901234567890"),
			Topics: []common.Hash{
				common.HexToHash("0xabcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"),
			},
			Data:        []byte("test log data"),
			BlockNumber: 1,
			TxHash:      common.HexToHash("0x1111111111111111111111111111111111111111111111111111111111111111"),
			TxIndex:     0,
			BlockHash:   common.HexToHash("0x2222222222222222222222222222222222222222222222222222222222222222"),
			Index:       0,
		},
	}
}

// Note: These tests require a running Redis instance
// They will be skipped if Redis is not available

func TestRedisStoreConnection(t *testing.T) {
	config := DefaultConfig()

	// Try to create Redis store
	store, err := NewRedisStore(config)
	if err != nil {
		t.Skipf("Redis not available, skipping test: %v", err)
		return
	}
	defer store.Close()

	t.Log("Successfully connected to Redis")
}

func TestBlockStorage(t *testing.T) {
	config := DefaultConfig()

	store, err := NewRedisStore(config)
	if err != nil {
		t.Skipf("Redis not available, skipping test: %v", err)
		return
	}
	defer store.Close()

	// Create test block and logs
	block := createTestBlock()
	logs := createTestLogs()

	// Store block
	err = store.StoreBlock(block, logs)
	if err != nil {
		t.Fatalf("Failed to store block: %v", err)
	}

	// Retrieve block fields
	fields, err := store.GetBlockFields(block.Hash())
	if err != nil {
		t.Fatalf("Failed to retrieve block fields: %v", err)
	}

	if fields == nil {
		t.Fatal("Retrieved block fields is nil")
	}

	// Verify essential fields
	expectedNumber := block.NumberU64()
	if fields["blocknumber"] != "1" {
		t.Fatalf("Block number mismatch. Expected: %d, Got: %s", expectedNumber, fields["blocknumber"])
	}

	if fields["blockgasprice"] == "" {
		t.Fatal("Block gas price should not be empty")
	}

	if fields["txshashes"] == "" {
		t.Fatal("Transaction hashes should not be empty (should be empty array)")
	}

	if fields["txslogs"] == "" {
		t.Fatal("Transaction logs should not be empty (should be empty array or logs)")
	}

	// Retrieve logs
	retrievedLogs, err := store.GetLogs(block.Hash())
	if err != nil {
		t.Fatalf("Failed to retrieve logs: %v", err)
	}

	if len(retrievedLogs) != len(logs) {
		t.Fatalf("Log count mismatch. Expected: %d, Got: %d", len(logs), len(retrievedLogs))
	}

	t.Logf("Successfully stored and retrieved block %s with %d logs", block.Hash().Hex(), len(logs))
}

func TestTxManager(t *testing.T) {
	config := DefaultConfig()

	store, err := NewRedisStore(config)
	if err != nil {
		t.Skipf("Redis not available, skipping test: %v", err)
		return
	}
	defer store.Close()

	// Create transaction manager
	txMgr := NewTxManager(store)
	err = txMgr.Init()
	if err != nil {
		t.Fatalf("Failed to initialize transaction manager: %v", err)
	}
	defer txMgr.Close()

	// Create test transaction
	key, _ := crypto.GenerateKey()
	tx := types.NewTransaction(
		0,                      // nonce
		common.Address{0x1},    // to
		big.NewInt(1000),       // value
		21000,                  // gas limit
		big.NewInt(1000000000), // gas price
		nil,                    // data
	)
	signer := types.NewEIP155Signer(big.NewInt(1))
	signedTx, _ := types.SignTx(tx, signer, key)

	// Store transaction
	err = txMgr.StoreTx(signedTx)
	if err != nil {
		t.Fatalf("Failed to store transaction: %v", err)
	}

	// Wait a bit for async processing
	time.Sleep(100 * time.Millisecond)

	// Retrieve transaction
	storedTx, err := txMgr.GetTx(signedTx.Hash())
	if err != nil {
		t.Fatalf("Failed to retrieve transaction: %v", err)
	}

	if storedTx == nil {
		t.Fatal("Retrieved transaction is nil")
	}

	if storedTx.Hash != signedTx.Hash() {
		t.Fatalf("Transaction hash mismatch. Expected: %s, Got: %s", signedTx.Hash().Hex(), storedTx.Hash.Hex())
	}

	// Test transaction status update
	blockHash := common.HexToHash("0x3333333333333333333333333333333333333333333333333333333333333333")
	err = txMgr.UpdateTxStatus(signedTx.Hash(), blockHash, 123, 0, 1)
	if err != nil {
		t.Fatalf("Failed to update transaction status: %v", err)
	}

	// Retrieve updated transaction
	updatedTx, err := txMgr.GetTx(signedTx.Hash())
	if err != nil {
		t.Fatalf("Failed to retrieve updated transaction: %v", err)
	}

	if updatedTx.BlockHash != blockHash {
		t.Fatalf("Block hash not updated. Expected: %s, Got: %s", blockHash.Hex(), updatedTx.BlockHash.Hex())
	}

	if updatedTx.Status != 1 {
		t.Fatalf("Status not updated. Expected: 1, Got: %d", updatedTx.Status)
	}

	// Test stats
	stats := txMgr.Stats()
	t.Logf("Transaction manager stats: %+v", stats)

	t.Log("Transaction manager test completed successfully")
}

func TestErrorHandling(t *testing.T) {
	// Test with invalid configuration
	config := &Config{
		Enabled:  true,
		Network:  "unix",
		Address:  "/invalid/redis/socket",
		Username: "invalid",
		Password: "invalid",
		DB:       0,
	}

	_, err := NewRedisStore(config)
	if err == nil {
		t.Fatal("Expected error with invalid Redis configuration")
	}

	t.Logf("Correctly handled invalid configuration: %v", err)
}

func BenchmarkCompression(b *testing.B) {
	testData := make([]byte, 1024) // 1KB test data
	for i := range testData {
		testData[i] = byte(i % 256)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		compressed, err := Compress(testData)
		if err != nil {
			b.Fatal(err)
		}

		_, err = Decompress(compressed)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBlockStorage(b *testing.B) {
	config := DefaultConfig()

	store, err := NewRedisStore(config)
	if err != nil {
		b.Skipf("Redis not available, skipping benchmark: %v", err)
		return
	}
	defer store.Close()

	block := createTestBlock()
	logs := createTestLogs()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := store.StoreBlock(block, logs)
		if err != nil {
			b.Fatal(err)
		}
	}
}
