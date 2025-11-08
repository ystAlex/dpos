#!/bin/bash

# run_batch.sh
# 批量启动P2P节点进行性能测试

echo "=========================================="
echo "  NM-DPoS 批量节点性能测试"
echo "=========================================="
echo ""

# 配置参数
NODE_COUNT=${1:-10}      # 节点数量（默认10）
START_PORT=${2:-8000}    # 起始端口（默认8000）
SEED_ADDR=${3:-localhost:9000}  # 种子节点地址
ROUNDS=${4:-30}          # 运行轮数（默认30）

echo "测试配置:"
echo "  节点数量: $NODE_COUNT"
echo "  端口范围: $START_PORT - $((START_PORT + NODE_COUNT - 1))"
echo "  种子节点: $SEED_ADDR"
echo "  运行轮次: $ROUNDS"
echo ""

# 检查种子节点是否运行
echo "检查种子节点连接..."
if ! curl -s "http://$SEED_ADDR/health" > /dev/null; then
    echo "错误: 种子节点 $SEED_ADDR 不可达"
    echo "请先运行: ./scripts/run_seed.sh"
    exit 1
fi

echo "种子节点连接成功！"
echo ""

# 创建日志目录
LOG_DIR="logs/batch_$(date +%Y%m%d_%H%M%S)"
mkdir -p "$LOG_DIR"

echo "启动 $NODE_COUNT 个节点..."
echo "日志目录: $LOG_DIR"
echo ""

# 批量启动节点
for i in $(seq 0 $((NODE_COUNT - 1))); do
    NODE_ID="BatchNode-$(printf "%04d" $i)"
    NODE_PORT=$((START_PORT + i))

    # 随机生成节点参数
    WEIGHT=$(awk -v min=50 -v max=150 'BEGIN{srand(); print min+rand()*(max-min)}')
    PERFORMANCE=$(awk -v min=0.3 -v max=0.95 'BEGIN{srand(); print min+rand()*(max-min)}')
    DELAY=$(awk -v min=10 -v max=50 'BEGIN{srand(); print min+rand()*(max-min)}')

    # 后台启动节点
    NODE_PORT=$NODE_PORT SEED_NODES=$SEED_ADDR \
    go run main.go -mode standalone \
        -node-id "$NODE_ID" \
        -node-weight "$WEIGHT" \
        -node-performance "$PERFORMANCE" \
        -node-delay "$DELAY" \
        -log 1 \
        > "$LOG_DIR/${NODE_ID}.log" 2>&1 &

    echo "  [$((i+1))/$NODE_COUNT] 启动节点 $NODE_ID (端口: $NODE_PORT)"

    # 每启动10个节点暂停一下
    if [ $((($i + 1) % 10)) -eq 0 ]; then
        sleep 1
    fi
done

echo ""
echo "所有节点已启动！"
echo ""
echo "性能监控运行中... (将运行约 $((ROUNDS * 3)) 秒)"
echo "  查看实时日志: tail -f $LOG_DIR/BatchNode-*.log"
echo "  查看种子节点: curl http://$SEED_ADDR/health"
echo ""
echo "按 Ctrl+C 停止所有节点"
echo ""

# 等待指定时间
sleep $((ROUNDS * 3))

echo ""
echo "测试时间到，正在停止所有节点..."

# 停止所有Go进程（谨慎使用）
pkill -f "go run main.go -mode standalone"

echo "所有节点已停止"
echo ""
echo "性能测试完成！"
echo "日志文件保存在: $LOG_DIR"
