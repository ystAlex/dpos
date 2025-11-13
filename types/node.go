package types

import (
	"sync"
	"time"
)

// node.go
// 节点数据结构定义
// 包含节点的所有状态和行为

// NodeType 节点类型枚举
type NodeType int

const (
	// VoterNode 投票节点
	// 参与投票选举代理节点（第4章）
	VoterNode NodeType = iota

	// DelegateNode 代理节点
	// 负责出块和验证（第3.2节）
	DelegateNode
)

// String 实现Stringer接口
func (nt NodeType) String() string {
	switch nt {
	case VoterNode:
		return "Voter"
	case DelegateNode:
		return "Delegate"
	default:
		return "Unknown"
	}
}

// Int 实现Int接口
func (nt NodeType) Int(nodeType string) NodeType {
	switch nodeType {
	case "Voter":
		return VoterNode
	case "Delegate":
		return DelegateNode
	default:
		return NodeType(-1)
	}
}

// Node 网络节点
// 核心数据结构，包含节点的所有状态信息
type Node struct {
	mu sync.RWMutex
	// ================================
	// 基本信息
	// ================================
	ID   string   // 节点唯一标识符
	Type NodeType // 节点类型（投票者/代理者）

	// ================================
	// 第2章：权重衰减相关
	// ================================

	// 初始权重 W_i(0)
	// 来自质押的代币数量，是衰减计算的基准
	InitialWeight float64

	// 当前权重 W_i(t)
	// 根据牛顿冷却公式动态衰减
	// 公式：W_i(t) = W_env + (W_i(0) - W_env) * e^[-c*(1-R_i)*t]
	CurrentWeight float64

	// 上次权重更新时间
	// 用于计算时间周期 t
	LastUpdateTime time.Time

	// 是否活跃
	// CurrentWeight < MinActiveWeight 时自动设为false
	IsActive bool

	// ================================
	// 第3章：节点表现评估相关
	// ================================

	// 综合表现评分 R_i（投票节点）或 E_j（代理节点）
	// 取值范围：[0, 1]
	// 影响权重衰减速度：衰减率 λ = c * (1 - R_i)
	PerformanceScore float64

	// --- 投票节点专用指标 ---

	// 平均投票速率 V_i（第3.1.1节）
	// 公式：V_i = Σ(T_threshold - t_vote) / (n * T_threshold)
	VotingSpeed float64

	// 历史投票行为 B_i（第3.1.2节）
	// 公式：B_i = 1 / (1 + ln(γ/(γ+β)))
	VotingBehavior float64

	// 成功投票次数 γ
	SuccessfulVotes int

	// 失败投票次数 β
	FailedVotes int

	// --- 代理节点专用指标 ---

	// 出块效率 P_j（第3.2.1节）
	// 公式：P_j = (B_valid - B_delayed - B_invalid) / B_max
	BlockEfficiency float64

	// 验证效率 C_j（第3.2.2节）
	// 公式：C_j = B_correct / B_allocated
	ValidationEfficiency float64

	// 在线时长（周期数）
	// 用于计算 O_j = 1 / (1 + e^(-t_j/τ))
	OnlineTime float64

	// 有效区块数
	ValidBlocks int

	// 延迟区块数
	DelayedBlocks int

	// 无效区块数
	InvalidBlocks int

	// 正确验证数
	CorrectValidations int

	// 总验证数
	TotalValidations int

	// 出块状态追踪
	BlockProductionStatus map[int]bool // roundID -> 是否成功出块

	// ================================
	// 第3.3章：连任惩罚相关
	// ================================

	// 连任次数 r_t
	// 用于计算所需委托权重：G_j(r_t) = G_j * (2-E_j)^r_t
	ConsecutiveTerms int

	LastElectedRound int // 上次当选的轮次

	RequiredWeight float64 // 连任所需的委托权重

	// 委托权重（仅代理节点）
	// 来自投票节点的权重总和
	DelegatedWeight float64

	// ================================
	// 第4章：动态投票机制相关 网络参数
	// ================================

	// 网络延迟（毫秒）
	// 影响正常投票区段计算：T_normal = T_base * (1 + ΣD_i/(n*D_s)) * ln(1+N_vote/N_all)
	NetworkDelay float64

	// ================================
	// 第5章：奖励分配相关 经济参数
	// ================================

	// 保证金
	// 计算：Deposit = CurrentWeight * DepositRatio
	// 投票时锁定，结果确认后释放或扣除
	Deposit float64

	// 累计奖励
	TotalReward float64

	// 累计补偿
	TotalCompensation float64

	// 区块奖励相关
	BlockRewards map[int]float64 // roundID -> 该轮获得的奖励

	// ================================
	// 历史记录
	// ================================

	// 投票历史记录
	VoteHistory []VoteRecord

	// 出块历史记录
	BlockHistory []BlockRecord

	// 性能快照历史
	PerformanceHistory []PerformanceSnapshot
}

