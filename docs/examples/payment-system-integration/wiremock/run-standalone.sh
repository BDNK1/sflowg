#!/bin/bash
# Standalone WireMock Runner
# Downloads and runs WireMock services for testing

set -e

WIREMOCK_VERSION="3.3.1"
WIREMOCK_JAR="wiremock-standalone-${WIREMOCK_VERSION}.jar"
WIREMOCK_URL="https://repo1.maven.org/maven2/org/wiremock/wiremock-standalone/${WIREMOCK_VERSION}/${WIREMOCK_JAR}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "🎭 WireMock Standalone Runner"
echo ""

# Download WireMock if not exists
if [ ! -f "$WIREMOCK_JAR" ]; then
    echo "📦 Downloading WireMock $WIREMOCK_VERSION..."
    curl -L -o "$WIREMOCK_JAR" "$WIREMOCK_URL"
    echo -e "${GREEN}✓ Downloaded WireMock${NC}"
else
    echo -e "${GREEN}✓ WireMock already downloaded${NC}"
fi

# Check Java
if ! command -v java &> /dev/null; then
    echo -e "${RED}❌ Java not found. Please install Java 11 or higher.${NC}"
    exit 1
fi

echo ""
echo "Starting mock services..."
echo "  🏪 Inventory Service:    http://localhost:9001"
echo "  💳 Payment Provider:     http://localhost:9002"
echo "  📧 Notification Service: http://localhost:9000"
echo ""
echo -e "${YELLOW}Press Ctrl+C to stop all services${NC}"
echo ""

# Create log directory
mkdir -p logs

# Function to cleanup on exit
cleanup() {
    echo ""
    echo "🛑 Stopping all services..."
    kill $INVENTORY_PID $PAYMENT_PID $NOTIFICATION_PID 2>/dev/null || true
    echo "✓ All services stopped"
    exit 0
}

trap cleanup SIGINT SIGTERM

# Start services in background
echo "Starting Inventory Service (port 9001)..."
java -jar "$WIREMOCK_JAR" \
    --port 9001 \
    --root-dir . \
    --global-response-templating \
    --verbose \
    > logs/inventory.log 2>&1 &
INVENTORY_PID=$!

echo "Starting Payment Provider (port 9002)..."
java -jar "$WIREMOCK_JAR" \
    --port 9002 \
    --root-dir . \
    --global-response-templating \
    --verbose \
    > logs/payment.log 2>&1 &
PAYMENT_PID=$!

echo "Starting Notification Service (port 9000)..."
java -jar "$WIREMOCK_JAR" \
    --port 9000 \
    --root-dir . \
    --global-response-templating \
    --verbose \
    > logs/notification.log 2>&1 &
NOTIFICATION_PID=$!

# Wait for services to start
sleep 3

# Check if services are running
check_service() {
    local port=$1
    local name=$2
    if curl -sf http://localhost:${port}/__admin/mappings > /dev/null 2>&1; then
        echo -e "${GREEN}✓ $name is running${NC}"
    else
        echo -e "${RED}✗ $name failed to start (check logs/$name.log)${NC}"
    fi
}

echo ""
check_service 9001 "Inventory Service"
check_service 9002 "Payment Provider"
check_service 9000 "Notification Service"

echo ""
echo "📊 View logs:"
echo "  tail -f logs/inventory.log"
echo "  tail -f logs/payment.log"
echo "  tail -f logs/notification.log"
echo ""
echo "🔧 Admin APIs:"
echo "  curl http://localhost:9001/__admin/mappings"
echo "  curl http://localhost:9002/__admin/mappings"
echo "  curl http://localhost:9000/__admin/mappings"
echo ""
echo "Press Ctrl+C to stop all services"

# Wait forever (until Ctrl+C)
wait
