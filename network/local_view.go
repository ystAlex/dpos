package network

import (
	"nm-dpos/config"
	"nm-dpos/core"
	"nm-dpos/types"
	"nm-dpos/utils"
	"sort"
)

// local_view.go
// 节点的本地网络视图（完整版）

// LocalNetworkView 本地网络视图
type LocalNetworkView struct {
	// ================================
	// 核心数据结构（保留所有必要字段）
	// ================================

	// 本地节点
	LocalNode *types.Node

	// 网络状态（完整的types.Network）
	Network *types.Network

	// 已知的对等节点
	KnownPeers map[string]*types.Node

	// 当前代理节点列表
	CurrentDelegates []*types.Node

	// 投票会话
	VotingSession *core.VotingSession

	// ================================
	// 通信和数据存储
	// ================================

	// 交易池
	TxPool *TransactionPool

	// 同步管理器
	SyncManager *SyncManager

	// ================================
	// 统计信息
	// ================================

	Logger            *utils.Logger
	CurrentRound      int
	TotalWeight       float64
	EnvironmentWeight float64
	SystemDelay       float64
	ActivePeerCount   int
}

// NewLocalNetworkView 创建本地网络视图
func NewLocalNetworkView(localNode *types.Node, logger *utils.Logger) *LocalNetworkView {
	blockPool := NewBlockPool()
	txPool := NewTransactionPool()

	// 创建完整的Network结构
	network := types.NewNetwork()

	view := &LocalNetworkView{
		LocalNode:   localNode,
		Network:     network,
		KnownPeers:  make(map[string]*types.Node),
		TxPool:      txPool,
		Logger:      logger,
		SystemDelay: 20.0,
	}

	// 初始化同步管理器（使用MessageBus）
	view.SyncManager = NewSyncManager(localNode, blockPool, logger)

	// 将本地节点添加到Network
	view.Network.Nodes = append(view.Network.Nodes, localNode)

	return view
}

// ================================
// 节点管理
// ================================

// UpdatePeerInfo 更新对等节点信息
func (lnv *LocalNetworkView) UpdatePeerInfo(nodeID string, node *types.Node) {
	lnv.KnownPeers[nodeID] = node

	// 同步更新到Network.Nodes
	found := false
	for i, n := range lnv.Network.Nodes {
		if n.ID == nodeID {
			lnv.Network.Nodes[i] = node
			found = true
			break
		}
	}

	if !found {
		lnv.Network.Nodes = append(lnv.Network.Nodes, node)
	}

	lnv.recalculateNetworkStats()
}

// RemovePeer 移除对等节点
func (lnv *LocalNetworkView) RemovePeer(nodeID string) {
	delete(lnv.KnownPeers, nodeID)

	// 从Network.Nodes中移除
	for i, node := range lnv.Network.Nodes {
		if node.ID == nodeID {
			lnv.Network.Nodes = append(lnv.Network.Nodes[:i], lnv.Network.Nodes[i+1:]...)
			break
		}
	}

	lnv.recalculateNetworkStats()
}

// GetAllKnownNodes 获取所有已知节点（包括自己）
func (lnv *LocalNetworkView) GetAllKnownNodes() []*types.Node {
	return lnv.Network.Nodes
}

// GetActiveNodes 获取所有活跃节点
func (lnv *LocalNetworkView) GetActiveNodes() []*types.Node {
	nodes := make([]*types.Node, 0)
	for _, node := range lnv.Network.Nodes {
		if node.IsActive {
			nodes = append(nodes, node)
		}
	}
	return nodes
}

// GetVoterNodes 获取所有投票节点
func (lnv *LocalNetworkView) GetVoterNodes() []*types.Node {
	voters := make([]*types.Node, 0)
	for _, node := range lnv.Network.Nodes {
		if node.IsActive && node.Type == types.VoterNode {
			voters = append(voters, node)
		}
	}
	return voters
}

// ================================
// 网络统计（完整实现）
// ================================