// NewNode 创建新节点
// 参数：
//   - id: 节点唯一标识
//   - initialWeight: 初始质押权重 W_i(0)
//   - performanceScore: 初始表现评分 R_i（建议0.5）
//   - networkDelay: 网络延迟（毫秒）
func NewNode(id string, initialWeight, performanceScore, networkDelay float64) *Node {
	// 计算初始保证金（第5.4节）
	deposit := initialWeight * 1.0 // DepositRatio从config包引入

	return &Node{
		// 基本信息
		ID:             id,
		Type:           VoterNode,
		IsActive:       true,
		LastUpdateTime: time.Now(),

		// 权重信息
		InitialWeight: initialWeight,
		CurrentWeight: initialWeight,

		// 表现评分
		PerformanceScore: performanceScore,
		VotingSpeed:      0.5, // 初始中等水平
		VotingBehavior:   0.5, // 初始中等水平

		// 网络参数
		NetworkDelay: networkDelay,

		// 经济参数
		Deposit: deposit,

		// 初始化历史记录
		VoteHistory:        make([]VoteRecord, 0),
		BlockHistory:       make([]BlockRecord, 0),
		PerformanceHistory: make([]PerformanceSnapshot, 0),
	}
}

// Clone 深拷贝节点
// 用于模拟和回滚场景
func (n *Node) Clone() *Node {
	clone := *n

	// 深拷贝切片
	clone.VoteHistory = make([]VoteRecord, len(n.VoteHistory))
	copy(clone.VoteHistory, n.VoteHistory)

	clone.BlockHistory = make([]BlockRecord, len(n.BlockHistory))
	copy(clone.BlockHistory, n.BlockHistory)

	clone.PerformanceHistory = make([]PerformanceSnapshot, len(n.PerformanceHistory))
	copy(clone.PerformanceHistory, n.PerformanceHistory)

	return &clone
}

// RecordVote 记录投票
// 参数：
//   - votedFor: 投票对象ID
//   - weight: 投票权重
//   - timeCost: 投票耗时（毫秒）
//   - compensation: 获得的补偿
//   - success: 是否成功
//   - isProxy: 是否为代投
//   - proxyFor: 代投对象ID
func (n *Node) RecordVote(
	votedFor string,
	weight float64,
	timeCost float64,
	compensation float64,
	success bool,
	isProxy bool,
	proxyFor string,
) {
	record := VoteRecord{
		Timestamp:    time.Now(),
		VotedFor:     votedFor,
		Weight:       weight,
		TimeCost:     timeCost,
		Success:      success,
		IsProxy:      isProxy,
		ProxyFor:     proxyFor,
		Compensation: compensation,
	}

	n.VoteHistory = append(n.VoteHistory, record)

	if !isProxy {
		// 更新统计
		if success {
			n.SuccessfulVotes++
		} else {
			n.FailedVotes++
		}
	}
}

// GetDecayRate 获取衰减率
func (n *Node) GetDecayRate() float64 {
	return 0.2 * (1.0 - n.PerformanceScore)
}

// GetHalfLife 计算半衰期
func (n *Node) GetHalfLife() float64 {
	decayRate := n.GetDecayRate()
	if decayRate <= 0 {
		return 1e9
	}
	return 0.693 / decayRate
}

// HasProducedBlockInRound 检查节点在指定轮次是否成功出块
func (n *Node) HasProducedBlockInRound(roundID int) bool {
	n.mu.RLock()
	defer n.mu.RUnlock()

	if n.BlockProductionStatus == nil {
		return false
	}

	success, exists := n.BlockProductionStatus[roundID]
	return exists && success
}

// RecordBlockProductionStatus 记录出块状态
func (n *Node) RecordBlockProductionStatus(roundID int, success bool) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.BlockProductionStatus == nil {
		n.BlockProductionStatus = make(map[int]bool)
	}

	n.BlockProductionStatus[roundID] = success
}

// IncrementConsecutiveTerms 增加连任次数
func (n *Node) IncrementConsecutiveTerms(currentRound int) {
	if n.LastElectedRound != 0 && n.LastElectedRound == currentRound-1 {
		n.ConsecutiveTerms++
	} else {
		n.ConsecutiveTerms = 1
	}
	n.LastElectedRound = currentRound
}

// ResetConsecutiveTerms 重置连任次数(未当选时调用)
func (n *Node) ResetConsecutiveTerms() {
	n.ConsecutiveTerms = 0
}

// RecordBlockReward 记录区块奖励
func (n *Node) RecordBlockReward(roundID int, reward float64) {
	if n.BlockRewards == nil {
		n.BlockRewards = make(map[int]float64)
	}
	n.BlockRewards[roundID] = reward
	n.TotalReward += reward
}
