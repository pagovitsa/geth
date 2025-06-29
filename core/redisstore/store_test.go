package redisstore

import (
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestDoubleStoreBlock(t *testing.T) {
	// Create Redis store
	cfg := DefaultConfig()
	store, err := NewRedisStore(cfg)
	if err != nil {
		t.Fatalf("Failed to create Redis store: %v", err)
	}
	defer store.Close()

	// Create a test block
	header := &types.Header{
		Number:     big.NewInt(1),
		Time:       uint64(time.Now().Unix()),
		Difficulty: big.NewInt(1),
		GasLimit:   1000000,
	}
	block := types.NewBlockWithHeader(header)

	// Create test logs
	logs := []*types.Log{
		{
			Address: common.HexToAddress("0x1234567890"),
			Topics:  []common.Hash{common.HexToHash("0xabcdef")},
			Data:    []byte("test log"),
		},
	}

	// Store block first time
	if err := store.StoreBlock(block, logs); err != nil {
		t.Fatalf("Failed to store block first time: %v", err)
	}

	// Verify block is stored
	fields1, err := store.GetBlockFields(block.Hash())
	if err != nil {
		t.Fatalf("Failed to get block fields after first store: %v", err)
	}

	// Store block second time
	if err := store.StoreBlock(block, logs); err != nil {
		t.Fatalf("Failed to store block second time: %v", err)
	}

	// Verify block is still stored with same data
	fields2, err := store.GetBlockFields(block.Hash())
	if err != nil {
		t.Fatalf("Failed to get block fields after second store: %v", err)
	}

	// Compare fields
	if len(fields1) != len(fields2) {
		t.Errorf("Field count mismatch after double store: got %d, want %d", len(fields2), len(fields1))
	}
	for k, v1 := range fields1 {
		if v2, ok := fields2[k]; !ok || v1 != v2 {
			t.Errorf("Field %s mismatch after double store: got %s, want %s", k, v2, v1)
		}
	}
}
