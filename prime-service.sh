#!/bin/bash

# Prime Service Management Script

SERVICE_NAME="prime-server"
LOG_DIR="./logs"
PID_FILE="./prime-server.pid"
POOL_DIR="./prime_pool"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

start() {
    echo "🚀 Starting Prime Service..."

    # Check if already running
    if [ -f "$PID_FILE" ]; then
        PID=$(cat $PID_FILE)
        if ps -p $PID > /dev/null 2>&1; then
            echo -e "${RED}❌ Prime service is already running with PID: $PID${NC}"
            return 1
        else
            echo -e "${YELLOW}⚠️  Found stale PID file, removing...${NC}"
            rm $PID_FILE
        fi
    fi

    # Create log directory
    mkdir -p $LOG_DIR

    # Build the service
    echo "🔨 Building prime service..."
    go build -o $SERVICE_NAME cmd/server/main.go
    if [ $? -ne 0 ]; then
        echo -e "${RED}❌ Build failed${NC}"
        return 1
    fi

    # Start the service
    nohup ./$SERVICE_NAME > $LOG_DIR/prime-server.log 2>&1 &
    PID=$!
    echo $PID > $PID_FILE

    # Check if started successfully
    sleep 2
    if ps -p $PID > /dev/null; then
        echo -e "${GREEN}✅ Prime service started successfully${NC}"
        echo "   PID: $PID"
        echo "   Log: $LOG_DIR/prime-server.log"
        echo "   Pool: $POOL_DIR/"
    else
        echo -e "${RED}❌ Failed to start prime service${NC}"
        rm $PID_FILE
        return 1
    fi
}

stop() {
    echo "🛑 Stopping Prime Service..."

    if [ ! -f "$PID_FILE" ]; then
        echo -e "${YELLOW}⚠️  PID file not found. Checking for running processes...${NC}"
        PIDS=$(pgrep -f $SERVICE_NAME)
        if [ -z "$PIDS" ]; then
            echo -e "${GREEN}✅ Prime service is not running${NC}"
            return 0
        else
            for PID in $PIDS; do
                echo "   Stopping process $PID..."
                kill -TERM $PID
            done
            sleep 2
            for PID in $PIDS; do
                if ps -p $PID > /dev/null 2>&1; then
                    kill -9 $PID
                fi
            done
            echo -e "${GREEN}✅ Prime service stopped${NC}"
            return 0
        fi
    fi

    PID=$(cat $PID_FILE)

    if ! ps -p $PID > /dev/null 2>&1; then
        echo -e "${YELLOW}⚠️  Process $PID is not running${NC}"
        rm $PID_FILE
        return 0
    fi

    # Graceful shutdown
    kill -TERM $PID

    # Wait for process to stop
    COUNTER=0
    while [ $COUNTER -lt 10 ]; do
        if ! ps -p $PID > /dev/null 2>&1; then
            break
        fi
        sleep 1
        COUNTER=$((COUNTER + 1))
        echo -n "."
    done
    echo ""

    # Force kill if needed
    if ps -p $PID > /dev/null 2>&1; then
        echo -e "${YELLOW}⚠️  Process didn't stop gracefully, force killing...${NC}"
        kill -9 $PID
        sleep 1
    fi

    rm -f $PID_FILE
    echo -e "${GREEN}✅ Prime service stopped successfully${NC}"
    echo "   Prime pool saved to: $POOL_DIR/"
}

status() {
    echo "═══════════════════════════════════════"
    echo "       Prime Service Status"
    echo "═══════════════════════════════════════"
    echo ""

    # Check service status
    if [ -f "$PID_FILE" ]; then
        PID=$(cat $PID_FILE)
        if ps -p $PID > /dev/null 2>&1; then
            echo -e "${GREEN}✅ Status: RUNNING${NC}"
            echo "   PID: $PID"

            # Process info
            PS_INFO=$(ps -p $PID -o pid,vsz,rss,pcpu,pmem,etime,comm | tail -n 1)
            echo "   Memory: $(echo $PS_INFO | awk '{print $3/1024}') MB"
            echo "   CPU: $(echo $PS_INFO | awk '{print $4}')%"
            echo "   Uptime: $(echo $PS_INFO | awk '{print $6}')"
        else
            echo -e "${YELLOW}⚠️  Status: STOPPED (stale PID file)${NC}"
        fi
    else
        PIDS=$(pgrep -f $SERVICE_NAME)
        if [ ! -z "$PIDS" ]; then
            echo -e "${YELLOW}⚠️  Status: RUNNING (no PID file)${NC}"
            echo "   PID(s): $PIDS"
        else
            echo -e "${RED}❌ Status: NOT RUNNING${NC}"
        fi
    fi

    echo ""
    echo "📊 Prime Pool Statistics:"
    echo "───────────────────────"

    # Check prime pool
    if [ -d "$POOL_DIR" ]; then
        PRIME_FILES=$(find $POOL_DIR -name "primes_*.json" 2>/dev/null | wc -l | xargs)
        if [ "$PRIME_FILES" -gt 0 ]; then
            echo "   Pool files: $PRIME_FILES"
            TOTAL_PRIMES=$(grep -c '"value"' $POOL_DIR/primes_*.json 2>/dev/null | awk -F: '{sum += $2} END {print sum}')
            if [ ! -z "$TOTAL_PRIMES" ]; then
                echo "   Cached primes: ~$TOTAL_PRIMES"
            fi
            LATEST_FILE=$(ls -t $POOL_DIR/primes_*.json 2>/dev/null | head -n 1)
            if [ ! -z "$LATEST_FILE" ]; then
                echo "   Latest save: $(basename $LATEST_FILE)"
            fi
        else
            echo "   No prime files found"
        fi
    else
        echo "   Pool directory not found"
    fi

    # Show logs
    echo ""
    echo "📝 Recent Logs:"
    echo "───────────────────"
    if [ -f "$LOG_DIR/prime-server.log" ]; then
        tail -n 5 $LOG_DIR/prime-server.log | sed 's/^/   /'
    else
        echo "   No log file found"
    fi
    echo ""
}

restart() {
    echo "🔄 Restarting Prime Service..."
    stop
    echo ""
    sleep 2
    start
}

logs() {
    if [ -f "$LOG_DIR/prime-server.log" ]; then
        tail -f $LOG_DIR/prime-server.log
    else
        echo -e "${RED}❌ Log file not found${NC}"
    fi
}

clean() {
    echo "🧹 Cleaning Prime Service data..."

    # Stop service first
    stop

    echo ""
    read -p "Delete prime pool? (y/N): " -n 1 -r
    echo ""
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        rm -rf $POOL_DIR
        echo "   Deleted prime pool"
    fi

    read -p "Delete logs? (y/N): " -n 1 -r
    echo ""
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        rm -rf $LOG_DIR
        echo "   Deleted logs"
    fi

    echo -e "${GREEN}✅ Cleanup complete${NC}"
}

# Main command handler
case "$1" in
    start)
        start
        ;;
    stop)
        stop
        ;;
    status)
        status
        ;;
    restart)
        restart
        ;;
    logs)
        logs
        ;;
    clean)
        clean
        ;;
    *)
        echo "Prime Service Management"
        echo "Usage: $0 {start|stop|status|restart|logs|clean}"
        echo ""
        echo "Commands:"
        echo "  start    - Start the prime service"
        echo "  stop     - Stop the prime service"
        echo "  status   - Show service status and statistics"
        echo "  restart  - Restart the service"
        echo "  logs     - Tail the service logs"
        echo "  clean    - Stop service and clean data (with confirmation)"
        exit 1
        ;;
esac