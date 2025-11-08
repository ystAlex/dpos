package types

import "time"

// network.go
// 网络数据结构定义
// 管理整个DPoS网络的状态

// Network DPoS网络
// 核心管理结构，协调所有节点和共识流程
type Network struct {
	// ================================
	// 节点管理
	// ================================

	// 所有节点列表
	Nodes []*Node

	// 当前代理节点列表
	// 通过选举产生，数量为 NumDelegates
	Delegates []*Node

	// ================================
	// 第2章：权重统计（环境权重计算）
	// ================================

	// 全网总权重
	// TotalWeight = Σ W_i（所有活跃节点）
	TotalWeight float64

	// 环境权重 W_env（第1.3节）
	// 公式：W_env = (总权重 - 最大 - 最小) / (节点数 - 2)
	// 本方案设为0，实现纯指数衰减
	EnvironmentWeight float64

	// 平均权重
	// AverageWeight = TotalWeight / ActiveNodesCount
	AverageWeight float64

	// 活跃节点数量
	ActiveNodesCount int

	// ================================
	// 第4章：动态投票机制相关
	// ================================

	// 系统平均网络延迟 D_s（毫秒）
	// 用于计算正常投票区段
	SystemNetworkDelay float64

	// 当前正常投票区段时间（毫秒）
	// 公式：T_normal = T_base * (1 + ΣD_i/(n*D_s)) * ln(1+N_vote/N_all)
	NormalVotingPeriod float64

	// 代投区段时间（毫秒）
	// ProxyVotingPeriod = ProxyPeriodEnd - ProxyPeriodStart
	ProxyVotingPeriod float64

	// 本轮参与投票的节点数
	ParticipatingVoters int

	// ================================
	// 第5章：权重补偿与奖励分配
	// ================================

	// 本轮扣除的总权重（第5.2节）
	// 来自未投票或投票失败的节点
	// 将分配给成功投票的节点作为热补偿
	TotalDeductedWeight float64

	// 本轮没收的保证金总额
	// 来自投票失败和出块失败的节点
	TotalConfiscatedDeposit float64

	// 热补偿资金池
	// Pool_hot = TotalDeductedWeight + TotalConfiscatedDeposit
	HotCompensationPool float64

	// ================================
	// 区块链状态
	// ================================

	// 当前轮次
	// 一轮 = 所有代理节点各出一个块
	CurrentRound int

	// 当前区块高度
	CurrentBlockHeight int

	// 最新区块记录
	LatestBlock *BlockRecord

	// ================================
	// 历史记录
	// ================================

	// 网络快照历史
	SnapshotHistory []NetworkSnapshot
}

// NewNetwork 创建新网络实例
func NewNetwork() *Network {
	return &Network{
		Nodes:              make([]*Node, 0),
		Delegates:          make([]*Node, 0),
		CurrentRound:       0,
		CurrentBlockHeight: 0,
		NormalVotingPeriod: 50.0, // InitialNormalPeriod
		ProxyVotingPeriod:  50.0, // ProxyPeriodEnd - ProxyPeriodStart
		SnapshotHistory:    make([]NetworkSnapshot, 0),
	}
}

// VotingStatistics 投票统计信息
// 用于投票阶段的详细分析
type VotingStatistics struct {
	// 总投票节点数
	TotalVoters int

	// 成功投票节点数（在正常区段完成）
	SuccessfulVoters int

	// 代投节点数（在代投区段完成）
	ProxiedVoters int

	// 失败投票节点数（超时或无代投）
	FailedVoters int

	// 平均投票时间（毫秒）
	AverageVotingTime float64

	// 正常投票区段时间（毫秒）
	NormalPeriod float64

	// 代投区段时间（毫秒）
	ProxyPeriod float64

	// 投票参与率
	// ParticipationRate = (SuccessfulVoters + ProxiedVoters) / TotalVoters
	ParticipationRate float64
}

// BlockStatistics 出块统计信息
// 用于出块阶段的详细分析
type BlockStatistics struct {
	// 总区块数
	TotalBlocks int

	// 有效区块数
	ValidBlocks int

	// 延迟区块数
	DelayedBlocks int

	// 无效区块数
	InvalidBlocks int

	// 平均出块时间（秒）
	AverageBlockTime float64

	// 出块成功率
	// SuccessRate = ValidBlocks / TotalBlocks
	SuccessRate float64

	// 总奖励分配
	TotalRewardDistributed float64

	// 平均区块奖励
	AverageBlockReward float64
}

// RoundSummary 轮次摘要
// 总结一轮的完整信息
type RoundSummary struct {
	// 轮次ID
	RoundID int

	// 开始和结束时间
	StartTime time.Time
	EndTime   time.Time

	// 选举的代理节点列表
	DelegateIDs []string

	// 投票统计
	VotingStats VotingStatistics

	// 出块统计
	BlockStats BlockStatistics

	// 网络状态快照
	NetworkSnapshot NetworkSnapshot

	// 关键事件
	KeyEvents []string // 例如：节点掉线、作恶行为、权重归零等
}
