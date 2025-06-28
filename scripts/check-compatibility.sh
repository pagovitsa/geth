#!/bin/bash

# Redis Integration Compatibility Checker for go-ethereum
# This script checks if a new go-ethereum version is compatible with our Redis integration

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}Redis Integration Compatibility Checker${NC}"
echo "========================================"

# Check if we're in a go-ethereum directory
if [ ! -f "go.mod" ] || ! grep -q "github.com/ethereum/go-ethereum" go.mod; then
    echo -e "${RED}Error: This doesn't appear to be a go-ethereum directory${NC}"
    exit 1
fi

echo -e "${YELLOW}Checking go-ethereum version and structure...${NC}"

# Get version info
if [ -f "params/version.go" ]; then
    VERSION=$(grep -E "VersionMajor|VersionMinor|VersionPatch" params/version.go | head -3)
    echo "Version info found:"
    echo "$VERSION"
else
    echo -e "${RED}Warning: Could not find version information${NC}"
fi

echo ""
echo -e "${YELLOW}Checking critical files and structures...${NC}"

# Check critical files that our integration depends on
CRITICAL_FILES=(
    "core/blockchain.go"
    "core/txpool/legacypool/legacypool.go"
    "go.mod"
)

COMPATIBILITY_SCORE=0
TOTAL_CHECKS=0

for file in "${CRITICAL_FILES[@]}"; do
    echo -n "Checking $file... "
    if [ -f "$file" ]; then
        echo -e "${GREEN}✓ Found${NC}"
        ((COMPATIBILITY_SCORE++))
    else
        echo -e "${RED}✗ Missing${NC}"
    fi
    ((TOTAL_CHECKS++))
done

echo ""
echo -e "${YELLOW}Checking critical functions and structures...${NC}"

# Check for critical functions in blockchain.go
if [ -f "core/blockchain.go" ]; then
    echo -n "Checking BlockChain struct... "
    if grep -q "type BlockChain struct" core/blockchain.go; then
        echo -e "${GREEN}✓ Found${NC}"
        ((COMPATIBILITY_SCORE++))
    else
        echo -e "${RED}✗ Not found or changed${NC}"
    fi
    ((TOTAL_CHECKS++))
    
    echo -n "Checking NewBlockChain function... "
    if grep -q "func NewBlockChain" core/blockchain.go; then
        echo -e "${GREEN}✓ Found${NC}"
        ((COMPATIBILITY_SCORE++))
    else
        echo -e "${RED}✗ Not found or changed${NC}"
    fi
    ((TOTAL_CHECKS++))
    
    echo -n "Checking block processing functions... "
    if grep -qE "(writeBlockWithState|insertChain|writeBlock)" core/blockchain.go; then
        echo -e "${GREEN}✓ Found${NC}"
        ((COMPATIBILITY_SCORE++))
    else
        echo -e "${RED}✗ Not found or changed${NC}"
    fi
    ((TOTAL_CHECKS++))
fi

# Check transaction pool structure
if [ -f "core/txpool/legacypool/legacypool.go" ]; then
    echo -n "Checking LegacyPool struct... "
    if grep -q "type LegacyPool struct" core/txpool/legacypool/legacypool.go; then
        echo -e "${GREEN}✓ Found${NC}"
        ((COMPATIBILITY_SCORE++))
    else
        echo -e "${RED}✗ Not found or changed${NC}"
    fi
    ((TOTAL_CHECKS++))
    
    echo -n "Checking transaction addition functions... "
    if grep -qE "(func.*add|func.*Add)" core/txpool/legacypool/legacypool.go; then
        echo -e "${GREEN}✓ Found${NC}"
        ((COMPATIBILITY_SCORE++))
    else
        echo -e "${RED}✗ Not found or changed${NC}"
    fi
    ((TOTAL_CHECKS++))
else
    echo -e "${YELLOW}Checking alternative transaction pool locations...${NC}"
    if [ -f "core/txpool/pool.go" ]; then
        echo -e "${YELLOW}Found core/txpool/pool.go - may need adaptation${NC}"
    elif [ -f "core/tx_pool.go" ]; then
        echo -e "${YELLOW}Found core/tx_pool.go - may need adaptation${NC}"
    else
        echo -e "${RED}No transaction pool implementation found${NC}"
    fi
fi

echo ""
echo -e "${YELLOW}Checking for potential conflicts...${NC}"

# Check if redisstore directory already exists
if [ -d "core/redisstore" ]; then
    echo -e "${YELLOW}Warning: core/redisstore directory already exists${NC}"
    echo "This might indicate a previous integration or conflict"
fi

# Check for Redis-related imports
if grep -r "redisstore" . --exclude-dir=.git >/dev/null 2>&1; then
    echo -e "${YELLOW}Warning: Found existing redisstore references${NC}"
    echo "Locations:"
    grep -r "redisstore" . --exclude-dir=.git | head -5
fi

# Check go.mod for Redis dependencies
if grep -q "redis" go.mod; then
    echo -e "${YELLOW}Info: Found existing Redis dependencies${NC}"
    grep "redis" go.mod
fi

echo ""
echo -e "${BLUE}Compatibility Assessment${NC}"
echo "======================="

PERCENTAGE=$((COMPATIBILITY_SCORE * 100 / TOTAL_CHECKS))

echo "Score: $COMPATIBILITY_SCORE/$TOTAL_CHECKS ($PERCENTAGE%)"

if [ $PERCENTAGE -ge 80 ]; then
    echo -e "${GREEN}✓ High compatibility - Automatic patch likely to work${NC}"
    echo "Recommendation: Try automatic patch first"
elif [ $PERCENTAGE -ge 60 ]; then
    echo -e "${YELLOW}⚠ Medium compatibility - Manual integration may be needed${NC}"
    echo "Recommendation: Try automatic patch, be prepared for manual fixes"
else
    echo -e "${RED}✗ Low compatibility - Manual integration required${NC}"
    echo "Recommendation: Use manual integration guide"
fi

echo ""
echo -e "${BLUE}Next Steps${NC}"
echo "=========="

if [ $PERCENTAGE -ge 80 ]; then
    echo "1. Run: ./apply-redis-patch.sh"
    echo "2. If that fails, use manual-integration-guide.md"
elif [ $PERCENTAGE -ge 60 ]; then
    echo "1. Try: ./apply-redis-patch.sh"
    echo "2. If conflicts occur, resolve manually using manual-integration-guide.md"
    echo "3. Pay attention to changed function signatures"
else
    echo "1. Use manual-integration-guide.md"
    echo "2. Carefully adapt integration points to new structure"
    echo "3. Test thoroughly after integration"
fi

echo ""
echo "Additional resources:"
echo "- redis-integration-patch.md: Complete integration overview"
echo "- manual-integration-guide.md: Step-by-step manual integration"
echo "- Redis store tests: go test -v ./core/redisstore/..."
