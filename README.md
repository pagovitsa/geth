# go-ethereum with Redis Integration

This is a fork of [go-ethereum](https://github.com/ethereum/go-ethereum) with integrated Redis storage for enhanced transaction and block management.

## 🚀 Key Features

### Redis Integration
- **Smart Blockchain Number Caching**: Efficient caching mechanism that only updates when new blocks are processed
- **Complete Transaction Storage**: All transaction fields including hash, nonce, from, to, rawdata, gasprice, gas, value, type, maxfeepergas, maxpriorityfeepergas, and blockchain_number
- **Block Storage**: Stores blocknumber, blockgasprice, txshashes (JSON), and txslogs (JSON)
- **Data Consistency**: All hash and address data stored in lowercase format
- **Performance Optimizations**: TTL management, startup optimization, worker pools, duplicate prevention

### Production Ready
- ✅ Tested with Ethereum mainnet
- ✅ Handles 11,447+ existing transactions
- ✅ Complete test suite (6/6 tests passing)
- ✅ Smart caching for optimal performance
- ✅ Graceful error handling

## 📁 Redis Integration Files

### Core Module (`core/redisstore/`)
- `config.go` - Redis configuration management
- `store.go` - Block and log storage implementation
- `txmanager.go` - Transaction management with blockchain number caching
- `store_test.go` - Comprehensive test suite
- `compression.go` - Data compression utilities

### Integration Points
- `core/blockchain.go` - Modified for Redis block storage
- `core/txpool/legacypool/legacypool.go` - Modified for Redis transaction storage
- `go.mod` - Updated with Redis dependencies

## 🔧 Quick Start

### Prerequisites
```bash
# Install Redis
sudo apt-get install redis-server
# or
brew install redis
```

### Build and Run
```bash
# Clone this repository
git clone https://github.com/pagovitsa/geth.git
cd geth

# Install dependencies
go mod tidy

# Build geth
make geth

# Start Redis
redis-server &

# Run geth with Redis integration
./build/bin/geth --cache=25536 --datadir ./data
```

### Verify Redis Integration
```bash
# Check Redis for stored data
redis-cli keys "tx:*" | head -5
redis-cli keys "block:*" | head -5

# Monitor Redis activity
redis-cli monitor
```

## 📊 Performance

### Optimizations Implemented
- **Smart Caching**: Blockchain number cached and updated only on new blocks
- **Worker Pools**: Async transaction processing with 10 workers
- **Duplicate Prevention**: Hash-based cache prevents duplicate storage
- **Compression**: Optional data compression for storage efficiency
- **Connection Pooling**: Redis connection pool for optimal performance

### Production Metrics
- **Startup Optimization**: Loads existing transaction hashes (11,447+ in production)
- **Memory Efficient**: Minimal memory overhead with smart caching
- **High Throughput**: Handles Ethereum mainnet transaction volume
- **Fault Tolerant**: Continues operation if Redis is unavailable

## 🧪 Testing

```bash
# Run Redis store tests
go test -v ./core/redisstore/...

# Run all tests
make test

# Build verification
make geth
```

## ⚙️ Configuration

### Redis Configuration
The Redis integration can be configured through environment variables or code:

```go
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
```

### TTL Settings
- **Blocks**: 60 seconds (short-lived for recent block data)
- **Transactions**: 10 days (longer retention for transaction history)

## 🔄 Updating from Upstream

This repository includes tools for applying the Redis integration to future go-ethereum updates:

### Automatic Patching
```bash
# Check compatibility with new version
./check-compatibility.sh

# Apply Redis integration automatically
./apply-redis-patch.sh
```

### Manual Integration
See `manual-integration-guide.md` for detailed steps when automatic patching fails.

## 📚 Documentation

- `QUICK-START-GUIDE.md` - Simple setup instructions
- `manual-integration-guide.md` - Manual integration steps
- `redis-integration-patch.md` - Technical overview
- `README-Redis-Integration.md` - Complete Redis integration documentation

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Run tests: `go test -v ./core/redisstore/...`
5. Submit a pull request

## 📄 License

This project inherits the license from go-ethereum. See [COPYING](COPYING) and [COPYING.LESSER](COPYING.LESSER) for details.

## 🔗 Links

- [Original go-ethereum](https://github.com/ethereum/go-ethereum)
- [Redis](https://redis.io/)
- [Go Redis Client](https://github.com/go-redis/redis)

## 📞 Support

For Redis integration specific issues:
- Check Redis connectivity: `redis-cli ping`
- Verify configuration in logs
- Run Redis store tests independently
- Review documentation in the `docs/` directory

---

**Note**: This integration is designed to be minimally invasive and optional. It can be disabled without affecting core go-ethereum functionality.
