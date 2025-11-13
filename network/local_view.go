package network

import (
	"github.com/google/uuid"
	"math/rand"
	"nm-dpos/core"
	"nm-dpos/types"
	"nm-dpos/utils"
	"sync"
	"time"
)

// local_view.go
// 节点的本地网络视图（完整版）

// LocalNetworkView 本地网络视图
type LocalNetworkView struct {
	mu sync.RWMutex

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

	//TODO: 为了方便测试先生成100000条交易到池子里面去。
	for i := 0; i < 1000000; i++ {
		tx := Transaction{
			ID:        uuid.New().String(),
			From:      "",
			To:        "",
			Amount:    10,
			Fee:       2,
			Timestamp: time.Now(),
			Signature: nil,
			Nonce:     int64(i),
		}
		txPool.AddTransaction(&tx)
	}

	// 创建完整的Network结构
	network := types.NewNetwork()

	view := &LocalNetworkView{
		LocalNode:        localNode,
		Network:          network,
		KnownPeers:       make(map[string]*types.Node),
		CurrentDelegates: make([]*types.Node, 0),
		TxPool:           txPool,
		Logger:           logger,
		SystemDelay:      20.0,
	}

	// 初始化同步管理器
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
	lnv.mu.Lock()
	lnv.KnownPeers[nodeID] = node
	lnv.mu.Unlock()

	if !lnv.Network.UpdateNode(nodeID, node) {
		lnv.Network.AddNode(node)
	}

	lnv.recalculateNetworkStats()
}

// RemovePeer 移除对等节点
func (lnv *LocalNetworkView) RemovePeer(nodeID string) {
	lnv.mu.Lock()
	delete(lnv.KnownPeers, nodeID)
	lnv.mu.Unlock()

	// 从Network.Nodes中移除
	lnv.Network.RemoveNode(nodeID)

	lnv.recalculateNetworkStats()
}

// GetAllKnownNodes 获取所有已知节点（包括自己）
func (lnv *LocalNetworkView) GetAllKnownNodes() []*types.Node {
	return lnv.Network.GetNodes()
}

// GetActiveNodes 获取所有活跃节点
func (lnv *LocalNetworkView) GetActiveNodes() []*types.Node {
	nodes := lnv.Network.GetNodes()
	activeNodes := make([]*types.Node, 0)

	for _, node := range nodes {
		if node.IsActive {
			activeNodes = append(activeNodes, node)
		}
	}

	return activeNodes
}

// GetVoterNodes 获取所有投票节点
func (lnv *LocalNetworkView) GetVoterNodes() []*types.Node {
	nodes := lnv.Network.GetNodes()
	voters := make([]*types.Node, 0)

	for _, node := range nodes {
		if node.IsActive {
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
	nodes := lnv.Network.GetNodes()

	totalWeight := 0.0
	weights := make([]float64, 0)
	totalDelay := 0.0
	activeCount := 0

	for _, node := range nodes {
		if node.IsActive {
			totalWeight += node.CurrentWeight
			weights = append(weights, node.CurrentWeight)
			totalDelay += node.NetworkDelay
			activeCount++
		}
	}

	envWeight := utils.MinMaxExcluded(weights)
	avgDelay := 0.0
	if activeCount > 0 {
		avgDelay = totalDelay / float64(activeCount)
	}

	// 使用线程安全的更新方法
	lnv.Network.UpdateStats(types.NetworkStats{
		TotalWeight:        totalWeight,
		EnvironmentWeight:  envWeight,
		AverageWeight:      totalWeight / float64(activeCount),
		ActiveNodesCount:   activeCount,
		SystemNetworkDelay: avgDelay,
	})

	// 更新本地字段
	lnv.mu.Lock()
	lnv.TotalWeight = totalWeight
	lnv.EnvironmentWeight = envWeight
	lnv.ActivePeerCount = activeCount
	lnv.SystemDelay = avgDelay
	lnv.mu.Unlock()
}

// GetVotingTarget 根据本地视图选择投票目标
// ================================
// 投票目标选择
// ================================
func (lnv *LocalNetworkView) GetVotingTarget() *types.Node {
	if len(lnv.GetActiveNodes()) == 0 {
		return nil
	}

	// 可以投票给任何代理节点(包括自己)
	//bestScore := -1.0
	var bestDelegate *types.Node

	rand.Seed(time.Now().UnixNano())

	//随机选择
	bestDelegate = lnv.GetActiveNodes()[rand.Intn(len(lnv.GetActiveNodes()))]

	//for _, delegate := range lnv.CurrentDelegates {
	//	// 综合评分：表现评分60% + 权重占比40%
	//	score := delegate.PerformanceScore*0.6 +
	//		(delegate.CurrentWeight/lnv.TotalWeight)*0.4
	//
	//	if score > bestScore {
	//		bestScore = score
	//		bestDelegate = delegate
	//	}
	//}

	return bestDelegate
}

// ================================
// 权重更新（使用core/decay.go）
// ================================

// UpdateLocalWeight 更新本地节点权重
func (lnv *LocalNetworkView) UpdateLocalWeight() {
	core.UpdateNodeWeight(lnv.LocalNode, 1.0, lnv.EnvironmentWeight)
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
// 节点状态查询
// ================================

// IsLocalDelegate 判断本节点是否为代理节点
func (lnv *LocalNetworkView) IsLocalDelegate() bool {
	for _, delegate := range lnv.CurrentDelegates {
		//fmt.Println("222222222222当前的代理节点是", delegate.ID)
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
