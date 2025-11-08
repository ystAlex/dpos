#!/bin/bash

# performance_test.sh
# 完整的性能测试套件

echo "=========================================="
echo "  NM-DPoS 完整性能测试套件"
echo "=========================================="
echo ""

# 创建结果目录
RESULT_DIR="results/perf_$(date +%Y%m%d_%H%M%S)"
mkdir -p "$RESULT_DIR"

echo "结果将保存到: $RESULT_DIR"
echo ""

# ================================
# 测试1：节点规模测试
# ================================
echo "【测试1】节点规模性能测试"
echo "测试不同节点数量对系统性能的影响"
echo ""

NODE_COUNTS=(50 100 200 500 1000 2000)
ROUNDS=50

for COUNT in "${NODE_COUNTS[@]}"; do
    echo "  测试 $COUNT 个节点..."

    START_TIME=$(date +%s)
    go run main.go -mode stress -nodes $COUNT -rounds $ROUNDS -log 0 > "$RESULT_DIR/nodes_${COUNT}.log" 2>&1
    END_TIME=$(date +%s)

    DURATION=$((END_TIME - START_TIME))
    echo "    完成，耗时: ${DURATION}秒"
done

echo ""

# ================================
# 测试2：权重衰减验证
# ================================
echo "【测试2】权重衰减公平性验证"
echo "验证相同表现节点的衰减率一致性"
echo ""

go run main.go -mode test -log 2 > "$RESULT_DIR/fairness_test.log" 2>&1
grep -A 15 "公平性验证" "$RESULT_DIR/fairness_test.log"

echo ""

# ================================
# 测试3：长期运行测试
# ================================
echo "【测试3】长期运行稳定性测试"
echo "测试系统在长时间运行下的表现"
echo ""

LONG_ROUNDS=200

echo "  运行 $LONG_ROUNDS 轮测试..."
go run main.go -mode normal -rounds $LONG_ROUNDS -log 1 > "$RESULT_DIR/long_run.log" 2>&1

# 提取关键指标
grep "最终总权重" "$RESULT_DIR/long_run.log"
grep "存活率" "$RESULT_DIR/long_run.log"
grep "平均成功率" "$RESULT_DIR/long_run.log"

echo ""

# ================================
# 测试4：不同衰减参数测试
# ================================
echo "【测试4】衰减参数敏感性测试"
echo "测试不同衰减常数的影响"
echo ""

# 注意：此测试需要修改config/constants.go中的DecayConstant
echo "  提示：需要手动修改config/constants.go中的DecayConstant值"
echo "  建议测试值: 0.1, 0.15, 0.2, 0.25, 0.3"
echo "  跳过此测试..."

echo ""

# ================================
# 生成测试报告
# ================================
echo "【生成测试报告】"
echo ""

cat > "$RESULT_DIR/README.md" << EOF
# NM-DPoS 性能测试报告

## 测试时间
$(date)

## 测试环境
- 操作系统: $(uname -s)
- CPU: $(sysctl -n machdep.cpu.brand_string 2>/dev/null || cat /proc/cpuinfo | grep "model name" | head -1 | cut -d: -f2)
- 内存: $(free -h 2>/dev/null | grep Mem | awk '{print $2}' || sysctl -n hw.memsize | awk '{print $1/1024/1024/1024 " GB"}')

## 测试项目
1. 节点规模测试 (50 - 2000节点)
2. 权重衰减公平性验证
3. 长期运行稳定性测试 ($LONG_ROUNDS轮)

## 测试文件
EOF

ls -lh "$RESULT_DIR"/*.log | awk '{print "- " $9}' >> "$RESULT_DIR/README.md"

echo "测试报告已生成: $RESULT_DIR/README.md"
echo ""
echo "=========================================="
echo "  性能测试套件完成！"
echo "=========================================="
echo ""
echo "查看结果: cat $RESULT_DIR/README.md"
echo "查看详细日志: ls $RESULT_DIR/"
