## 快速开始

### 环境要求
- Go 1.24+
- Linux/Mac (Windows需WSL)

- 安装依赖
- go mod tidy

### 启动多节点网络

chmod +x scripts/start_network.sh 
./scripts/start_network.sh 

参数为节点数目,这会启动5个节点的测试网络，包括:

1个种子节点(高权重优秀节点)
2个高权重节点(良好/差劲)
2个低权重节点(优秀/良好)



### 查看运行状态

# 查看日志
tail -f logs/node-0.log

# 查看所有节点
ls logs/

# 查看测试结果
ls test_results/


### 停止网络
./scripts/stop_network.sh
# 或按 Ctrl+C

## 详细使用

单节点启动
``
go run cmd/seed/main.go \
-id="node-1" \
-listen="localhost:8001" \
-seeds="localhost:8000" \
-weight=100 \
-perf=0.7 \
-delay=20 \
-time="" 
``
- 参数说明
  | 参数          | 说明       | 默认值            | 示例                              |
  | ----------- | --------    | --------------      | ------------------------------- |
  | `-id`       | 节点唯一标识   | (必需)           | "node-1"                        |
  | `-listen`   | 监听地址      | localhost:8000    | "localhost:8001"                |
  | `-seeds`    | 种子节点列表   | ""               | "localhost:8000,localhost:8001" |
  | `-weight`   | 初始权重      | 100.0             | 150.0                           |
  | `-perf`     | 初始表现评分   | 0.7              | 0.95                            |
  | `-delay`    | 网络延迟(ms)  | 20.0               | 15.0                            |
  | `-test`     | 启用测试模式   | false             | true                            |
  | `-rounds`   | 测试轮数       | 100                | 200                             |
  | `-output`   | 测试结果目录   | ./test_results     | ./results                       |
  | `-time`     | 整个genenis开始的时间格式要对   | 2006-01-02T15:04:05+07:00 | 当前时间往后推1分钟 |
  | `-mode`     | 测试模式   | full|throughput|latency|decentralization|voting|malicious | full |
 


启动测试模式
注意：脚本默认启动测试模式

``go run cmd/seed/main.go \
-id="test-node" \
-listen="localhost:9000" \
-weight=150 \
-perf=0.95 \
-test=true \
-rounds=100 \
-output="./my_results" \
-time="" \
-mode="full"
``



benchmark测试说明
测试场景
脚本会自动创建以下测试场景(对应文档表格):
| 节点类型        | 初始权重 | 表现评分 | 半衰期  | 预期结果 |
| ----------- | ---- | ---- | ---- | ---- |
| H-G (高权重优秀) | 150  | 0.95 | 69.3 | 权重稳定 |
| H-M (高权重良好) | 150  | 0.70 | 11.6 | 温和衰减 |
| H-B (高权重差劲) | 150  | 0.30 | 4.95 | 快速淘汰 |
| L-G (低权重优秀) | 50   | 0.95 | 69.3 | 权重稳定 |
| L-M (低权重良好) | 50   | 0.70 | 11.6 | 温和衰减 |
测试输出
测试会生成以下文件:
1. CSV数据文件 ({node-id}_results_{timestamp}.csv)
   每轮的详细数据
   可用于绘图分析
2. JSON报告 ({node-id}_report_{timestamp}.json)
   测试摘要
   统计分析
   性能指标

分析测试结果

# 查看CSV数据
cat test_results/node-0_results_*.csv | column -t -s','

# 查看JSON报告
cat test_results/node-0_report_*.json | jq .
