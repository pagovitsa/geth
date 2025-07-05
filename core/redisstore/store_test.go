package redisstore

import (
	"encoding/json"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/trie"
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

func TestStoreTransactionData(t *testing.T) {
	// Create Redis store
	cfg := DefaultConfig()
	store, err := NewRedisStore(cfg)
	if err != nil {
		t.Fatalf("Failed to create Redis store: %v", err)
	}
	defer store.Close()

	// Create a private key for signing transactions
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("Failed to generate private key: %v", err)
	}

	// Create test transactions
	legacySigner := types.NewEIP155Signer(big.NewInt(1))

	// Legacy transaction
	legacyTx := types.NewTransaction(
		0, // nonce
		common.HexToAddress("0x1234567890abcdef"), // to
		big.NewInt(1000),        // value
		21000,                   // gas limit
		big.NewInt(20000000000), // gas price
		[]byte("test data"),     // data
	)
	signedLegacyTx, err := types.SignTx(legacyTx, legacySigner, privateKey)
	if err != nil {
		t.Fatalf("Failed to sign legacy transaction: %v", err)
	}

	// EIP-1559 transaction
	eip1559Signer := types.NewLondonSigner(big.NewInt(1))
	eip1559Tx := types.NewTx(&types.DynamicFeeTx{
		ChainID:   big.NewInt(1),
		Nonce:     1,
		To:        &common.Address{0x12, 0x34, 0x56, 0x78, 0x90, 0xab, 0xcd, 0xef},
		Value:     big.NewInt(2000),
		Gas:       21000,
		GasFeeCap: big.NewInt(30000000000),
		GasTipCap: big.NewInt(2000000000),
		Data:      []byte("eip1559 data"),
	})
	signedEip1559Tx, err := types.SignTx(eip1559Tx, eip1559Signer, privateKey)
	if err != nil {
		t.Fatalf("Failed to sign EIP-1559 transaction: %v", err)
	}

	// Create a test block with transactions
	header := &types.Header{
		Number:     big.NewInt(1),
		Time:       uint64(time.Now().Unix()),
		Difficulty: big.NewInt(1),
		GasLimit:   1000000,
		BaseFee:    big.NewInt(1000000000), // 1 gwei
	}

	txs := []*types.Transaction{signedLegacyTx, signedEip1559Tx}
	body := &types.Body{Transactions: txs}
	block := types.NewBlock(header, body, nil, trie.NewStackTrie(nil))

	// Store block
	if err := store.StoreBlock(block, nil); err != nil {
		t.Fatalf("Failed to store block: %v", err)
	}

	// Retrieve and verify transaction data
	fields, err := store.GetBlockFields(block.Hash(), "txs")
	if err != nil {
		t.Fatalf("Failed to get block fields: %v", err)
	}

	txsDataStr, ok := fields["txs"]
	if !ok {
		t.Fatalf("Transaction data field not found")
	}

	// Parse transaction data
	var txsData []map[string]interface{}
	if err := json.Unmarshal([]byte(txsDataStr), &txsData); err != nil {
		t.Fatalf("Failed to parse transaction data: %v", err)
	}

	// Verify we have the correct number of transactions
	if len(txsData) != 2 {
		t.Fatalf("Expected 2 transactions, got %d", len(txsData))
	}

	// Verify legacy transaction data
	legacyTxData := txsData[0]
	if legacyTxData["hash"] != strings.ToLower(signedLegacyTx.Hash().Hex()) {
		t.Errorf("Legacy transaction hash mismatch: got %s, want %s",
			legacyTxData["hash"], strings.ToLower(signedLegacyTx.Hash().Hex()))
	}
	if legacyTxData["type"] != float64(0) {
		t.Errorf("Legacy transaction type mismatch: got %v, want 0", legacyTxData["type"])
	}
	if legacyTxData["to"] != "0x0000000000000000000000001234567890abcdef" {
		t.Errorf("Legacy transaction to address mismatch: got %s", legacyTxData["to"])
	}

	// Verify EIP-1559 transaction data
	eip1559TxData := txsData[1]
	if eip1559TxData["hash"] != strings.ToLower(signedEip1559Tx.Hash().Hex()) {
		t.Errorf("EIP-1559 transaction hash mismatch: got %s, want %s",
			eip1559TxData["hash"], strings.ToLower(signedEip1559Tx.Hash().Hex()))
	}
	if eip1559TxData["type"] != float64(2) {
		t.Errorf("EIP-1559 transaction type mismatch: got %v, want 2", eip1559TxData["type"])
	}
	if eip1559TxData["maxFeePerGas"] == float64(0) {
		t.Errorf("EIP-1559 transaction maxFeePerGas should not be 0")
	}
	if eip1559TxData["maxPriorityFeePerGas"] == float64(0) {
		t.Errorf("EIP-1559 transaction maxPriorityFeePerGas should not be 0")
	}

	t.Logf("Successfully stored and verified transaction data for %d transactions", len(txsData))
}
