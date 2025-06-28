# Manual Redis Integration Guide

This guide provides step-by-step instructions for manually integrating our Redis storage system into a new go-ethereum version when the automatic patch fails.

## Prerequisites

1. Fresh go-ethereum repository
2. Redis server installed
3. Our Redis integration files

## Step 1: Copy Redis Store Module

Copy the entire `core/redisstore/` directory from our implementation:

```bash
cp -r old-geth/core/redisstore/ new-geth/core/
```

This includes:
- `config.go` - Redis configuration
- `store.go` - Block and log storage
- `txmanager.go` - Transaction management
- `store_test.go` - Tests
- `compression.go` - Data compression

## Step 2: Update Dependencies

Add to `go.mod`:

```go
require (
    // ... existing dependencies
    github.com/go-redis/redis/v8 v8.11.5
)
```

Run:
```bash
go mod tidy
```

## Step 3: Blockchain Integration

### File: `core/blockchain.go`

Add imports:
```go
import (
    // ... existing imports
    "github.com/ethereum/go-ethereum/core/redisstore"
)
```

Add to `BlockChain` struct:
```go
type BlockChain struct {
    // ... existing fields
    redisStore *redisstore.RedisBlockStore
    txManager  *redisstore.TxManager
}
```

In `NewBlockChain` function, add after database initialization:
```go
// Initialize Redis store
redisConfig := &redisstore.Config{
    Enabled:     true,
    Address:     "localhost:6379",
    Password:    "",
    DB:          0,
    PoolSize:    10,
    MinIdle:     5,
    MaxRetries:  3,
    RetryDelay:  time.Millisecond * 100,
    Compression: true,
}

var redisStore *redisstore.RedisBlockStore
var txManager *redisstore.TxManager

if redisConfig.IsEnabled() {
    var err error
    redisStore, err = redisstore.NewRedisStore(redisConfig)
    if err != nil {
        log.Warn("Failed to initialize Redis store", "err", err)
    } else {
        txManager = redisstore.NewTxManager(redisStore)
        if err := txManager.Init(); err != nil {
            log.Warn("Failed to initialize transaction manager", "err", err)
            txManager = nil
        } else {
            log.Info("Redis storage initialized successfully")
        }
    }
}
```

Add to blockchain struct initialization:
```go
bc := &BlockChain{
    // ... existing fields
    redisStore: redisStore,
    txManager:  txManager,
}
```

In block processing function (usually `writeBlockWithState` or similar), add:
```go
// Store block and logs in Redis
if bc.redisStore != nil {
    if err := bc.redisStore.StoreBlock(block, logs); err != nil {
        log.Warn("Failed to store block in Redis", "number", block.Number(), "hash", block.Hash(), "err", err)
    }
}
```

Add cleanup in `Stop` method:
```go
func (bc *BlockChain) Stop() {
    // ... existing cleanup
    
    if bc.txManager != nil {
        bc.txManager.Close()
    }
    if bc.redisStore != nil {
        bc.redisStore.Close()
    }
}
```

## Step 4: Transaction Pool Integration

### File: `core/txpool/legacypool/legacypool.go`

Add import:
```go
import (
    // ... existing imports
    "github.com/ethereum/go-ethereum/core/redisstore"
)
```

Add to pool struct:
```go
type LegacyPool struct {
    // ... existing fields
    txManager *redisstore.TxManager
}
```

In constructor, add:
```go
func New(config Config, chain BlockChain) *LegacyPool {
    // ... existing code
    
    // Get transaction manager from blockchain
    var txManager *redisstore.TxManager
    if bc, ok := chain.(*core.BlockChain); ok {
        txManager = bc.GetTxManager() // You'll need to add this getter method
    }
    
    pool := &LegacyPool{
        // ... existing fields
        txManager: txManager,
    }
    
    // ... rest of function
}
```

In transaction addition function (usually `add` or `addTx`):
```go
func (pool *LegacyPool) add(tx *types.Transaction, local bool) (replaced bool, err error) {
    // ... existing validation code
    
    // Store transaction in Redis
    if pool.txManager != nil {
        if err := pool.txManager.StoreTx(tx); err != nil {
            log.Debug("Failed to store transaction in Redis", "hash", tx.Hash(), "err", err)
            // Don't fail the transaction addition if Redis storage fails
        }
    }
    
    // ... rest of function
}
```

## Step 5: Add Getter Method to BlockChain

Add to `core/blockchain.go`:

```go
// GetTxManager returns the transaction manager for external use
func (bc *BlockChain) GetTxManager() *redisstore.TxManager {
    return bc.txManager
}

// GetRedisStore returns the Redis store for external use
func (bc *BlockChain) GetRedisStore() *redisstore.RedisBlockStore {
    return bc.redisStore
}
```

## Step 6: Command Line Configuration (Optional)

### File: `cmd/geth/config.go`

Add Redis configuration to the config struct:
```go
type gethConfig struct {
    // ... existing fields
    Redis redisstore.Config `toml:",omitempty"`
}
```

### File: `cmd/geth/main.go`

Add Redis flags:
```go
var (
    // ... existing flags
    redisEnabledFlag = &cli.BoolFlag{
        Name:  "redis.enabled",
        Usage: "Enable Redis storage for transactions and blocks",
    }
    redisAddrFlag = &cli.StringFlag{
        Name:  "redis.addr",
        Usage: "Redis server address",
        Value: "localhost:6379",
    }
    redisPasswordFlag = &cli.StringFlag{
        Name:  "redis.password",
        Usage: "Redis server password",
    }
    redisDBFlag = &cli.IntFlag{
        Name:  "redis.db",
        Usage: "Redis database number",
        Value: 0,
    }
)
```

Add flags to the app:
```go
app.Flags = append(app.Flags, []cli.Flag{
    // ... existing flags
    redisEnabledFlag,
    redisAddrFlag,
    redisPasswordFlag,
    redisDBFlag,
}...)
```

## Step 7: Testing

1. **Test Redis store module**:
```bash
go test -v ./core/redisstore/...
```

2. **Build geth**:
```bash
make geth
```

3. **Test with Redis**:
```bash
# Start Redis
redis-server

# Test geth
./build/bin/geth --datadir ./testdata --redis.enabled
```

## Step 8: Verification

Check that the integration works:

1. **Redis connection**: Look for "Redis storage initialized successfully" in logs
2. **Transaction storage**: Check Redis for `tx:*` keys
3. **Block storage**: Check Redis for `block:*` keys
4. **Performance**: Monitor Redis memory usage and geth performance

## Troubleshooting

### Common Issues

1. **Import path conflicts**: Update import paths if go-ethereum restructures packages
2. **Function signature changes**: Adapt to new function signatures in go-ethereum
3. **Struct field changes**: Update field access if structs change

### Debug Steps

1. **Enable debug logging**:
```bash
./build/bin/geth --verbosity 4 --redis.enabled
```

2. **Check Redis connectivity**:
```bash
redis-cli ping
```

3. **Monitor Redis**:
```bash
redis-cli monitor
```

### Rollback

If issues occur:
1. Remove Redis-related code
2. Set `redis.enabled=false`
3. Rebuild without Redis integration

## Maintenance

1. **Regular testing** after go-ethereum updates
2. **Monitor performance** impact
3. **Update Redis configuration** as needed
4. **Keep Redis store module** up to date with go-ethereum changes

## Support

For integration issues:
1. Check go-ethereum changelog for breaking changes
2. Test Redis store module independently
3. Verify configuration settings
4. Check Redis server status and logs