// recalculateNetworkStats 重新计算网络统计
func (lnv *LocalNetworkView) recalculateNetworkStats() {
	totalWeight := 0.0
	weights := make([]float64, 0)
	totalDelay := 0.0
	activeCount := 0

	for _, node := range lnv.Network.Nodes {
		if node.IsActive {
			totalWeight += node.CurrentWeight
			weights = append(weights, node.CurrentWeight)
			totalDelay += node.NetworkDelay
			activeCount++
		}
	}

	lnv.TotalWeight = totalWeight
	lnv.EnvironmentWeight = utils.MinMaxExcluded(weights)
	lnv.ActivePeerCount = activeCount

	if activeCount > 0 {
		lnv.SystemDelay = totalDelay / float64(activeCount)
	}

	// 同步更新到Network字段
	lnv.Network.TotalWeight = totalWeight
	lnv.Network.EnvironmentWeight = lnv.EnvironmentWeight
	lnv.Network.ActiveNodesCount = activeCount
	lnv.Network.SystemNetworkDelay = lnv.SystemDelay

	if activeCount > 0 {
		lnv.Network.AverageWeight = totalWeight / float64(activeCount)
	}
}

// ================================
// 代理节点选举（使用core/算法）
// ================================

// SelectDelegatesLocally 基于本地视图选举代理节点
func (lnv *LocalNetworkView) SelectDelegatesLocally() []*types.Node {
	candidates := lnv.GetActiveNodes()

	// 按权重排序
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].CurrentWeight > candidates[j].CurrentWeight
	})

	numDelegates := config.NumDelegates
	if len(candidates) < numDelegates {
		numDelegates = len(candidates)
	}

	delegates := make([]*types.Node, numDelegates)
	for i := 0; i < numDelegates; i++ {
		delegates[i] = candidates[i]
		delegates[i].Type = types.DelegateNode
		delegates[i].DelegatedWeight = delegates[i].CurrentWeight
	}

	lnv.CurrentDelegates = delegates
	lnv.Network.Delegates = delegates

	return delegates
}

// ================================
// 投票机制（使用core/voting.go完整逻辑）
// ================================

// ConductLocalVoting 执行本地投票逻辑（包含代理投票）
func (lnv *LocalNetworkView) ConductLocalVoting() *types.VotingStatistics {
	// 创建投票会话
	voters := lnv.GetVoterNodes()
	candidates := lnv.CurrentDelegates

	// 动态计算投票区段
	normalPeriod := core.CalculateNormalVotingPeriod(
		lnv.GetAllKnownNodes(),
		lnv.SystemDelay,
		len(voters),
	)

	session := core.NewVotingSession(
		lnv.CurrentRound,
		normalPeriod,
		voters,
		candidates,
	)

	// 执行投票（使用core/voting.go的完整逻辑，包含代理投票）
	stats := core.ConductVoting(session)

	// 分配热补偿
	successfulVoters := append(session.SuccessfulVoters, getProxiedVotersFromSession(session)...)
	hotCompensation := core.DistributeHotCompensation(session, successfulVoters)

	// 更新本地视图
	lnv.VotingSession = session
	lnv.Network.NormalVotingPeriod = normalPeriod
	lnv.Network.ParticipatingVoters = stats.SuccessfulVoters + stats.ProxiedVoters
	lnv.Network.TotalDeductedWeight = session.TotalDeductedWeight
	lnv.Network.HotCompensationPool = hotCompensation

	return stats
}

// getProxiedVotersFromSession 从投票会话中提取被代投的节点
func getProxiedVotersFromSession(session *core.VotingSession) []*types.Node {
	proxied := make([]*types.Node, 0)
	proxiedIDs := make(map[string]bool)

	for voterID := range session.ProxiedVoters {
		proxiedIDs[voterID] = true
	}

	for _, voter := range session.Voters {
		if proxiedIDs[voter.ID] {
			proxied = append(proxied, voter)
		}
	}

	return proxied
}

// GetVotingTarget 根据本地视图选择投票目标
func (lnv *LocalNetworkView) GetVotingTarget() *types.Node {
	if len(lnv.CurrentDelegates) == 0 {
		return nil
	}

	bestScore := -1.0
	var bestDelegate *types.Node

	for _, delegate := range lnv.CurrentDelegates {
		// 综合评分：表现评分60% + 权重占比40%
		score := delegate.PerformanceScore*0.6 +
			(delegate.CurrentWeight/lnv.TotalWeight)*0.4

		if score > bestScore {
			bestScore = score
			bestDelegate = delegate
		}
	}

	return bestDelegate
}

// ================================
// 权重更新（使用core/decay.go）
// ================================

// UpdateLocalWeight 更新本地节点权重
func (lnv *LocalNetworkView) UpdateLocalWeight() {
	core.UpdateNodeWeight(lnv.LocalNode, 1.0, lnv.EnvironmentWeight)
}

