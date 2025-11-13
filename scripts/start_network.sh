#!/bin/bash

set -e

# 配置
NUM_NODES=5
BASE_PORT=12000
TEST_MODE=true
TEST_ROUNDS=100
MODE="full"
OUTPUT_DIR="./test_results"

# 颜色输出
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}NM-DPoS 多节点网络启动脚本${NC}"
echo -e "${GREEN}========================================${NC}"

# ============================================
# 计算统一的创世时间（当前时间+1分钟）
# ============================================
GENESIS_TIME=$(date -u -d '+1 minute' '+%Y-%m-%dT%H:%M:%S+00:00' 2>/dev/null || date -u -v+1M '+%Y-%m-%dT%H:%M:%S+00:00')

echo -e "${GREEN}创世时间: ${GENESIS_TIME}${NC}"
echo -e "${YELLOW}所有节点将在此时间同步启动区块产生${NC}"
echo ""

# 清理
echo -e "${YELLOW}清理旧进程和日志...${NC}"
pkill -f "bin/nm-dpos" || true
sleep 2
rm -rf logs
rm -rf ${OUTPUT_DIR}
mkdir -p logs
mkdir -p ${OUTPUT_DIR}

# 编译
echo -e "${YELLOW}编译程序...${NC}"
go build -o bin/nm-dpos ../cmd/seed/main.go

# 健康检查函数
check_node_ready() {
    local addr=$1
    local max_attempts=30

    for i in $(seq 1 $max_attempts); do
        if curl -s "http://${addr}/status" > /dev/null 2>&1; then
            echo -e "${GREEN}✓ 节点 ${addr} 已就绪${NC}"
            return 0
        fi
        sleep 1
    done

    echo -e "${RED}✗ 节点 ${addr} 启动超时${NC}"
    return 1
}

# 启动种子节点
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}1. 启动种子节点 (node-0)${NC}"
echo -e "${GREEN}========================================${NC}"

SEED_ADDR="localhost:${BASE_PORT}"

./bin/nm-dpos \
    -id="node-0" \
    -listen="${SEED_ADDR}" \
    -weight=150 \
    -perf=0.95 \
    -delay=10 \
    -time="${GENESIS_TIME}" \
    -test=${TEST_MODE} \
    -rounds=${TEST_ROUNDS} \
    -mode=${MODE} \
    -output=${OUTPUT_DIR} \
    > logs/node-0.log 2>&1 &

echo "PID: $!"

echo -e "${YELLOW}等待种子节点启动...${NC}"
check_node_ready "${SEED_ADDR}" || exit 1

echo ""
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}2. 启动其他节点${NC}"
echo -e "${GREEN}========================================${NC}"

# 启动其他节点
for i in $(seq 1 $((NUM_NODES-1))); do
    PORT=$((BASE_PORT + i))
    ADDR="localhost:${PORT}"

    # 不同配置
    case $i in
        1) WEIGHT=150; PERF=0.70; DELAY=20 ;;
        2) WEIGHT=150; PERF=0.30; DELAY=30 ;;
        3) WEIGHT=50;  PERF=0.95; DELAY=15 ;;
        4) WEIGHT=50;  PERF=0.70; DELAY=25 ;;
    esac

    echo ""
    echo -e "${GREEN}启动节点 node-${i}${NC}"
    echo "  地址: ${ADDR}"
    echo "  种子: ${SEED_ADDR}"
    echo "  配置: 权重=${WEIGHT}, 表现=${PERF}, 延迟=${DELAY}ms"
    echo "  创世时间: ${GENESIS_TIME}"

    ./bin/nm-dpos \
        -id="node-${i}" \
        -listen="${ADDR}" \
        -seeds="${SEED_ADDR}" \
        -weight=${WEIGHT} \
        -perf=${PERF} \
        -delay=${DELAY} \
        -time="${GENESIS_TIME}" \
        -test=${TEST_MODE} \
        -rounds=${TEST_ROUNDS} \
        -output=${OUTPUT_DIR} \
        > logs/node-${i}.log 2>&1 &

    echo "PID: $!"

    echo -e "${YELLOW}等待节点 node-${i} 启动...${NC}"
    check_node_ready "${ADDR}"

    sleep 3  # 给节点间发现留出时间
done

# 验证连接
echo ""
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}3. 验证节点连接状态${NC}"
echo -e "${GREEN}========================================${NC}"

sleep 10  # 等待节点发现完成

echo ""
for i in $(seq 0 $((NUM_NODES-1))); do
    PORT=$((BASE_PORT + i))
    PEER_COUNT=$(curl -s "http://localhost:${PORT}/status" | grep -o '"connected_peers":[0-9]*' | cut -d: -f2)

    if [ -z "$PEER_COUNT" ] || [ "$PEER_COUNT" = "0" ]; then
        echo -e "node-${i}: ${RED}${PEER_COUNT:-0} 对等节点${NC}"
    else
        echo -e "node-${i}: ${GREEN}${PEER_COUNT} 对等节点${NC}"
    fi
done

# 显示创世时间倒计时
echo ""
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}等待创世时间到达...${NC}"
echo -e "${GREEN}========================================${NC}"

GENESIS_TIMESTAMP=$(date -d "${GENESIS_TIME}" +%s 2>/dev/null || date -j -f "%Y-%m-%dT%H:%M:%S+00:00" "${GENESIS_TIME}" +%s)

while true; do
    NOW=$(date +%s)
    REMAINING=$((GENESIS_TIMESTAMP - NOW))

    if [ $REMAINING -le 0 ]; then
        echo -e "\n${GREEN}✓ 创世时间已到达！区块产生已开始${NC}"
        break
    fi

    echo -ne "\r${YELLOW}倒计时: ${REMAINING} 秒...${NC}"
    sleep 1
done

echo ""
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}启动完成！${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""
echo "创世时间: ${GENESIS_TIME}"
echo "查看日志: tail -f logs/node-0.log"
echo "查看所有日志: ls logs/"
echo "停止网络: ./scripts/stop_network.sh 或按 Ctrl+C"
echo ""

# 等待中断
trap 'echo -e "\n${RED}停止所有节点...${NC}"; pkill -f "bin/nm-dpos"; exit 0' INT TERM

wait
