package core

import (
	"fmt"
	"math/rand"
	"nm-dpos/config"
	"nm-dpos/types"
	"nm-dpos/utils"
	"sort"
	"sync"
	"time"
)

// voting.go
// 第4章：动态投票机制实现
// 包含正常投票、代投、补偿等完整流程

// ================================
// 第4.1节：投票时间窗口设计
// ================================

// VotingPhase 投票阶段枚举
type VotingPhase int

const (
	VotingPrepare  VotingPhase = iota // 准备阶段
	VotingActive                      // 投票进行中
	VotingProxy                       // 代投阶段
	VotingFinished                    // 已完成
)

// VotingSession 投票会话
// 管理一轮投票的完整过程
type VotingSession struct {
	mu             sync.RWMutex
	RoundID        int       // 轮次ID
	NormalPeriod   float64   // 正常投票区段（毫秒）
	ProxyPeriod    float64   // 代投区段（毫秒）
	StartTime      time.Time //正常投票开始时间
	ProxyStartTime time.Time // 代投投票开始时间
	EndTime        time.Time // 整个流程结束时间
	Phase          VotingPhase
	// 参与者
	Voters     []*types.Node // 投票节点列表
	Candidates []*types.Node // 候选节点列表

	// 投票记录 (nodeID -> targetID)
	VoteRecords map[string]string
	// 投票时间记录 (nodeID -> votingTime)
	VoteTimes map[string]float64

	// 分类结果
	SuccessfulVoters []*types.Node     // 成功投票节点
	ProxiedVoters    map[string]string // 代投映射：被代投者ID -> 代投者ID
	FailedVoters     []*types.Node     // 失败投票节点

	// 统计数据
	VoteResults             map[string]float64 // 投票结果：候选节点ID -> 获得权重
	TotalDeductedWeight     float64            // 扣除的总权重
	TotalConfiscatedDeposit float64            // 没收的保证金总额
	HotCompensationPool     float64            // 热补偿资金池
}

// NewVotingSession 创建新的投票会话
func NewVotingSession(roundID int, normalPeriod float64, voters, candidates []*types.Node, startTime time.Time) *VotingSession {
	return &VotingSession{
		RoundID:                 roundID,
		NormalPeriod:            normalPeriod,
		ProxyPeriod:             config.ProxyPeriodEnd - config.ProxyPeriodStart,
		StartTime:               startTime,
		ProxyStartTime:          startTime.Add(time.Duration(normalPeriod) * time.Millisecond),
		EndTime:                 startTime.Add(time.Duration(normalPeriod+config.ProxyPeriodEnd-config.ProxyPeriodStart) * time.Millisecond),
		Voters:                  voters,
		Candidates:              candidates,
		VoteRecords:             make(map[string]string),
		VoteTimes:               make(map[string]float64),
		SuccessfulVoters:        make([]*types.Node, 0),
		ProxiedVoters:           make(map[string]string),
		FailedVoters:            make([]*types.Node, 0),
		VoteResults:             make(map[string]float64),
		TotalDeductedWeight:     0,
		TotalConfiscatedDeposit: 0,
		HotCompensationPool:     0,
	}
}

// RecordVote 记录投票(线程安全)
func (vs *VotingSession) RecordVote(voterID, targetID string, votingTime float64) {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	vs.VoteRecords[voterID] = targetID
	vs.VoteTimes[voterID] = votingTime

	// ✅ 更新目标节点的委托权重
	for _, candidate := range vs.Candidates {
		if candidate.ID == targetID {
			// 找到投票者的权重
			for _, voter := range vs.Voters {
				if voter.ID == voterID {
					vs.VoteResults[targetID] += voter.CurrentWeight
					candidate.DelegatedWeight += voter.CurrentWeight // 累计委托权重
					break
				}
			}
			break
		}
	}
	// 判断是否在正常区段
	if votingTime <= vs.NormalPeriod {
		for _, voter := range vs.Voters {
			if voter.ID == voterID {
				vs.SuccessfulVoters = append(vs.SuccessfulVoters, voter)
				//for i, failed := range vs.FailedVoters {
				//	if failed.ID == voter.ID {
				//		failedVoters := append(vs.FailedVoters[:i], vs.FailedVoters[i+1:]...)
				//		vs.FailedVoters = failedVoters
				//	}
				//}
				break
			}
		}
	}
}

func (vs *VotingSession) RecordProxyVote(voterID, proxyID string) {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	vs.ProxiedVoters[voterID] = proxyID
}

// GetNodesNeedingProxy 获取需要代投的节点
func (vs *VotingSession) GetNodesNeedingProxy() []*types.Node {
	vs.mu.RLock()
	defer vs.mu.RUnlock()

	needProxy := make([]*types.Node, 0)
	for _, voter := range vs.Voters {
		if _, voted := vs.VoteRecords[voter.ID]; !voted {
			// 未投票的节点需要代投
			needProxy = append(needProxy, voter)
		}
	}

	return needProxy
}

// ================================
// 第4.2节：正常投票区段动态调整
// ================================

// CalculateNormalVotingPeriod 计算正常投票区段时间
// 公式：T_normal = T_base * (1 + ΣD_i/(n*D_s)) * ln(1 + N_vote/N_all)
//
// 参数：
//   - nodes: 所有节点
//   - systemDelay: 系统平均延迟
//   - votingNodes: 参与投票节点数
//
// 返回：正常投票区段时间（毫秒）
func CalculateNormalVotingPeriod(nodes []*types.Node, systemDelay float64, votingNodes int) float64 {
	if len(nodes) == 0 {
		return config.InitialNormalPeriod
	}

	// 收集所有节点延迟
	delays := make([]float64, 0, len(nodes))
	for _, node := range nodes {
		if node.IsActive {
			delays = append(delays, node.NetworkDelay)
		}
	}

	return utils.CalculateNormalVotingPeriod(
		config.BaseVotingTime,
		delays,
		systemDelay,
		votingNodes,
		len(nodes),
	)
}