// UpdateAllWeights 更新所有节点权重（用于集中式模式）
func (lnv *LocalNetworkView) UpdateAllWeights() {
	allNodes := lnv.GetAllKnownNodes()
	core.UpdateAllNodesWeight(allNodes, 1.0, lnv.EnvironmentWeight)
	lnv.recalculateNetworkStats()
}

// ================================
// 表现评估（使用core/performance.go）
// ================================

// UpdateLocalPerformance 更新本地节点表现评分
func (lnv *LocalNetworkView) UpdateLocalPerformance() {
	core.UpdateOnlineTime(lnv.LocalNode)

	if lnv.LocalNode.Type == types.VoterNode {
		delegatePerf := lnv.GetAverageDelegatePerformance()
		core.UpdateVoterPerformance(lnv.LocalNode, lnv.Network.NormalVotingPeriod, delegatePerf)
	} else {
		core.UpdateDelegatePerformance(lnv.LocalNode)
	}
}

// UpdateAllPerformanceScores 更新所有节点表现评分（集中式模式）
func (lnv *LocalNetworkView) UpdateAllPerformanceScores() {
	delegatePerformance := lnv.GetAverageDelegatePerformance()

	for _, node := range lnv.Network.Nodes {
		if !node.IsActive {
			continue
		}

		core.UpdateOnlineTime(node)

		if node.Type == types.VoterNode {
			core.UpdateVoterPerformance(node, lnv.Network.NormalVotingPeriod, delegatePerformance)
		} else {
			core.UpdateDelegatePerformance(node)
		}
	}
}

// GetAverageDelegatePerformance 计算代理节点平均表现
func (lnv *LocalNetworkView) GetAverageDelegatePerformance() float64 {
	if len(lnv.CurrentDelegates) == 0 {
		return 0.5
	}

	total := 0.0
	for _, delegate := range lnv.CurrentDelegates {
		total += delegate.PerformanceScore
	}

	return total / float64(len(lnv.CurrentDelegates))
}

// ================================
// 出块相关（集中式模式）
// ================================

// ProduceBlock 生产区块
func (lnv *LocalNetworkView) ProduceBlock(delegate *types.Node) *types.BlockRecord {
	lnv.Network.CurrentBlockHeight++

	// 模拟出块成功率
	successProb := delegate.PerformanceScore
	isSuccess := utils.RandomFloat() < successProb
	isDelayed := utils.RandomFloat() < 0.1

	block := &types.BlockRecord{
		Timestamp:   utils.TimeNow(),
		BlockHeight: lnv.Network.CurrentBlockHeight,
		RoundID:     lnv.CurrentRound,
		Success:     isSuccess,
		IsDelayed:   isDelayed,
		IsInvalid:   !isSuccess,
	}

	// 记录出块
	core.RecordBlockProduction(delegate, block.BlockHeight, isSuccess, isDelayed, !isSuccess)

	return block
}

// DistributeBlockReward 分配区块奖励
func (lnv *LocalNetworkView) DistributeBlockReward(producer *types.Node, validators []*types.Node) map[string]float64 {
	transactionFees := 50.0 + utils.RandomFloat()*100.0
	return core.DistributeBlockRewards(producer, validators, transactionFees)
}

// ================================
// 节点状态查询
// ================================

// IsLocalDelegate 判断本节点是否为代理节点
func (lnv *LocalNetworkView) IsLocalDelegate() bool {
	for _, delegate := range lnv.CurrentDelegates {
		if delegate.ID == lnv.LocalNode.ID {
			return true
		}
	}
	return false
}

// GetPeerCount 获取对等节点数量
func (lnv *LocalNetworkView) GetPeerCount() int {
	return len(lnv.KnownPeers)
}

// GetActivePeerCount 获取活跃对等节点数量
func (lnv *LocalNetworkView) GetActivePeerCount() int {
	return lnv.ActivePeerCount
}

// GetNetworkSnapshot 创建网络快照
func (lnv *LocalNetworkView) GetNetworkSnapshot() types.NetworkSnapshot {
	performances := make([]float64, 0)
	for _, node := range lnv.Network.Nodes {
		if node.IsActive {
			performances = append(performances, node.PerformanceScore)
		}
	}

	return types.NetworkSnapshot{
		Timestamp:           utils.TimeNow(),
		RoundID:             lnv.CurrentRound,
		TotalNodes:          len(lnv.Network.Nodes),
		ActiveNodes:         lnv.Network.ActiveNodesCount,
		DelegateNodes:       len(lnv.CurrentDelegates),
		TotalWeight:         lnv.Network.TotalWeight,
		EnvironmentWeight:   lnv.Network.EnvironmentWeight,
		AverageWeight:       lnv.Network.AverageWeight,
		AveragePerformance:  utils.Mean(performances),
		MedianPerformance:   utils.Median(performances),
		TopPerformance:      utils.Max(performances),
		BottomPerformance:   utils.Min(performances),
		SystemNetworkDelay:  lnv.Network.SystemNetworkDelay,
		VotingParticipation: float64(lnv.Network.ParticipatingVoters) / float64(len(lnv.Network.Nodes)),
	}
}

