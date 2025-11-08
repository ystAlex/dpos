package types

import "time"

// records.go
// 记录数据结构定义
// 用于追踪节点的历史行为

// ================================
// 第3章：节点表现评估 - 历史记录
// ================================

// VoteRecord 投票记录
// 记录节点每次投票的详细信息，用于计算：
// - 平均投票速率 V_i（第3.1.1节）
// - 历史投票行为 B_i（第3.1.2节）
type VoteRecord struct {
	// 基本信息
	Timestamp time.Time // 投票时间戳
	RoundID   int       // 所属轮次

	// 投票对象
	VotedFor string // 投票给的代理节点ID

	// 权重信息
	Weight float64 // 投票时的权重

	// 性能指标
	TimeCost float64 // 投票耗时（毫秒）
	// 用于计算 V_i = Σ(T_threshold - t_vote) / (n * T_threshold)

	Success bool // 是否成功
	// 用于计算 B_i = 1 / (1 + ln(γ/(γ+β)))

	// 代投相关
	IsProxy  bool   // 是否为代投
	ProxyFor string // 代投对象ID（如果是代投）

	// 补偿信息
	Compensation float64 // 获得的热补偿（第5.2节）
}

// BlockRecord 出块记录
// 记录代理节点每次出块的详细信息，用于计算：
// - 出块效率 P_j（第3.2.1节）
// - 验证效率 C_j（第3.2.2节）
type BlockRecord struct {
	// 基本信息
	Timestamp   time.Time // 出块时间戳
	BlockHeight int       // 区块高度
	RoundID     int       // 所属轮次

	// 出块结果
	Success bool // 是否成功出块
	// 用于 P_j = (B_valid - B_delayed - B_invalid) / B_max

	IsDelayed bool // 是否延迟
	// 延迟但有效的块不计入P_j分子

	IsInvalid bool // 是否无效
	// 无效块从P_j分子中扣除

	// 验证信息
	Validators        []string // 参与验证的节点ID列表
	CorrectValidators []string // 正确验证的节点ID列表
	// 用于计算 C_j = B_correct / B_allocated

	// 奖励信息
	Reward             float64            // 区块奖励（第5章）
	RewardDistribution map[string]float64 // 奖励分配详情
	// 基于修正Shapley值（第5.3节）
}

// PerformanceSnapshot 性能快照
// 周期性记录节点的综合表现，用于分析和可视化
type PerformanceSnapshot struct {
	// 时间信息
	Timestamp time.Time // 快照时间
	RoundID   int       // 轮次ID

	// 节点标识
	NodeID   string   // 节点ID
	NodeType NodeType // 节点类型

	// 权重信息（第2章）
	InitialWeight   float64 // 初始权重 W_i(0)
	CurrentWeight   float64 // 当前权重 W_i(t)
	WeightDecayRate float64 // 衰减率 λ = c*(1-R_i)
	HalfLife        float64 // 半衰期 t_1/2 = ln(2)/λ

	// 表现评分（第3章）
	PerformanceScore     float64 // 综合评分 R_i 或 E_j
	VotingSpeed          float64 // 投票速率 V_i
	VotingBehavior       float64 // 投票行为 B_i
	BlockEfficiency      float64 // 出块效率 P_j
	ValidationEfficiency float64 // 验证效率 C_j
	OnlineScore          float64 // 在线评分 O_j

	// 统计数据
	TotalVotes      int // 总投票次数
	SuccessfulVotes int // 成功投票次数
	FailedVotes     int // 失败投票次数
	TotalBlocks     int // 总出块数
	ValidBlocks     int // 有效区块数
	DelayedBlocks   int // 延迟区块数
	InvalidBlocks   int // 无效区块数

	// 经济数据（第5章）
	TotalReward       float64 // 累计奖励
	TotalCompensation float64 // 累计补偿
	CurrentDeposit    float64 // 当前保证金
}

// NetworkSnapshot 网络快照
// 记录整个网络在某一时刻的状态
type NetworkSnapshot struct {
	// 时间信息
	Timestamp time.Time // 快照时间
	RoundID   int       // 轮次ID

	// 网络统计
	TotalNodes        int     // 总节点数
	ActiveNodes       int     // 活跃节点数
	DelegateNodes     int     // 代理节点数
	TotalWeight       float64 // 总权重
	EnvironmentWeight float64 // 环境权重 W_env
	AverageWeight     float64 // 平均权重

	// 性能统计
	AveragePerformance float64 // 平均表现评分
	MedianPerformance  float64 // 中位数表现评分
	TopPerformance     float64 // 最高表现评分
	BottomPerformance  float64 // 最低表现评分

	// 网络健康度
	SystemNetworkDelay  float64 // 系统平均延迟（毫秒）
	VotingParticipation float64 // 投票参与率
	BlockSuccessRate    float64 // 出块成功率

	// 经济统计
	TotalRewardPool       float64 // 总奖励池
	TotalCompensationPool float64 // 总补偿池
	AverageNodeReward     float64 // 节点平均奖励
}
