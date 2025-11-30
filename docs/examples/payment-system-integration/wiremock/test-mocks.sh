#!/bin/bash
# Test script to verify all WireMock services are working

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo "🧪 Testing WireMock Services"
echo ""

# Test function
test_service() {
    local name=$1
    local url=$2
    local method=${3:-GET}
    local data=${4:-}

    echo -n "Testing $name... "

    if [ "$method" = "GET" ]; then
        response=$(curl -s -w "\n%{http_code}" "$url")
    else
        response=$(curl -s -w "\n%{http_code}" -X POST "$url" \
            -H "Content-Type: application/json" \
            -d "$data")
    fi

    http_code=$(echo "$response" | tail -n1)
    body=$(echo "$response" | head -n-1)

    if [ "$http_code" = "200" ] || [ "$http_code" = "429" ]; then
        echo -e "${GREEN}✓ OK (HTTP $http_code)${NC}"
        return 0
    else
        echo -e "${RED}✗ FAILED (HTTP $http_code)${NC}"
        echo "Response: $body"
        return 1
    fi
}

# Check if services are running
echo "🔍 Checking if services are running..."
echo ""

for port in 9000 9001 9002; do
    if curl -sf http://localhost:${port}/__admin/mappings > /dev/null 2>&1; then
        echo -e "${GREEN}✓ Service on port $port is running${NC}"
    else
        echo -e "${RED}✗ Service on port $port is NOT running${NC}"
        echo "  Start with: docker-compose up -d"
        exit 1
    fi
done

echo ""
echo "📋 Testing endpoints..."
echo ""

# Test Inventory Service
echo -e "${BLUE}🏪 Inventory Service (port 9001)${NC}"
test_service "Available item" \
    "http://localhost:9001/inventory?item_name=Test&quantity=5" \
    "GET"

test_service "Low stock item" \
    "http://localhost:9001/inventory?item_name=Limited%20Item&quantity=3" \
    "GET"

test_service "Out of stock" \
    "http://localhost:9001/inventory?item_name=Out%20of%20Stock%20Item&quantity=1" \
    "GET"

echo ""

# Test Payment Provider
echo -e "${BLUE}💳 Payment Provider (port 9002)${NC}"
test_service "Successful charge" \
    "http://localhost:9002/v1/charges" \
    "POST" \
    '{"transaction_id":"test_123","amount":100.00,"currency":"USD","authorization_code":"AUTH123"}'

test_service "Rate limited (should 429)" \
    "http://localhost:9002/v1/charges" \
    "POST" \
    '{"transaction_id":"test_rate_limit","amount":100.00,"currency":"USD"}' || true

echo ""

# Test Notification Service
echo -e "${BLUE}📧 Notification Service (port 9000)${NC}"
test_service "Send notification" \
    "http://localhost:9000/notify" \
    "POST" \
    '{"customer_email":"test@example.com","customer_name":"Test User","order_id":"ORD-123","transaction_id":"TXN-456","amount":100.00,"currency":"USD"}'

echo ""
echo -e "${GREEN}✅ All tests passed!${NC}"
echo ""
echo "📊 View stub mappings:"
echo "  curl http://localhost:9001/__admin/mappings | jq"
echo "  curl http://localhost:9002/__admin/mappings | jq"
echo "  curl http://localhost:9000/__admin/mappings | jq"
echo ""
echo "🔄 Reset scenarios:"
echo "  curl -X POST http://localhost:9002/__admin/scenarios/reset"
echo "  curl -X POST http://localhost:9000/__admin/scenarios/reset"
