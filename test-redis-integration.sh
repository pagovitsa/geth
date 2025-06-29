#!/bin/bash

# Test script to check Redis transaction functionality

echo "=== Testing Redis Transaction Integration ==="

# Check if Redis is running
echo "1. Checking Redis connection..."
redis-cli ping

# Start with a clean Redis
echo "2. Flushing Redis database..."
redis-cli flushall

# Show initial Redis state
echo "3. Initial Redis keys:"
redis-cli keys "tx:*"

echo ""
echo "Redis transaction debugging setup complete."
echo "Now run geth and watch for Redis transaction events."
echo ""
echo "To monitor Redis in real-time, run:"
echo "  watch -n 1 'redis-cli keys \"tx:*\" | wc -l'"
echo ""
echo "To see transaction keys:"
echo "  redis-cli keys \"tx:*\""
echo ""
echo "To see a specific transaction:"
echo "  redis-cli hgetall \"tx:HASH\""