// ================================
// 代投机制
// ================================

// GetProxyNodes 查看代投信息
// delegates 当前的代理节点的集合
func (vs *VotingSession) GetProxyNodes(delegates []*types.Node) []*types.Node {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	vs.Phase = VotingProxy

	proxyNodes := make([]*types.Node, 0)

	// 选择代投节点(按投票速度排序,按照顺序选择即可)
	proxyNodes = selectProxyNodes(vs.SuccessfulVoters, vs.VoteTimes, delegates)

	return proxyNodes
}

// selectProxyNodes 选择代投节点
func selectProxyNodes(voters []*types.Node, voteTimes map[string]float64, currentDelegates []*types.Node) []*types.Node {
	type voterWithTime struct {
		node *types.Node
		time float64
	}

	candidates := make([]voterWithTime, 0)
	for _, voter := range voters {
		if t, ok := voteTimes[voter.ID]; ok {
			candidates = append(candidates, voterWithTime{voter, t})
		}
	}

	// 按投票时间升序排序
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].time < candidates[j].time
	})

	result := make([]*types.Node, 0)
	for _, c := range candidates {
		for _, delegate := range currentDelegates {
			if delegate.ID == c.node.ID {
				result = append(result, c.node)
			}
		}
	}

	return result
}

// CalculateProxyCompensation 计算代投补偿
func CalculateProxyCompensation(proxiedNode *types.Node, proxyPeriodDuration float64) float64 {
	timePeriods := proxyPeriodDuration / 1000.0
	weightBefore := proxiedNode.CurrentWeight

	decayRate := CalculateDecayRate(proxiedNode.PerformanceScore)
	weightAfter := utils.NewtonCooling(weightBefore, 0, decayRate, timePeriods)

	compensation := weightBefore - weightAfter
	if compensation < 0 {
		compensation = 0
	}

	return compensation
}

// ================================
// 失败处理
// ================================

// ProcessFailures 处理投票失败节点
func (vs *VotingSession) ProcessFailures() []*types.Node {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	failedVoters := make([]*types.Node, 0)

	for _, voter := range vs.Voters {
		if !voter.IsActive {
			continue
		}

		// 检查是否已投票或被代投
		_, voted := vs.VoteRecords[voter.ID]
		_, proxied := vs.ProxiedVoters[voter.ID]

		if !voted && !proxied {
			// 完全失败,执行惩罚
			deductedWeight := voter.CurrentWeight * config.VoteFailurePenalty
			voter.CurrentWeight -= deductedWeight

			confiscated := voter.Deposit
			voter.Deposit = 0

			vs.TotalDeductedWeight += deductedWeight
			vs.TotalConfiscatedDeposit += confiscated

			var flag = false
			for _, voter1 := range vs.FailedVoters {
				if voter1.ID == voter.ID {
					flag = true
				}
			}
			if !flag {
				vs.FailedVoters = append(vs.FailedVoters, voter)
			}
			failedVoters = append(failedVoters, voter)
			// 记录失败
			RecordVoteFailure(voter)
		}
	}

	return failedVoters
}

// ================================
// 统计函数
// ================================

// GetStatistics 获取投票统计
func (vs *VotingSession) GetStatistics() *types.VotingStatistics {
	vs.mu.RLock()
	defer vs.mu.RUnlock()

	totalVoted := len(vs.VoteRecords)
	proxiedCount := len(vs.ProxiedVoters)
	normalCount := len(vs.SuccessfulVoters)
	failedCount := len(vs.FailedVoters)

	// 计算平均投票时间
	totalTime := 0.0
	for _, t := range vs.VoteTimes {
		totalTime += t
	}
	avgTime := 0.0
	if len(vs.VoteTimes) > 0 {
		avgTime = totalTime / float64(len(vs.VoteTimes))
	}

	return &types.VotingStatistics{
		TotalVoters:       len(vs.Voters),
		SuccessfulVoters:  normalCount,
		ProxiedVoters:     proxiedCount,
		FailedVoters:      failedCount,
		NormalPeriod:      vs.NormalPeriod,
		ProxyPeriod:       vs.ProxyPeriod,
		ParticipationRate: float64(totalVoted) / float64(len(vs.Voters)),
		AverageVotingTime: avgTime,
	}
}

// ValidateVotingSession 验证会话有效性
func ValidateVotingSession(session *VotingSession) error {
	if len(session.Voters) == 0 {
		return fmt.Errorf("no voters in session")
	}
	if len(session.Candidates) == 0 {
		return fmt.Errorf("no candidates in session")
	}
	if session.NormalPeriod <= 0 {
		return fmt.Errorf("invalid normal period: %f", session.NormalPeriod)
	}
	return nil
}

// SimulateVotingTime 模拟投票时间
func SimulateVotingTime(node *types.Node) float64 {
	return node.NetworkDelay + rand.Float64()*20.0
}

// IsExpired 检查投票会话是否已过期
// 返回 true 表示会话已结束，不应再处理投票
func (vs *VotingSession) IsExpired() bool {
	vs.mu.RLock()
	defer vs.mu.RUnlock()

	now := time.Now().UTC()
	return now.After(vs.EndTime)
}
