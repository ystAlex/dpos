nm-dpos/
├── types/
│   ├── node.go              # 节点数据结构
│   ├── network.go           # 网络数据结构
│   └── records.go           # 记录数据结构
│
├── core/
│   ├── decay.go             # 权重衰减算法
│   ├── performance.go       # 节点表现评估
│   ├── voting.go            # 动态投票机制
│   └── reward.go            # 奖励分配算法
│
├── network/
│   ├── communication.go   # 区块和交易池
│   ├── local_view.go      # 本地网络视图
│   └── sync.go              # 区块同步
│
├── p2p/                     # P2P 网络层
│   ├── discovery.go         # 节点发现
│   ├── peer.go              # 对等节点管理
│   ├── transport.go         # 网络传输（HTTP/WebSocket）
│
├── node/
│   └──p2p_node.go          # P2P 独立节点（替换 standalone.go）
│
├── simulation/
│   └── simulator.go         # 集中式模拟（测试用）
├── config/
│   └── constants.go        # 全局常量
└── utils/
│   ├── math.go              # 数学工具
│   └── logger.go            # 日志工具
└── cmd/
    └── seed/
        └── main.go       # 种子节点程序



双模式支持：
集中式模式（simulation/）：用于性能测试和实验验证
分布式模式（p2p/）：真实的P2P网络运行




运行模式
1. 集中式模拟（性能测试推荐）
# 标准测试（50轮）
go run main.go -mode normal -rounds 50

# 快速测试（10轮）
go run main.go -mode normal -rounds 10 -log 1

# 长期测试（200轮）
go run main.go -mode normal -rounds 200

2. 单节点P2P运行
# 终端1：启动种子节点
go run cmd/seed/main.go -port 9000

# 终端2：启动节点1
NODE_PORT=8001 SEED_NODES=localhost:9000 \
go run main.go -mode standalone -node-id Node-001 -node-weight 150 -node-performance 0.95

# 终端3：启动节点2
NODE_PORT=8002 SEED_NODES=localhost:9000 \
go run main.go -mode standalone -node-id Node-002 -node-weight 100 -node-performance 0.7

3. 批量性能测试（推荐）
# 启动种子节点
make seed

# 另一个终端：批量启动10个节点，运行30轮
./scripts/run_batch.sh 10 8000 localhost:9000 30

# 或使用make
make batch

4. 压力测试
# 1000节点，20轮
go run main.go -mode stress -nodes 1000 -rounds 20

# 或使用make
make stress

5. 功能测试
# 运行所有测试用例
go run main.go -mode test

# 或使用make
make test

性能测试说明
集中式性能测试（推荐用于论文实验）
# 方式1：直接运行
go run main.go -mode normal -rounds 100 -nodes 50

# 方式2：使用Makefile
make run

# 方式3：压力测试
make stress


输出指标：

权重演化曲线

节点表现分布

投票参与率

出块成功率

TPS（每秒交易数）

平均延迟

批量P2P性能测试
# 10个真实P2P节点
./scripts/run_batch.sh 10 8000 localhost:9000 50

# 50个节点
./scripts/run_batch.sh 50 8000 localhost:9000 30

完整性能测试套件
./scripts/performance_test.sh


自动运行：

节点规模测试（50-2000节点）

公平性验证

长期稳定性测试

生成测试报告

命令行参数
-mode string
运行模式 (默认: "normal")
可选: normal, standalone, cluster, batch, test, stress

-rounds int
模拟轮数 (默认: 50)

-nodes int
节点数量 (默认: 50)

-log int
日志级别 (默认: 2)
0=ERROR, 1=WARN, 2=INFO, 3=DEBUG

-node-id string
节点ID (默认: "Node-001")

-node-weight float
节点初始权重 (默认: 100.0)

-node-performance float
节点表现评分 (默认: 0.7)

-cluster-size int
集群节点数量 (默认: 10)

-start-port int
起始端口号 (默认: 8000)

-seed string
种子节点地址 (默认: "localhost:9000")

实验验证
验证第2章：权重衰减
go run main.go -mode test


查看输出中的"权重演化表"，对比理论值。

验证第3章：表现评估
go run main.go -mode normal -rounds 50


查看最终报告中的"节点表现分布"。

验证第4章：投票机制
go run main.go -mode test


查看"投票区段动态调整测试"输出。

验证第5章：奖励分配
go run main.go -mode test


查看"修正Shapley值分配测试"输出。

项目结构
nm-dpos/
├── config/          # 配置
├── types/           # 数据结构
├── core/            # 核心算法
├── network/         # 网络管理
├── p2p/             # P2P网络
├── node/            # 独立节点
├── simulation/      # 模拟框架
├── utils/           # 工具函数
├── cmd/seed/        # 种子节点
└── scripts/         # 测试脚本

许可证

MIT License


---

现在所有代码已经完整，包括：

✅ **完整的代理投票实现**（在 `core/voting.go` 中）
✅ **修正的 Simulator**（使用 `LocalNetworkView`）
✅ **完整的 LocalNetworkView**（包含 Network 和 MessageBus）
✅ **批量启动脚本**（一条命令启动多个节点）
✅ **性能测试套件**（符合PDF实验要求）
✅ **种子节点程序**
✅ **完整的文档和使用说明**

可以立即运行测试！




# dpos
