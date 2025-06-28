#!/bin/bash

# Redis Integration Patch Application Script for go-ethereum
# This script applies our Redis integration to a fresh go-ethereum repository

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}Redis Integration Patch Application Script${NC}"
echo "=========================================="

# Check if we're in the right directory
if [ ! -f "go.mod" ] || ! grep -q "github.com/ethereum/go-ethereum" go.mod; then
    echo -e "${RED}Error: This doesn't appear to be a go-ethereum directory${NC}"
    echo "Please run this script from the root of a go-ethereum repository"
    exit 1
fi

# Check if Redis patch file exists
PATCH_FILE="redis-integration.patch"
if [ ! -f "../$PATCH_FILE" ]; then
    echo -e "${RED}Error: Redis integration patch file not found${NC}"
    echo "Expected: ../$PATCH_FILE"
    exit 1
fi

echo -e "${YELLOW}Step 1: Backing up current state...${NC}"
git stash push -m "Backup before Redis integration" || true

echo -e "${YELLOW}Step 2: Applying Redis integration patch...${NC}"
if git apply "../$PATCH_FILE"; then
    echo -e "${GREEN}✓ Patch applied successfully${NC}"
else
    echo -e "${RED}✗ Patch application failed${NC}"
    echo "This might be due to conflicts with the new go-ethereum version"
    echo "Manual integration may be required"
    exit 1
fi

echo -e "${YELLOW}Step 3: Installing dependencies...${NC}"
go mod tidy

echo -e "${YELLOW}Step 4: Running tests...${NC}"
if go test -v ./core/redisstore/...; then
    echo -e "${GREEN}✓ Redis store tests passed${NC}"
else
    echo -e "${RED}✗ Redis store tests failed${NC}"
    echo "Please check the test output above"
    exit 1
fi

echo -e "${YELLOW}Step 5: Building geth...${NC}"
if make geth; then
    echo -e "${GREEN}✓ Build successful${NC}"
else
    echo -e "${RED}✗ Build failed${NC}"
    echo "Please check the build output above"
    exit 1
fi

echo -e "${GREEN}Redis integration applied successfully!${NC}"
echo ""
echo "Next steps:"
echo "1. Start Redis server: redis-server"
echo "2. Test the integration: ./build/bin/geth --help | grep redis"
echo "3. Run with Redis: ./build/bin/geth --datadir ./testdata"
echo ""
echo "Configuration options:"
echo "  --redis.enabled         Enable Redis storage"
echo "  --redis.addr            Redis server address (default: localhost:6379)"
echo "  --redis.password        Redis password"
echo "  --redis.db              Redis database number"