// ================================
// 完整的共识轮次执行（集中式模式）
// ================================

// ExecuteRound 执行完整的共识轮次
func (lnv *LocalNetworkView) ExecuteRound() *types.RoundSummary {
	lnv.CurrentRound++
	lnv.Network.CurrentRound = lnv.CurrentRound

	startTime := utils.TimeNow()

	// 1. 更新权重
	lnv.UpdateAllWeights()

	// 2. 选举代理节点
	delegates := lnv.SelectDelegatesLocally()

	// 3. 投票阶段
	votingStats := lnv.ConductLocalVoting()

	// 4. 出块阶段
	blockStats := lnv.conductBlockProduction()

	// 5. 更新表现评分
	lnv.UpdateAllPerformanceScores()

	// 6. 更新连任次数
	lnv.updateConsecutiveTerms(delegates)

	// 创建轮次摘要
	summary := &types.RoundSummary{
		RoundID:         lnv.CurrentRound,
		StartTime:       startTime,
		EndTime:         utils.TimeNow(),
		DelegateIDs:     extractNodeIDs(delegates),
		VotingStats:     *votingStats,
		BlockStats:      *blockStats,
		NetworkSnapshot: lnv.GetNetworkSnapshot(),
	}

	lnv.Network.SnapshotHistory = append(lnv.Network.SnapshotHistory, summary.NetworkSnapshot)

	return summary
}

// conductBlockProduction 执行出块阶段
func (lnv *LocalNetworkView) conductBlockProduction() *types.BlockStatistics {
	stats := &types.BlockStatistics{}

	for _, delegate := range lnv.CurrentDelegates {
		block := lnv.ProduceBlock(delegate)
		stats.TotalBlocks++

		if block.Success {
			if block.IsDelayed {
				stats.DelayedBlocks++
			} else {
				stats.ValidBlocks++
			}

			// 选择验证者
			validators := lnv.selectValidators(delegate, 3)
			rewards := lnv.DistributeBlockReward(delegate, validators)

			block.Reward = rewards[delegate.ID]
			block.Validators = extractNodeIDs(validators)
			block.RewardDistribution = rewards

			stats.TotalRewardDistributed += block.Reward
		} else {
			stats.InvalidBlocks++
			// 惩罚出块失败
			core.ConfiscateDeposit(delegate, config.BlockFailurePenalty)
		}

		delegate.BlockHistory = append(delegate.BlockHistory, *block)
	}

	if stats.TotalBlocks > 0 {
		stats.SuccessRate = float64(stats.ValidBlocks) / float64(stats.TotalBlocks)
		stats.AverageBlockReward = stats.TotalRewardDistributed / float64(stats.TotalBlocks)
	}

	return stats
}

// selectValidators 选择验证者
func (lnv *LocalNetworkView) selectValidators(producer *types.Node, count int) []*types.Node {
	candidates := make([]*types.Node, 0)
	for _, delegate := range lnv.CurrentDelegates {
		if delegate.ID != producer.ID && delegate.IsActive {
			candidates = append(candidates, delegate)
		}
	}

	utils.Shuffle(candidates)

	if count > len(candidates) {
		count = len(candidates)
	}

	return candidates[:count]
}

// updateConsecutiveTerms 更新连任次数
func (lnv *LocalNetworkView) updateConsecutiveTerms(currentDelegates []*types.Node) {
	currentIDs := make(map[string]bool)
	for _, delegate := range currentDelegates {
		currentIDs[delegate.ID] = true
	}

	for _, node := range lnv.Network.Nodes {
		if currentIDs[node.ID] {
			node.ConsecutiveTerms++
		} else if node.Type == types.DelegateNode {
			node.ConsecutiveTerms = 0
			node.Type = types.VoterNode
		}
	}
}

// extractNodeIDs 提取节点ID列表
func extractNodeIDs(nodes []*types.Node) []string {
	ids := make([]string, len(nodes))
	for i, node := range nodes {
		ids[i] = node.ID
	}
	return ids
}
