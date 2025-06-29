# Redis Transaction Integration

This document describes how the Redis transaction integration works in the Ethereum blockchain.

## Overview

The blockchain now automatically manages Redis transaction storage by:

1. **Removing transactions from Redis** when they are included in a new block
2. **Re-adding transactions to Redis** during chain reorganizations when transactions become orphaned

## How it Works

### New Block Processing

When a new block is processed and becomes the canonical head:

1. The blockchain extracts all transaction hashes from the block
2. These transactions are removed from the Redis mempool using `RemoveTxs()`
3. The current block number is updated in the Redis transaction manager
4. Operations are performed asynchronously to avoid blocking blockchain operations

### Chain Reorganizations (Reorgs)

During a chain reorganization:

1. **Orphaned transactions**: Transactions from the old canonical chain that are not in the new chain are re-added to the Redis mempool
2. **Newly mined transactions**: Transactions in the new canonical chain are automatically removed by the `writeHeadBlock` calls

## Key Components

### TxManager Methods

- `RemoveTx(hash)`: Remove a single transaction from Redis
- `RemoveTxs(hashes)`: Remove multiple transactions from Redis (batch operation)
- `StoreTx(tx)`: Add a transaction to Redis mempool
- `UpdateCurrentBlockNumber(num)`: Update the cached current block number

### Metrics

The following metrics are available to monitor Redis transaction operations:

- `chain/redis/txremoval`: Number of transactions removed from Redis
- `chain/redis/txremoval/errors`: Number of errors during transaction removal
- `chain/redis/reorg/add`: Number of transactions re-added during reorgs
- `chain/redis/reorg/errors`: Number of errors during reorg transaction handling

## Integration Points

### BlockChain.writeHeadBlock()

This method is called whenever a new block becomes the canonical head. It:

- Stores the block and logs in Redis
- Updates the transaction manager's current block number
- Removes all mined transactions from the Redis mempool

### BlockChain.reorg()

This method handles chain reorganizations and:

- Identifies orphaned transactions (in old chain but not new chain)
- Re-adds orphaned transactions back to the Redis mempool
- Tracks metrics for reorg operations

## Error Handling

- All Redis operations are performed asynchronously to avoid blocking blockchain operations
- Errors are logged but do not prevent blockchain operations from continuing
- Metrics track error rates for monitoring purposes

## Example Usage

The integration is automatic and requires no manual intervention. When transactions are added to the mempool via the transaction manager:

```go
// Adding a transaction to Redis (handled by mempool)
bc.redisTxMgr.StoreTx(tx)

// When block is mined, transactions are automatically removed
// When reorg happens, orphaned transactions are automatically re-added
```

## Configuration

The Redis integration uses the default configuration from `redisstore.DefaultConfig()`. This includes:

- Redis connection settings
- TTL for transactions (10 days)
- Worker pool size for async operations
- Compression settings

## Monitoring

Monitor the following metrics to ensure proper operation:

1. `chain/redis/txremoval` - Should increase as blocks are mined
2. `chain/redis/txremoval/errors` - Should remain low
3. `chain/redis/reorg/add` - Increases during chain reorganizations
4. `chain/redis/reorg/errors` - Should remain low

## Performance Considerations

- Transaction removal operations are performed asynchronously
- Batch operations are used when possible to improve performance
- Duplicate caching prevents redundant operations
- Transaction lookup cache is purged during reorgs to maintain consistency
