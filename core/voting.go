package core

import (
	"fmt"
	"math/rand"
	"nm-dpos/config"
	"nm-dpos/types"
	"nm-dpos/utils"
	"sort"
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
	NormalVotingPhase VotingPhase = iota // 正常投票区段
	ProxyVotingPhase                     // 代投区段
	VotingClosed                         // 投票结束
)

// VotingSession 投票会话
// 管理一轮投票的完整过程
type VotingSession struct {
	RoundID                 int                // 轮次ID
	NormalPeriod            float64            // 正常投票区段（毫秒）
	ProxyPeriod             float64            // 代投区段（毫秒）
	StartTime               time.Time          // 开始时间
	Voters                  []*types.Node      // 投票节点列表
	Candidates              []*types.Node      // 候选节点列表
	SuccessfulVoters        []*types.Node      // 成功投票节点
	ProxiedVoters           map[string]string  // 代投映射：被代投者ID -> 代投者ID
	FailedVoters            []*types.Node      // 失败投票节点
	VoteResults             map[string]float64 // 投票结果：候选节点ID -> 获得权重
	TotalDeductedWeight     float64            // 扣除的总权重
	TotalConfiscatedDeposit float64            // 没收的保证金总额
	HotCompensationPool     float64            // 热补偿资金池
}

// NewVotingSession 创建新的投票会话
func NewVotingSession(roundID int, normalPeriod float64, voters, candidates []*types.Node) *VotingSession {
	return &VotingSession{
		RoundID:                 roundID,
		NormalPeriod:            normalPeriod,
		ProxyPeriod:             config.ProxyPeriodEnd - config.ProxyPeriodStart,
		StartTime:               time.Now(),
		Voters:                  voters,
		Candidates:              candidates,
		SuccessfulVoters:        make([]*types.Node, 0),
		ProxiedVoters:           make(map[string]string),
		FailedVoters:            make([]*types.Node, 0),
		VoteResults:             make(map[string]float64),
		TotalDeductedWeight:     0,
		TotalConfiscatedDeposit: 0,
		HotCompensationPool:     0,
	}
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
// 第4章主流程：投票执行
// ================================

// ConductVoting 执行投票阶段
// 完整流程：正常投票 -> 代投 -> 失败处理
//
// 参数：
//   - session: 投票会话
//
// 返回：投票统计信息
func ConductVoting(session *VotingSession) *types.VotingStatistics {
	// 第一阶段：正常投票
	normalVoters := conductNormalVoting(session)

	// 第二阶段：代投
	proxiedVoters := conductProxyVoting(session, normalVoters)

	// 第三阶段：失败处理
	failedVoters := handleVotingFailures(session)

	// 统计结果
	stats := &types.VotingStatistics{
		TotalVoters:       len(session.Voters),
		SuccessfulVoters:  len(normalVoters),
		ProxiedVoters:     len(proxiedVoters),
		FailedVoters:      len(failedVoters),
		NormalPeriod:      session.NormalPeriod,
		ProxyPeriod:       session.ProxyPeriod,
		ParticipationRate: float64(len(normalVoters)+len(proxiedVoters)) / float64(len(session.Voters)),
	}

	// 计算平均投票时间
	totalTime := 0.0
	for _, voter := range normalVoters {
		if len(voter.VoteHistory) > 0 {
			totalTime += voter.VoteHistory[len(voter.VoteHistory)-1].TimeCost
		}
	}
	if len(normalVoters) > 0 {
		stats.AverageVotingTime = totalTime / float64(len(normalVoters))
	}

	return stats
}

// conductNormalVoting 执行正常投票阶段
// 模拟节点在正常投票区段内的投票行为
func conductNormalVoting(session *VotingSession) []*types.Node {
	successfulVoters := make([]*types.Node, 0)

	for _, voter := range session.Voters {
		if !voter.IsActive || voter.Type != types.VoterNode {
			continue
		}

		// 模拟投票时间：基础网络延迟 + 随机处理时间
		votingTime := voter.NetworkDelay + rand.Float64()*20.0

		// 判断是否在正常投票区段内完成
		if votingTime <= session.NormalPeriod {
			// 随机选择一个候选节点投票
			if len(session.Candidates) > 0 {
				targetIdx := rand.Intn(len(session.Candidates))
				target := session.Candidates[targetIdx]

				// 记录投票
				voter.RecordVote(
					target.ID,
					voter.CurrentWeight,
					votingTime,
					0, // 暂无补偿
					true,
					false,
					"",
				)

				// 更新投票结果
				session.VoteResults[target.ID] += voter.CurrentWeight

				// 添加到成功投票列表
				successfulVoters = append(successfulVoters, voter)
				session.SuccessfulVoters = append(session.SuccessfulVoters, voter)
			}
		}
	}

	return successfulVoters
}

// ================================
// 第4.3节：代投机制
// ================================

// conductProxyVoting 执行代投阶段
// 为未能在正常区段完成投票的节点安排代投
//
// 参数：
//   - session: 投票会话
//   - normalVoters: 已成功投票的节点列表
//
// 返回：被代投的节点列表
func conductProxyVoting(session *VotingSession, normalVoters []*types.Node) []*types.Node {
	// 找出需要代投的节点
	needProxyVoters := findNodesNeedingProxy(session, normalVoters)
	if len(needProxyVoters) == 0 {
		return []*types.Node{}
	}

	// 选择代投节点（按投票速度排序）
	proxyNodes := selectProxyNodes(normalVoters)
	if len(proxyNodes) == 0 {
		return []*types.Node{}
	}

	proxiedVoters := make([]*types.Node, 0)

	// 为每个需要代投的节点分配代投者
	for i, needProxyVoter := range needProxyVoters {
		if i >= len(proxyNodes) {
			// 代投节点不足，剩余节点将投票失败
			break
		}

		proxyNode := proxyNodes[i]

		// 随机选择候选节点
		if len(session.Candidates) > 0 {
			targetIdx := rand.Intn(len(session.Candidates))
			target := session.Candidates[targetIdx]

			// 计算代投补偿（第4.3节公式）
			compensation := calculateProxyCompensation(needProxyVoter, session.ProxyPeriod)

			// 代投者记录（获得补偿）
			proxyNode.RecordVote(
				target.ID,
				needProxyVoter.CurrentWeight,
				0, // 代投耗时记为0
				compensation,
				true,
				true,
				needProxyVoter.ID,
			)
			proxyNode.TotalCompensation += compensation

			// 被代投者记录
			needProxyVoter.RecordVote(
				target.ID,
				needProxyVoter.CurrentWeight,
				session.NormalPeriod, // 视为在临界点投票
				0,
				true,
				false,
				"",
			)

			// 更新投票结果
			session.VoteResults[target.ID] += needProxyVoter.CurrentWeight

			// 记录代投关系
			session.ProxiedVoters[needProxyVoter.ID] = proxyNode.ID

			proxiedVoters = append(proxiedVoters, needProxyVoter)
		}
	}

	return proxiedVoters
}

// findNodesNeedingProxy 找出需要代投的节点
// 条件：活跃 + 未在正常区段投票
func findNodesNeedingProxy(session *VotingSession, normalVoters []*types.Node) []*types.Node {
	// 构建已投票节点ID集合
	votedIDs := make(map[string]bool)
	for _, voter := range normalVoters {
		votedIDs[voter.ID] = true
	}

	needProxy := make([]*types.Node, 0)
	for _, voter := range session.Voters {
		if voter.IsActive && !votedIDs[voter.ID] {
			needProxy = append(needProxy, voter)
		}
	}

	return needProxy
}

// selectProxyNodes 选择代投节点
// 按投票速度排序，选择最快的节点
func selectProxyNodes(normalVoters []*types.Node) []*types.Node {
	// 创建副本并按投票速度排序
	proxyCandidates := make([]*types.Node, len(normalVoters))
	copy(proxyCandidates, normalVoters)

	sort.Slice(proxyCandidates, func(i, j int) bool {
		// 获取最近一次投票的耗时
		iTime := getLastVoteTime(proxyCandidates[i])
		jTime := getLastVoteTime(proxyCandidates[j])
		return iTime < jTime
	})

	return proxyCandidates
}

// getLastVoteTime 获取节点最近一次投票的耗时
func getLastVoteTime(node *types.Node) float64 {
	if len(node.VoteHistory) == 0 {
		return 999999.0 // 无历史记录，排在最后
	}
	lastVote := node.VoteHistory[len(node.VoteHistory)-1]
	return lastVote.TimeCost
}

// calculateProxyCompensation 计算代投补偿
// 公式：S_comp = F_i(t_e) - F_i(t_s)
// 即宕机节点在代投期间衰减的权重
//
// 参数：
//   - proxiedNode: 被代投节点
//   - proxyPeriodDuration: 代投区段时长（毫秒）
//
// 返回：补偿权重
func calculateProxyCompensation(proxiedNode *types.Node, proxyPeriodDuration float64) float64 {
	// 将毫秒转换为周期（假设1个周期=1000毫秒）
	timePeriods := proxyPeriodDuration / 1000.0

	// 计算代投前的权重
	weightBefore := proxiedNode.CurrentWeight

	// 计算代投后的权重（使用衰减公式）
	decayRate := CalculateDecayRate(proxiedNode.PerformanceScore)
	weightAfter := utils.NewtonCooling(
		weightBefore,
		0, // envWeight = 0
		decayRate,
		timePeriods,
	)

	// 补偿 = 衰减掉的权重
	compensation := weightBefore - weightAfter
	if compensation < 0 {
		compensation = 0
	}

	return compensation
}

// ================================
// 第4.4节：投票失败处理
// ================================

// handleVotingFailures 处理投票失败的节点
// 失败定义：超时且无代投节点可用
//
// 参数：
//   - session: 投票会话
//
// 返回：失败节点列表
func handleVotingFailures(session *VotingSession) []*types.Node {
	// 找出所有未成功投票的节点
	votedIDs := make(map[string]bool)
	for _, voter := range session.SuccessfulVoters {
		votedIDs[voter.ID] = true
	}
	for voterID := range session.ProxiedVoters {
		votedIDs[voterID] = true
	}

	failedVoters := make([]*types.Node, 0)

	for _, voter := range session.Voters {
		if !voter.IsActive {
			continue
		}

		if !votedIDs[voter.ID] {
			// 投票失败，执行惩罚
			applyVoteFailurePenalty(voter)
			session.FailedVoters = append(session.FailedVoters, voter)
			failedVoters = append(failedVoters, voter)

			// 累计扣除的权重
			session.TotalDeductedWeight += voter.CurrentWeight * config.VoteFailurePenalty
		}
	}

	return failedVoters
}

// applyVoteFailurePenalty 对投票失败节点应用惩罚
// 第5.1节：四种投票结果 - 情况A（未投票）
//
// 惩罚措施：
//  1. 扣除权重的10%
//  2. 没收保证金
//  3. 记录失败次数
func applyVoteFailurePenalty(node *types.Node) {
	// 1. 扣除权重
	deductedWeight := node.CurrentWeight * config.VoteFailurePenalty
	node.CurrentWeight -= deductedWeight

	// 2. 没收保证金
	node.Deposit = 0

	// 3. 记录失败
	RecordVoteFailure(node)
}

// ================================
// 辅助函数
// ================================

// SimulateVotingTime 模拟投票时间
// 基于节点网络延迟和随机处理时间
func SimulateVotingTime(node *types.Node) float64 {
	// 基础延迟 + 处理时间（20ms内随机）
	return node.NetworkDelay + rand.Float64()*20.0
}

// GetVotingPhase 获取当前投票阶段
func GetVotingPhase(elapsedTime, normalPeriod, proxyEnd float64) VotingPhase {
	if elapsedTime <= normalPeriod {
		return NormalVotingPhase
	} else if elapsedTime <= proxyEnd {
		return ProxyVotingPhase
	}
	return VotingClosed
}

// ValidateVotingSession 验证投票会话的有效性
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
