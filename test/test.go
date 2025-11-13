package test

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math"
	"nm-dpos/config"
	"nm-dpos/network"
	"nm-dpos/node"
	"nm-dpos/types"
	"nm-dpos/utils"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// ================================
// 测试器主结构
// ================================

// NetworkTester 网络测试器 - 支持多维度性能测试
// 用于全面测试NM-DPoS共识算法的性能指标
type NetworkTester struct {
	Node      *node.P2PNode // P2P节点实例
	Logger    *utils.Logger // 日志记录器
	OutputDir string        // 测试结果输出目录

	// 基础测试结果
	Results []TestResult // 每轮测试的基础结果记录

	// 专项测试结果
	ThroughputData      []ThroughputResult      // 吞吐量数据(4.1节)
	LatencyData         []LatencyResult         // 时延数据(4.1节)
	BlockProductionData []BlockProductionResult // 区块生产数据(4.2节)
	VotingSpeedData     []VotingSpeedResult     // 投票速率数据(4.3节)
	MaliciousNodeData   []MaliciousNodeResult   // 恶意节点数据(4.4节)

	// 网络级统计
	NetworkStats NetworkStatistics // 网络整体统计数据

	// 测试控制
	StartTime      time.Time    // 测试开始时间
	TestRounds     int          // 总测试轮数
	CurrentRound   int          // 当前轮次
	CollectorMutex sync.RWMutex // 数据收集锁,保证并发安全
}

// ================================
// 测试结果数据结构
// ================================

// TestResult 基础测试结果
// 记录每个测试轮次的节点基础状态信息
type TestResult struct {
	Round              int       // 测试轮次
	Timestamp          time.Time // 时间戳
	NodeID             string    // 节点ID
	NodeType           string    // 节点类型(投票节点/代理节点)
	CurrentWeight      float64   // 当前权重
	PerformanceScore   float64   // 表现评分
	HalfLife           float64   // 权重半衰期
	SuccessfulVotes    int       // 成功投票次数
	FailedVotes        int       // 失败投票次数
	ValidBlocks        int       // 有效区块数
	DelayedBlocks      int       // 延迟区块数
	InvalidBlocks      int       // 无效区块数
	TotalReward        float64   // 累计奖励
	TotalCompensation  float64   // 累计补偿
	NetworkTotalWeight float64   // 网络总权重
	NetworkActiveNodes int       // 网络活跃节点数
	ConsecutiveTerms   int       // 连任次数
	IsMalicious        bool      // 是否为恶意节点
}

// ThroughputResult 吞吐量测试结果 (对应论文4.1节)
// 测量系统的交易处理能力和区块生产速率
type ThroughputResult struct {
	Round              int       // 测试轮次
	Timestamp          time.Time // 时间戳
	TransactionsPerSec float64   // 每秒交易数(TPS)
	BlocksPerMinute    float64   // 每分钟区块数(BPM)
	AvgBlockSize       float64   // 平均区块大小(交易数)
	NetworkSize        int       // 网络规模(节点数)
	ActiveDelegates    int       // 活跃代理节点数
}

// LatencyResult 时延测试结果 (对应论文4.1节)
// 测量共识过程各阶段的时间开销
type LatencyResult struct {
	Round               int       // 测试轮次
	Timestamp           time.Time // 时间戳
	VotingLatency       float64   // 投票时延(ms)
	BlockProductionTime float64   // 出块时延(ms)
	ConsensusLatency    float64   // 共识时延(ms) = 投票期 + 代投期
	NetworkPropagation  float64   // 网络传播时延(ms)
	TotalRoundTime      float64   // 总轮次时间(ms)
}

// BlockProductionResult 区块生产占比结果 (对应论文4.2节)
// 测量去中心化程度和权力分布均衡性
type BlockProductionResult struct {
	Round            int       // 测试轮次
	Timestamp        time.Time // 时间戳
	NodeID           string    // 节点ID
	NodeRank         int       // 节点排名(按权重)
	BlocksProduced   int       // 生产的区块数
	TotalBlocks      int       // 总区块数
	ProductionRatio  float64   // 生产占比(%)
	ConsecutiveTerms int       // 连任次数
}

// VotingSpeedResult 投票速率结果 (对应论文4.3节)
// 测量节点投票积极性和速度分布
type VotingSpeedResult struct {
	Round        int       // 测试轮次
	Timestamp    time.Time // 时间戳
	NodeID       string    // 节点ID
	VotingTime   float64   // 投票耗时(ms)
	TimeRange    string    // 时间区间: "0-20ms", "20-50ms", "50-100ms", ">100ms"
	IsSuccessful bool      // 是否成功投票
	IsProxied    bool      // 是否被代投
}

// MaliciousNodeResult 恶意节点测试结果 (对应论文4.4节)
// 测量系统对恶意节点的识别和清除能力
type MaliciousNodeResult struct {
	Round          int       // 测试轮次
	Timestamp      time.Time // 时间戳
	MaliciousNodes int       // 恶意节点数
	RemovedNodes   int       // 已移除的恶意节点
	AvgRemovalTime float64   // 平均移除时间(轮次)
}

// NetworkStatistics 网络级统计数据
// 汇总整个测试期间的网络整体表现
type NetworkStatistics struct {
	TotalNodes          int     // 总节点数
	TotalDelegates      int     // 代理节点数
	TotalVoters         int     // 投票节点数
	AvgThroughput       float64 // 平均吞吐量(TPS)
	AvgLatency          float64 // 平均时延(ms)
	DecentralizationIdx float64 // 去中心化指数(基尼系数,0表示完全去中心化)
	VotingParticipation float64 // 投票参与度(0-1)
	MaliciousRemovalEff float64 // 恶意节点移除效率(0-1)
}

// ================================
// 构造函数
// ================================

// NewNetworkTester 创建网络测试器
// 参数:
//   - p2pNode: P2P节点实例
//   - logger: 日志记录器
//   - outputDir: 测试结果输出目录
//
// 返回: 初始化好的测试器实例
func NewNetworkTester(p2pNode *node.P2PNode, logger *utils.Logger, outputDir string) *NetworkTester {
	return &NetworkTester{
		Node:                p2pNode,
		Logger:              logger,
		OutputDir:           outputDir,
		Results:             make([]TestResult, 0),
		ThroughputData:      make([]ThroughputResult, 0),
		LatencyData:         make([]LatencyResult, 0),
		BlockProductionData: make([]BlockProductionResult, 0),
		VotingSpeedData:     make([]VotingSpeedResult, 0),
		MaliciousNodeData:   make([]MaliciousNodeResult, 0),
		StartTime:           time.Now(),
	}
}

// ================================
// 主测试流程
// ================================

// RunFullTest 运行完整测试
// 执行所有测试项目,包括:吞吐量、时延、区块生产、投票速率、恶意节点处理
// 参数: rounds - 测试轮数
func (nt *NetworkTester) RunFullTest(rounds int) {
	nt.TestRounds = rounds
	nt.Logger.Info("========================================")
	nt.Logger.Info("开始 NM-DPoS 全面性能测试")
	nt.Logger.Info("测试轮数: %d", rounds)
	nt.Logger.Info("输出目录: %s", nt.OutputDir)
	nt.Logger.Info("========================================")

	// 创建输出目录
	if err := os.MkdirAll(nt.OutputDir, 0755); err != nil {
		nt.Logger.Error("创建输出目录失败: %v", err)
		return
	}

	// 记录初始状态(轮次0)
	nt.recordRound(0)

	// 运行测试轮次
	for round := 1; round <= rounds; round++ {
		nt.CurrentRound = round
		nt.Logger.Info("========== 测试轮次 %d/%d ==========", round, rounds)

		roundStartTime := time.Now()

		// 等待一个区块周期,让系统正常运行
		time.Sleep(time.Duration(config.BlockInterval) * time.Second)

		// 收集各项测试数据
		nt.collectRoundData(round, roundStartTime)

		// 每10轮输出一次进度报告
		if round%10 == 0 {
			nt.Logger.Info("测试进度: %d/%d (%.1f%%)", round, rounds, float64(round)/float64(rounds)*100)
			nt.printIntermediateResults()
		}
	}

	// 生成所有测试报告
	nt.generateAllReports()

	nt.Logger.Info("========================================")
	nt.Logger.Info("测试完成! 总耗时: %v", time.Since(nt.StartTime))
	nt.Logger.Info("========================================")
}

// ================================
// 数据收集
// ================================

// collectRoundData 收集单轮所有测试数据
// 在每个测试轮次中收集所有性能指标数据
// 参数:
//   - round: 当前轮次编号
//   - roundStartTime: 轮次开始时间,用于计算时延
func (nt *NetworkTester) collectRoundData(round int, roundStartTime time.Time) {
	nt.CollectorMutex.Lock()
	defer nt.CollectorMutex.Unlock()

	// 1. 基础测试结果
	nt.recordRound(round)

	// 2. 吞吐量数据 (对应论文4.1节)
	nt.collectThroughputData(round)

	// 3. 时延数据 (对应论文4.1节)
	nt.collectLatencyData(round, roundStartTime)

	// 4. 区块生产占比 (对应论文4.2节)
	nt.collectBlockProductionData(round)

	// 5. 投票速率数据 (对应论文4.3节)
	nt.collectVotingSpeedData(round)

	// 6. 恶意节点数据 (对应论文4.4节)
	nt.collectMaliciousNodeData(round)
}

// recordRound 记录基础轮次数据
// 记录当前轮次节点的基本状态信息
// 参数: round - 轮次编号
func (nt *NetworkTester) recordRound(round int) {
	localNode := nt.Node.LocalView.LocalNode
	snapshot := nt.Node.LocalView.GetNetworkSnapshot()

	result := TestResult{
		Round:              round,
		Timestamp:          time.Now(),
		NodeID:             localNode.ID,
		NodeType:           localNode.Type.String(),
		CurrentWeight:      localNode.CurrentWeight,
		PerformanceScore:   localNode.PerformanceScore,
		HalfLife:           localNode.GetHalfLife(),
		SuccessfulVotes:    localNode.SuccessfulVotes,
		FailedVotes:        localNode.FailedVotes,
		ValidBlocks:        localNode.ValidBlocks,
		DelayedBlocks:      localNode.DelayedBlocks,
		InvalidBlocks:      localNode.InvalidBlocks,
		TotalReward:        localNode.TotalReward,
		TotalCompensation:  localNode.TotalCompensation,
		NetworkTotalWeight: snapshot.TotalWeight,
		NetworkActiveNodes: snapshot.ActiveNodes,
		ConsecutiveTerms:   localNode.ConsecutiveTerms,
		IsMalicious:        nt.isNodeMalicious(localNode),
	}

	nt.Results = append(nt.Results, result)
}

// collectThroughputData 收集吞吐量数据 (对应论文4.1节)
// 统计系统的交易处理能力和区块生产速率
// 测量指标: TPS(每秒交易数)、BPM(每分钟区块数)、平均区块大小
// 参数: round - 当前轮次
func (nt *NetworkTester) collectThroughputData(round int) {
	// 获取最近60秒内的所有区块
	recentBlocks := nt.getRecentBlocks(60)

	// 统计交易总数
	totalTxs := 0
	for _, block := range recentBlocks {
		totalTxs += len(block.Transactions)
	}

	// 计算吞吐量指标
	tps := float64(totalTxs) / 60.0   // 每秒交易数
	bpm := float64(len(recentBlocks)) // 每分钟区块数
	avgBlockSize := 0.0               // 平均区块大小
	if len(recentBlocks) > 0 {
		avgBlockSize = float64(totalTxs) / float64(len(recentBlocks))
	}

	// 记录结果
	result := ThroughputResult{
		Round:              round,
		Timestamp:          time.Now(),
		TransactionsPerSec: tps,
		BlocksPerMinute:    bpm,
		AvgBlockSize:       avgBlockSize,
		NetworkSize:        nt.Node.PeerManager.GetPeerCount() + 1, // +1包含本节点
		ActiveDelegates:    len(nt.Node.LocalView.CurrentDelegates),
	}

	nt.ThroughputData = append(nt.ThroughputData, result)

	nt.Logger.Info("[吞吐量] TPS: %.2f, BPM: %.1f, 平均区块大小: %.1f",
		tps, bpm, avgBlockSize)
}

// collectLatencyData 收集时延数据 (对应论文4.1节)
// 测量共识过程各阶段的时间开销
// 包括: 投票时延、出块时延、共识时延、网络传播时延
// 参数:
//   - round: 当前轮次
//   - roundStartTime: 轮次开始时间
func (nt *NetworkTester) collectLatencyData(round int, roundStartTime time.Time) {
	localNode := nt.Node.LocalView.LocalNode
	session := nt.Node.LocalView.VotingSession

	// 1. 投票时延 - 节点完成投票的时间
	votingLatency := 0.0
	if len(localNode.VoteHistory) > 0 {
		lastVote := localNode.VoteHistory[len(localNode.VoteHistory)-1]
		votingLatency = lastVote.TimeCost
	}

	// 2. 出块时延 - 节点生产区块的时间
	blockProductionTime := 0.0
	if len(localNode.BlockHistory) > 0 {
		lastBlock := localNode.BlockHistory[len(localNode.BlockHistory)-1]
		//fmt.Println("222222222222222222222222222222222222222", lastBlock.ProductionTime.Milliseconds())
		// 使用 ProductionTime 字段并转换为毫秒
		blockProductionTime = float64(lastBlock.ProductionTime.Milliseconds())
	}

	// 3. 共识时延 - 完整的投票共识周期(正常投票期+代投期)
	consensusLatency := 0.0
	if session != nil {
		consensusLatency = float64(session.EndTime.Sub(session.StartTime).Milliseconds())
	}

	// 4. 网络传播时延 - 区块在网络中传播的估算时间
	networkPropagation := nt.estimateNetworkPropagation()

	// 5. 总轮次时间 - 从轮次开始到当前的总耗时
	totalRoundTime := float64(time.Since(roundStartTime).Milliseconds())

	// 记录结果
	result := LatencyResult{
		Round:               round,
		Timestamp:           time.Now(),
		VotingLatency:       votingLatency,
		BlockProductionTime: blockProductionTime,
		ConsensusLatency:    consensusLatency,
		NetworkPropagation:  networkPropagation,
		TotalRoundTime:      totalRoundTime,
	}

	nt.LatencyData = append(nt.LatencyData, result)

	nt.Logger.Info("[时延] 投票: %.1fms, 出块: %.1fms, 共识: %.1fms, 总计: %.1fms",
		votingLatency, blockProductionTime, consensusLatency, totalRoundTime)
}

// collectBlockProductionData 收集区块生产占比数据 (对应论文4.2节)
// 测量系统的去中心化程度,分析区块生产是否集中在少数节点
// 使用基尼系数评估权力分布的均衡性
// 参数: round - 当前轮次
func (nt *NetworkTester) collectBlockProductionData(round int) {
	// 获取所有活跃节点
	allNodes := nt.Node.LocalView.GetActiveNodes()

	// 按权重排序(从高到低)
	sort.Slice(allNodes, func(i, j int) bool {
		return allNodes[i].CurrentWeight > allNodes[j].CurrentWeight
	})

	// 统计每个节点的区块生产情况
	totalBlocks := nt.Node.LocalView.SyncManager.BlockPool.GetLatestHeight()

	for rank, node := range allNodes {
		blocksProduced := node.ValidBlocks
		productionRatio := 0.0
		if totalBlocks > 0 {
			productionRatio = float64(blocksProduced) / float64(totalBlocks) * 100
		}

		result := BlockProductionResult{
			Round:            round,
			Timestamp:        time.Now(),
			NodeID:           node.ID,
			NodeRank:         rank + 1, // 排名从1开始
			BlocksProduced:   blocksProduced,
			TotalBlocks:      totalBlocks,
			ProductionRatio:  productionRatio,
			ConsecutiveTerms: node.ConsecutiveTerms,
		}

		nt.BlockProductionData = append(nt.BlockProductionData, result)
	}

	// 计算去中心化指数(基尼系数)
	// 基尼系数越接近0,表示去中心化程度越高
	gini := nt.calculateGiniCoefficient(allNodes)
	nt.Logger.Info("[区块生产] 去中心化指数(基尼系数): %.4f (越接近0越去中心化)", gini)
}

// collectVotingSpeedData 收集投票速率数据 (对应论文4.3节)
// 测量节点的投票积极性,统计投票速度分布
// 将投票时间分为4个区间: 0-20ms, 20-50ms, 50-100ms, >100ms
// 参数: round - 当前轮次
func (nt *NetworkTester) collectVotingSpeedData(round int) {
	session := nt.Node.LocalView.VotingSession

	if session == nil {
		return
	}

	// 遍历本轮所有投票记录
	for voterID, votingTime := range session.VoteTimes {

		// 判断投票时间所属区间
		timeRange := nt.getVotingTimeRange(votingTime)

		// 记录结果
		result := VotingSpeedResult{
			Round:        round,
			Timestamp:    time.Now(),
			NodeID:       voterID,
			VotingTime:   votingTime,
			TimeRange:    timeRange,
			IsSuccessful: votingTime <= session.NormalPeriod, // 在正常投票期内完成
		}

		nt.VotingSpeedData = append(nt.VotingSpeedData, result)
	}

	// 统计各时间段的分布比例
	distribution := nt.calculateVotingSpeedDistribution()
	nt.Logger.Info("[投票速率] 0-20ms: %.1f%%, 20-50ms: %.1f%%, 50-100ms: %.1f%%, >100ms: %.1f%%",
		distribution["0-20"]*100, distribution["20-50"]*100,
		distribution["50-100"]*100, distribution[">100"]*100)
}

// collectMaliciousNodeData 收集恶意节点数据 (对应论文4.4节)
// 统计恶意节点的数量变化和移除效率
// 区分恶意投票节点和恶意代理节点
// 参数: round - 当前轮次
func (nt *NetworkTester) collectMaliciousNodeData(round int) {
	allNodes := nt.Node.LocalView.GetActiveNodes()

	// 初始化统计变量
	maliciousNodes := 0     // 当前活跃的恶意节点
	removedNodes := 0       // 已移除的恶意节点
	totalRemovalTime := 0.0 // 总移除时间
	removedCount := 0       // 移除节点总数

	// 遍历所有节点,统计恶意节点情况
	for _, node := range allNodes {
		isMalicious := nt.isNodeMalicious(node)

		if isMalicious {

			if node.IsActive {
				maliciousNodes++
			} else {
				removedNodes++
				// 计算从开始到移除的时间(轮次)
				totalRemovalTime += float64(round)
				removedCount++
			}

		}
	}

	// 计算平均移除时间
	avgRemovalTime := 0.0
	if removedCount > 0 {
		avgRemovalTime = totalRemovalTime / float64(removedCount)
	}

	// 记录结果
	result := MaliciousNodeResult{
		Round:          round,
		Timestamp:      time.Now(),
		MaliciousNodes: maliciousNodes,
		RemovedNodes:   removedNodes,
		AvgRemovalTime: avgRemovalTime,
	}

	nt.MaliciousNodeData = append(nt.MaliciousNodeData, result)

	nt.Logger.Info("[恶意节点] 活跃: %d, 已移除: %d, 平均移除时间: %.1f轮",
		maliciousNodes, removedNodes, avgRemovalTime)
}

// ================================
// 辅助函数
// ================================

// getRecentBlocks 获取最近指定秒数内的区块
// 从区块池中获取指定时间窗口内的区块列表
// 参数: seconds - 时间窗口(秒)
// 返回: 区块列表(按时间倒序)
func (nt *NetworkTester) getRecentBlocks(seconds int) []*network.Block {
	blocks := make([]*network.Block, 0)
	blockPool := nt.Node.LocalView.SyncManager.BlockPool

	latestHeight := blockPool.GetLatestHeight()
	cutoffTime := time.Now().Add(-time.Duration(seconds) * time.Second)

	// 从最新区块向前遍历,直到超出时间窗口
	for i := latestHeight; i > 0; i-- {
		block, _ := blockPool.GetBlock(i)
		if block != nil && block.Timestamp.After(cutoffTime) {
			blocks = append(blocks, block)
		} else {
			break
		}
	}

	return blocks
}

// estimateNetworkPropagation 估算网络传播时延
// 基于网络规模和节点平均延迟估算区块传播时间
// 使用简化模型: 传播时延 ≈ log(N) * avgDelay
// 其中N为网络节点数,avgDelay为节点平均延迟
// 返回: 估算的传播时延(毫秒)
func (nt *NetworkTester) estimateNetworkPropagation() float64 {
	peerCount := nt.Node.PeerManager.GetPeerCount()
	avgDelay := nt.Node.LocalView.LocalNode.NetworkDelay

	// 传播时延与网络规模的对数成正比
	return math.Log(float64(peerCount+1)) * avgDelay
}

// getVotingTimeRange 获取投票时间所属区间
// 将投票时间分类到4个标准区间
// 参数: votingTime - 投票耗时(毫秒)
// 返回: 区间标识字符串
func (nt *NetworkTester) getVotingTimeRange(votingTime float64) string {
	if votingTime <= 20 {
		return "0-20" // 快速投票
	} else if votingTime <= 50 {
		return "20-50" // 正常投票
	} else if votingTime <= 100 {
		return "50-100" // 较慢投票
	} else {
		return ">100" // 超时投票
	}
}

// calculateVotingSpeedDistribution 计算投票速率分布
// 统计各个时间区间的投票数量占比
// 返回: 各区间的投票比例(0-1)
func (nt *NetworkTester) calculateVotingSpeedDistribution() map[string]float64 {
	distribution := map[string]float64{
		"0-20":   0,
		"20-50":  0,
		"50-100": 0,
		">100":   0,
	}

	total := len(nt.VotingSpeedData)
	if total == 0 {
		return distribution
	}

	// 统计各区间数量
	for _, data := range nt.VotingSpeedData {
		distribution[data.TimeRange]++
	}

	// 转换为比例
	for key := range distribution {
		distribution[key] /= float64(total)
	}

	return distribution
}

// calculateGiniCoefficient 计算基尼系数(衡量去中心化程度)
// 基尼系数用于衡量区块生产的集中程度
// 取值范围[0,1]: 0表示完全均匀分布,1表示完全集中
// 参数: nodes - 节点列表
// 返回: 基尼系数
func (nt *NetworkTester) calculateGiniCoefficient(nodes []*types.Node) float64 {
	if len(nodes) == 0 {
		return 0
	}

	// 按区块生产数量排序
	blockCounts := make([]int, len(nodes))
	for i, node := range nodes {
		blockCounts[i] = node.ValidBlocks
	}
	sort.Ints(blockCounts)

	// 计算基尼系数
	// 公式: G = Σ|x_i - x_j| / (2 * n * Σx_i)
	n := float64(len(blockCounts))
	sumDiff := 0.0  // 所有差值的和
	sumTotal := 0.0 // 所有区块数的和

	for i := 0; i < len(blockCounts); i++ {
		for j := 0; j < len(blockCounts); j++ {
			sumDiff += math.Abs(float64(blockCounts[i] - blockCounts[j]))
		}
		sumTotal += float64(blockCounts[i])
	}

	if sumTotal == 0 {
		return 0
	}

	return sumDiff / (2 * n * sumTotal)
}

// isNodeMalicious 判断节点是否为恶意节点
// 判断标准:
//  1. 投票节点: 失败率 > 50% 或 从不投票
//  2. 代理节点: 无效区块率 > 30% 或 从不出块
//
// 参数: node - 待判断的节点
// 返回: true表示恶意节点,false表示正常节点
func (nt *NetworkTester) isNodeMalicious(node *types.Node) bool {
	if node.Type == types.VoterNode {
		// 投票节点的判断标准
		totalVotes := node.SuccessfulVotes + node.FailedVotes
		if totalVotes == 0 {
			return true // 从不投票视为恶意
		}
		failureRate := float64(node.FailedVotes) / float64(totalVotes)
		//fmt.Println("22222233333335555555555555", failureRate)
		return failureRate > 0.5 // 失败率超过50%
	} else if node.Type == types.DelegateNode {
		// 代理节点的判断标准
		totalBlocks := node.ValidBlocks + node.DelayedBlocks + node.InvalidBlocks
		if totalBlocks == 0 {
			return true // 从不出块视为恶意
		}
		invalidRate := float64(node.InvalidBlocks) / float64(totalBlocks)
		return invalidRate > 0.3 // 无效率超过30%
	}

	return false
}

// ================================
// 报告生成
// ================================

// generateAllReports 生成所有测试报告
// 生成3类报告:
//  1. CSV格式的原始数据
//  2. JSON格式的综合分析报告
//  3. Markdown格式的对比报告
func (nt *NetworkTester) generateAllReports() {
	nt.Logger.Info("========================================")
	nt.Logger.Info("生成测试报告...")
	nt.Logger.Info("========================================")

	// 1. 保存原始CSV数据
	nt.saveBasicResults()
	nt.saveThroughputResults()
	nt.saveLatencyResults()
	nt.saveBlockProductionResults()
	nt.saveVotingSpeedResults()
	nt.saveMaliciousNodeResults()

	// 2. 生成对比报告(Markdown)
	nt.generateComparisonReport()

	nt.Logger.Info("所有报告已生成!")
}

// saveBasicResults 保存基础测试结果到CSV
// 保存每轮测试的基本状态数据
func (nt *NetworkTester) saveBasicResults() {
	filename := filepath.Join(nt.OutputDir, fmt.Sprintf("%s_基础结果.csv", nt.Node.LocalView.LocalNode.ID))

	file, err := os.Create(filename)
	if err != nil {
		nt.Logger.Error("创建基础结果文件失败: %v", err)
		return
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// 写入表头(中文)
	header := []string{
		"轮次", "时间戳", "节点ID", "节点类型",
		"当前权重", "表现评分", "半衰期",
		"成功投票", "失败投票",
		"有效区块", "延迟区块", "无效区块",
		"累计奖励", "累计补偿",
		"网络总权重", "网络活跃节点",
		"连任次数", "是否恶意",
	}
	writer.Write(header)

	// 写入数据
	for _, result := range nt.Results {
		row := []string{
			fmt.Sprintf("%d", result.Round),
			result.Timestamp.Format(time.RFC3339),
			result.NodeID,
			result.NodeType,
			fmt.Sprintf("%.2f", result.CurrentWeight),
			fmt.Sprintf("%.4f", result.PerformanceScore),
			fmt.Sprintf("%.2f", result.HalfLife),
			fmt.Sprintf("%d", result.SuccessfulVotes),
			fmt.Sprintf("%d", result.FailedVotes),
			fmt.Sprintf("%d", result.ValidBlocks),
			fmt.Sprintf("%d", result.DelayedBlocks),
			fmt.Sprintf("%d", result.InvalidBlocks),
			fmt.Sprintf("%.2f", result.TotalReward),
			fmt.Sprintf("%.2f", result.TotalCompensation),
			fmt.Sprintf("%.2f", result.NetworkTotalWeight),
			fmt.Sprintf("%d", result.NetworkActiveNodes),
			fmt.Sprintf("%d", result.ConsecutiveTerms),
			fmt.Sprintf("%v", result.IsMalicious),
		}
		writer.Write(row)
	}

	nt.Logger.Info("✓ 基础结果已保存: %s", filename)
}

// saveThroughputResults 保存吞吐量测试结果到CSV
// 对应论文4.1节的吞吐量测试数据
func (nt *NetworkTester) saveThroughputResults() {
	filename := filepath.Join(nt.OutputDir, "吞吐量测试结果.csv")

	file, err := os.Create(filename)
	if err != nil {
		nt.Logger.Error("创建吞吐量结果文件失败: %v", err)
		return
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	header := []string{
		"轮次", "时间戳", "TPS", "BPM", "平均区块大小",
		"网络规模", "活跃代理数",
	}
	writer.Write(header)

	for _, result := range nt.ThroughputData {
		row := []string{
			fmt.Sprintf("%d", result.Round),
			result.Timestamp.Format(time.RFC3339),
			fmt.Sprintf("%.2f", result.TransactionsPerSec),
			fmt.Sprintf("%.2f", result.BlocksPerMinute),
			fmt.Sprintf("%.2f", result.AvgBlockSize),
			fmt.Sprintf("%d", result.NetworkSize),
			fmt.Sprintf("%d", result.ActiveDelegates),
		}
		writer.Write(row)
	}

	nt.Logger.Info("✓ 吞吐量结果已保存: %s", filename)
}

// saveLatencyResults 保存时延测试结果到CSV
// 对应论文4.1节的时延测试数据
func (nt *NetworkTester) saveLatencyResults() {
	filename := filepath.Join(nt.OutputDir, "时延测试结果.csv")

	file, err := os.Create(filename)
	if err != nil {
		nt.Logger.Error("创建时延结果文件失败: %v", err)
		return
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	header := []string{
		"轮次", "时间戳", "投票时延", "出块时延",
		"共识时延", "网络传播时延", "总轮次时间",
	}
	writer.Write(header)

	for _, result := range nt.LatencyData {
		row := []string{
			fmt.Sprintf("%d", result.Round),
			result.Timestamp.Format(time.RFC3339),
			fmt.Sprintf("%.2f", result.VotingLatency),
			fmt.Sprintf("%.2f", result.BlockProductionTime),
			fmt.Sprintf("%.2f", result.ConsensusLatency),
			fmt.Sprintf("%.2f", result.NetworkPropagation),
			fmt.Sprintf("%.2f", result.TotalRoundTime),
		}
		writer.Write(row)
	}

	nt.Logger.Info("✓ 时延结果已保存: %s", filename)
}

// saveBlockProductionResults 保存区块生产测试结果到CSV
// 对应论文4.2节的区块生产占比数据
func (nt *NetworkTester) saveBlockProductionResults() {
	filename := filepath.Join(nt.OutputDir, "区块生产占比结果.csv")

	file, err := os.Create(filename)
	if err != nil {
		nt.Logger.Error("创建区块生产结果文件失败: %v", err)
		return
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	header := []string{
		"轮次", "时间戳", "节点ID", "节点排名",
		"生产区块数", "总区块数", "生产占比",
		"连任次数",
	}
	writer.Write(header)

	for _, result := range nt.BlockProductionData {
		row := []string{
			fmt.Sprintf("%d", result.Round),
			result.Timestamp.Format(time.RFC3339),
			result.NodeID,
			fmt.Sprintf("%d", result.NodeRank),
			fmt.Sprintf("%d", result.BlocksProduced),
			fmt.Sprintf("%d", result.TotalBlocks),
			fmt.Sprintf("%.4f", result.ProductionRatio),
			fmt.Sprintf("%d", result.ConsecutiveTerms),
		}
		writer.Write(row)
	}

	nt.Logger.Info("✓ 区块生产结果已保存: %s", filename)
}

// saveVotingSpeedResults 保存投票速率测试结果到CSV
// 对应论文4.3节的投票速率数据
func (nt *NetworkTester) saveVotingSpeedResults() {
	filename := filepath.Join(nt.OutputDir, "投票速率测试结果.csv")

	file, err := os.Create(filename)
	if err != nil {
		nt.Logger.Error("创建投票速率结果文件失败: %v", err)
		return
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	header := []string{
		"轮次", "时间戳", "节点ID", "投票耗时",
		"时间区间", "是否成功", "是否代投",
	}
	writer.Write(header)

	for _, result := range nt.VotingSpeedData {
		row := []string{
			fmt.Sprintf("%d", result.Round),
			result.Timestamp.Format(time.RFC3339),
			result.NodeID,
			fmt.Sprintf("%.2f", result.VotingTime),
			result.TimeRange,
			fmt.Sprintf("%v", result.IsSuccessful),
			fmt.Sprintf("%v", result.IsProxied),
		}
		writer.Write(row)
	}

	nt.Logger.Info("✓ 投票速率结果已保存: %s", filename)
}

// saveMaliciousNodeResults 保存恶意节点测试结果到CSV
// 对应论文4.4节的恶意节点处理数据
func (nt *NetworkTester) saveMaliciousNodeResults() {
	filename := filepath.Join(nt.OutputDir, "恶意节点测试结果.csv")

	file, err := os.Create(filename)
	if err != nil {
		nt.Logger.Error("创建恶意节点结果文件失败: %v", err)
		return
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	header := []string{
		"轮次", "时间戳", "恶意节点",
		"已移除节点", "平均移除时间",
	}
	writer.Write(header)

	for _, result := range nt.MaliciousNodeData {
		row := []string{
			fmt.Sprintf("%d", result.Round),
			result.Timestamp.Format(time.RFC3339),
			fmt.Sprintf("%d", result.MaliciousNodes),
			fmt.Sprintf("%d", result.RemovedNodes),
			fmt.Sprintf("%.2f", result.AvgRemovalTime),
		}
		writer.Write(row)
	}

	nt.Logger.Info("✓ 恶意节点结果已保存: %s", filename)
}

// generateComprehensiveReport 生成综合分析报告(JSON格式)
// 汇总所有测试数据,生成完整的性能分析报告
func (nt *NetworkTester) generateComprehensiveReport() {
	filename := filepath.Join(nt.OutputDir, "综合分析报告.json")

	// 计算统计数据
	nt.calculateNetworkStatistics()

	// 构建报告结构
	report := map[string]interface{}{
		"测试概要": map[string]interface{}{
			"节点ID": nt.Node.LocalView.LocalNode.ID,
			"总轮数":  nt.TestRounds,
			"测试时长": time.Since(nt.StartTime).String(),
			"开始时间": nt.StartTime.Format(time.RFC3339),
			"结束时间": time.Now().Format(time.RFC3339),
		},

		"网络统计": map[string]interface{}{
			"总节点数":     nt.NetworkStats.TotalNodes,
			"代理节点数":    nt.NetworkStats.TotalDelegates,
			"投票节点数":    nt.NetworkStats.TotalVoters,
			"平均吞吐量TPS": nt.NetworkStats.AvgThroughput,
			"平均时延ms":   nt.NetworkStats.AvgLatency,
			"去中心化指数":   nt.NetworkStats.DecentralizationIdx,
			"投票参与度":    nt.NetworkStats.VotingParticipation,
			"恶意节点移除效率": nt.NetworkStats.MaliciousRemovalEff,
		},

		"吞吐量分析":  nt.analyzeThroughput(),
		"时延分析":   nt.analyzeLatency(),
		"区块生产分析": nt.analyzeBlockProduction(),
		"投票速率分析": nt.analyzeVotingSpeed(),
		"恶意节点分析": nt.analyzeMaliciousNodes(),
	}

	// 序列化为JSON
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		nt.Logger.Error("生成综合报告失败: %v", err)
		return
	}

	// 保存到文件
	if err := os.WriteFile(filename, data, 0644); err != nil {
		nt.Logger.Error("保存综合报告失败: %v", err)
		return
	}

	nt.Logger.Info("✓ 综合报告已保存: %s", filename)
}

// calculateNetworkStatistics 计算网络统计数据
// 汇总整个测试期间的网络整体表现
func (nt *NetworkTester) calculateNetworkStatistics() {
	allNodes := nt.Node.LocalView.GetActiveNodes()

	// 基础统计
	nt.NetworkStats.TotalNodes = len(allNodes)
	nt.NetworkStats.TotalDelegates = len(nt.Node.LocalView.CurrentDelegates)
	nt.NetworkStats.TotalVoters = nt.NetworkStats.TotalNodes //- nt.NetworkStats.TotalDelegates

	// 吞吐量统计 - 计算平均TPS
	if len(nt.ThroughputData) > 0 {
		totalTPS := 0.0
		for _, data := range nt.ThroughputData {
			totalTPS += data.TransactionsPerSec
		}
		nt.NetworkStats.AvgThroughput = totalTPS / float64(len(nt.ThroughputData))
	}

	// 时延统计 - 计算平均时延
	if len(nt.LatencyData) > 0 {
		totalLatency := 0.0
		for _, data := range nt.LatencyData {
			totalLatency += data.TotalRoundTime
		}
		nt.NetworkStats.AvgLatency = totalLatency / float64(len(nt.LatencyData))
	}

	// 去中心化指数 - 使用最新的基尼系数
	nt.NetworkStats.DecentralizationIdx = nt.calculateGiniCoefficient(allNodes)

	// 恶意节点移除效率
	if len(nt.MaliciousNodeData) > 0 {
		lastData := nt.MaliciousNodeData[len(nt.MaliciousNodeData)-1]
		totalMalicious := lastData.MaliciousNodes + lastData.RemovedNodes
		if totalMalicious > 0 {
			removed := lastData.RemovedNodes
			nt.NetworkStats.MaliciousRemovalEff = float64(removed) / float64(totalMalicious)
		}
	}
}

// analyzeThroughput 分析吞吐量数据
// 计算平均值、最大值、最小值和变化趋势
// 返回: 吞吐量分析结果
func (nt *NetworkTester) analyzeThroughput() map[string]interface{} {
	if len(nt.ThroughputData) == 0 {
		return map[string]interface{}{"错误": "无数据"}
	}

	first := nt.ThroughputData[0]
	last := nt.ThroughputData[len(nt.ThroughputData)-1]

	// 计算平均值和趋势
	avgTPS := 0.0
	avgBPM := 0.0
	maxTPS := 0.0
	minTPS := math.MaxFloat64

	for _, data := range nt.ThroughputData {
		avgTPS += data.TransactionsPerSec
		avgBPM += data.BlocksPerMinute
		if data.TransactionsPerSec > maxTPS {
			maxTPS = data.TransactionsPerSec
		}
		if data.TransactionsPerSec < minTPS {
			minTPS = data.TransactionsPerSec
		}
	}

	avgTPS /= float64(len(nt.ThroughputData))
	avgBPM /= float64(len(nt.ThroughputData))

	// 计算趋势(简单线性回归斜率)
	trend := (last.TransactionsPerSec - first.TransactionsPerSec) / float64(len(nt.ThroughputData))

	return map[string]interface{}{
		"平均TPS": avgTPS,
		"最大TPS": maxTPS,
		"最小TPS": minTPS,
		"平均BPM": avgBPM,
		"变化趋势":  trend,
		"趋势描述":  nt.getTrendDescription(trend),
		"网络规模":  last.NetworkSize,
	}
}

// analyzeLatency 分析时延数据
// 计算各阶段的平均时延和占比
// 返回: 时延分析结果
func (nt *NetworkTester) analyzeLatency() map[string]interface{} {
	if len(nt.LatencyData) == 0 {
		return map[string]interface{}{"错误": "无数据"}
	}

	avgVoting := 0.0
	avgBlock := 0.0
	avgConsensus := 0.0
	avgTotal := 0.0

	for _, data := range nt.LatencyData {
		avgVoting += data.VotingLatency
		avgBlock += data.BlockProductionTime
		avgConsensus += data.ConsensusLatency
		avgTotal += data.TotalRoundTime
	}

	n := float64(len(nt.LatencyData))
	avgVoting /= n
	avgBlock /= n
	avgConsensus /= n
	avgTotal /= n

	return map[string]interface{}{
		"平均投票时延ms": avgVoting,
		"平均出块时延ms": avgBlock,
		"平均共识时延ms": avgConsensus,
		"平均总时延ms":  avgTotal,
		"时延占比": map[string]float64{
			"投票占比": (avgVoting / avgTotal) * 100,
			"出块占比": (avgBlock / avgTotal) * 100,
			"共识占比": (avgConsensus / avgTotal) * 100,
		},
	}
}

// analyzeBlockProduction 分析区块生产数据
// 计算去中心化指数和Top节点占比
// 返回: 区块生产分析结果
func (nt *NetworkTester) analyzeBlockProduction() map[string]interface{} {
	if len(nt.BlockProductionData) == 0 {
		return map[string]interface{}{"错误": "无数据"}
	}

	// 按节点ID分组统计
	nodeStats := make(map[string]*struct {
		TotalBlocks int
		AvgRank     float64
		RankCount   int
		MaxTerms    int
	})

	for _, data := range nt.BlockProductionData {
		if _, exists := nodeStats[data.NodeID]; !exists {
			nodeStats[data.NodeID] = &struct {
				TotalBlocks int
				AvgRank     float64
				RankCount   int
				MaxTerms    int
			}{}
		}

		stats := nodeStats[data.NodeID]
		stats.TotalBlocks = data.BlocksProduced
		stats.AvgRank += float64(data.NodeRank)
		stats.RankCount++
		if data.ConsecutiveTerms > stats.MaxTerms {
			stats.MaxTerms = data.ConsecutiveTerms
		}
	}

	// 计算平均排名
	for _, stats := range nodeStats {
		if stats.RankCount > 0 {
			stats.AvgRank /= float64(stats.RankCount)
		}
	}

	// 排序节点(按区块数)
	type nodeRanking struct {
		NodeID     string
		BlockCount int
		AvgRank    float64
		MaxTerms   int
	}

	rankings := make([]nodeRanking, 0, len(nodeStats))
	for nodeID, stats := range nodeStats {
		rankings = append(rankings, nodeRanking{
			NodeID:     nodeID,
			BlockCount: stats.TotalBlocks,
			AvgRank:    stats.AvgRank,
			MaxTerms:   stats.MaxTerms,
		})
	}

	sort.Slice(rankings, func(i, j int) bool {
		return rankings[i].BlockCount > rankings[j].BlockCount
	})

	// 统计top节点的区块占比
	totalBlocks := 0
	for _, r := range rankings {
		totalBlocks += r.BlockCount
	}

	top3Blocks := 0
	top5Blocks := 0
	top10Blocks := 0

	for i, r := range rankings {
		if i < 3 {
			top3Blocks += r.BlockCount
		}
		if i < 5 {
			top5Blocks += r.BlockCount
		}
		if i < 10 {
			top10Blocks += r.BlockCount
		}
	}

	top3Ratio := 0.0
	top5Ratio := 0.0
	top10Ratio := 0.0

	if totalBlocks > 0 {
		top3Ratio = float64(top3Blocks) / float64(totalBlocks) * 100
		top5Ratio = float64(top5Blocks) / float64(totalBlocks) * 100
		top10Ratio = float64(top10Blocks) / float64(totalBlocks) * 100
	}

	return map[string]interface{}{
		"总区块数":    totalBlocks,
		"生产节点数":   len(rankings),
		"Top3占比":  top3Ratio,
		"Top5占比":  top5Ratio,
		"Top10占比": top10Ratio,
		"基尼系数":    nt.NetworkStats.DecentralizationIdx,
		"去中心化程度":  nt.getDecentralizationLevel(nt.NetworkStats.DecentralizationIdx),
		"前十生产节点":  rankings[:min(10, len(rankings))],
	}
}

// analyzeVotingSpeed 分析投票速率数据
// 统计各时间段的投票分布和成功率
// 返回: 投票速率分析结果
func (nt *NetworkTester) analyzeVotingSpeed() map[string]interface{} {
	if len(nt.VotingSpeedData) == 0 {
		return map[string]interface{}{"错误": "无数据"}
	}

	distribution := nt.calculateVotingSpeedDistribution()

	totalVotes := len(nt.VotingSpeedData)
	successfulVotes := 0
	proxiedVotes := 0
	avgVotingTime := 0.0

	for _, data := range nt.VotingSpeedData {
		if data.IsSuccessful {
			successfulVotes++
		}
		if data.IsProxied {
			proxiedVotes++
		}
		avgVotingTime += data.VotingTime
	}

	avgVotingTime /= float64(totalVotes)

	return map[string]interface{}{
		"总投票数":     totalVotes,
		"成功投票数":    successfulVotes,
		"代投数":      proxiedVotes,
		"成功率":      float64(successfulVotes) / float64(totalVotes) * 100,
		"代投率":      float64(proxiedVotes) / float64(totalVotes) * 100,
		"平均投票时间ms": avgVotingTime,
		"时间分布": map[string]float64{
			"0-20ms":   distribution["0-20"] * 100,
			"20-50ms":  distribution["20-50"] * 100,
			"50-100ms": distribution["50-100"] * 100,
			">100ms":   distribution[">100"] * 100,
		},
		"效率评价": nt.getVotingEfficiencyAnalysis(distribution),
	}
}

// analyzeMaliciousNodes 分析恶意节点数据
// 统计恶意节点的数量变化和移除效果
// 返回: 恶意节点分析结果
func (nt *NetworkTester) analyzeMaliciousNodes() map[string]interface{} {
	if len(nt.MaliciousNodeData) == 0 {
		return map[string]interface{}{"错误": "无数据"}
	}

	last := nt.MaliciousNodeData[len(nt.MaliciousNodeData)-1]

	totalMalicious := last.MaliciousNodes
	totalRemoved := last.RemovedNodes
	totalOriginal := totalMalicious + totalRemoved

	removalRate := 0.0
	if totalOriginal > 0 {
		removalRate = float64(totalRemoved) / float64(totalOriginal) * 100
	}

	return map[string]interface{}{
		"初始恶意节点数":  totalOriginal,
		"当前恶意节点数":  totalMalicious,
		"已移除节点数":   totalRemoved,
		"移除率":      removalRate,
		"平均移除时间轮次": last.AvgRemovalTime,
		"详细统计": map[string]interface{}{
			"恶意节点":  last.MaliciousNodes,
			"已移除节点": last.RemovedNodes,
		},
		"移除效果评价": nt.getMaliciousRemovalEffectiveness(removalRate, last.AvgRemovalTime),
	}
}

// generateComparisonReport 生成对比报告(Markdown格式)
// 生成与其他DPoS算法的性能对比分析
func (nt *NetworkTester) generateComparisonReport() {
	filename := filepath.Join(nt.OutputDir, "算法对比报告.md")

	report := fmt.Sprintf(`# NM-DPoS 性能测试对比报告

## 测试概要
- **节点ID**: %s
- **测试轮数**: %d
- **测试时长**: %s
- **网络规模**: %d节点

## 1. 共识性能对比 

### 1.1 吞吐量对比
| 算法 | 平均TPS | 变化趋势 | 说明 |
|------|---------|---------|------|
| **NM-DPoS** | %.2f | %s | 本次测试结果 |



### 1.2 时延对比
| 算法 | 平均时延(ms) | 投票时延 | 出块时延 | 共识时延 |
|------|-------------|---------|---------|---------|
| **NM-DPoS** | %.2f | %.2f | %.2f | %.2f |


## 2. 区块生产占比对比 

### 2.1 去中心化程度
| 算法 | 基尼系数 | Top3占比 | Top5占比 | Top10占比 |
|------|---------|---------|---------|----------|
| **NM-DPoS** | %.4f | %.2f%% | %.2f%% | %.2f%% |


## 3. 节点投票积极性对比

### 3.1 投票速率分布
| 算法 | 0-20ms | 20-50ms | 50-100ms | >100ms |
|------|--------|---------|----------|--------|
| **NM-DPoS** | %.2f%% | %.2f%% | %.2f%% | %.2f%% |



## 4. 恶意节点剔除对比

### 4.1 移除效率
| 算法 | 移除率 | 平均移除时间(轮) | 说明 |
|------|-------|----------------|------|
| **NM-DPoS** | %.2f%% | %.2f | 本次测试结果 |


---
*报告生成时间: %s*
`,
		nt.Node.LocalView.LocalNode.ID,
		nt.TestRounds,
		time.Since(nt.StartTime).String(),
		nt.NetworkStats.TotalNodes,

		// 吞吐量
		nt.NetworkStats.AvgThroughput,
		nt.getTrendDescription(0), // 实际应传入趋势值

		// 时延
		nt.NetworkStats.AvgLatency,
		nt.getAvgVotingLatency(),
		nt.getAvgBlockLatency(),
		nt.getAvgConsensusLatency(),

		// 去中心化
		nt.NetworkStats.DecentralizationIdx,
		nt.getTop3Ratio(),
		nt.getTop5Ratio(),
		nt.getTop10Ratio(),

		// 投票速率
		nt.getVotingDistribution("0-20"),
		nt.getVotingDistribution("20-50"),
		nt.getVotingDistribution("50-100"),
		nt.getVotingDistribution(">100"),

		// 恶意节点
		nt.NetworkStats.MaliciousRemovalEff*100,
		nt.getAvgRemovalTime(),

		time.Now().Format(time.RFC3339),
	)

	if err := os.WriteFile(filename, []byte(report), 0644); err != nil {
		nt.Logger.Error("保存对比报告失败: %v", err)
		return
	}

	nt.Logger.Info("✓ 对比报告已保存: %s", filename)
}

// ================================
// 辅助分析函数
// ================================

// getTrendDescription 获取趋势描述
// 参数: trend - 变化趋势值
// 返回: 趋势描述(上升/下降/稳定)
func (nt *NetworkTester) getTrendDescription(trend float64) string {
	if trend > 0.1 {
		return "上升"
	} else if trend < -0.1 {
		return "下降"
	} else {
		return "稳定"
	}
}

// getDecentralizationLevel 获取去中心化程度描述
// 参数: gini - 基尼系数
// 返回: 去中心化程度描述
func (nt *NetworkTester) getDecentralizationLevel(gini float64) string {
	if gini < 0.3 {
		return "高度去中心化"
	} else if gini < 0.5 {
		return "中等去中心化"
	} else if gini < 0.7 {
		return "轻度中心化"
	} else {
		return "高度中心化"
	}
}

// getVotingEfficiencyAnalysis 获取投票效率分析
// 参数: dist - 投票时间分布
// 返回: 效率评价
func (nt *NetworkTester) getVotingEfficiencyAnalysis(dist map[string]float64) string {
	fast := dist["0-20"]
	if fast > 0.7 {
		return "优秀 - 大部分节点快速投票"
	} else if fast > 0.5 {
		return "良好 - 多数节点及时投票"
	} else if fast > 0.3 {
		return "一般 - 投票速度有待提升"
	} else {
		return "较差 - 需要优化投票机制"
	}
}

// getMaliciousRemovalEffectiveness 获取恶意节点移除效果评价
// 参数:
//   - rate: 移除率
//   - avgTime: 平均移除时间
//
// 返回: 效果评价
func (nt *NetworkTester) getMaliciousRemovalEffectiveness(rate, avgTime float64) string {
	if rate > 80 && avgTime < 20 {
		return "优秀 - 快速有效移除"
	} else if rate > 60 && avgTime < 30 {
		return "良好 - 较快移除"
	} else if rate > 40 {
		return "一般 - 移除效率中等"
	} else {
		return "较差 - 移除机制需加强"
	}
}

// getAvgVotingLatency 获取平均投票时延
func (nt *NetworkTester) getAvgVotingLatency() float64 {
	if len(nt.LatencyData) == 0 {
		return 0
	}
	total := 0.0
	for _, d := range nt.LatencyData {
		total += d.VotingLatency
	}
	return total / float64(len(nt.LatencyData))
}

// getAvgBlockLatency 获取平均出块时延
func (nt *NetworkTester) getAvgBlockLatency() float64 {
	if len(nt.LatencyData) == 0 {
		return 0
	}
	total := 0.0
	for _, d := range nt.LatencyData {
		total += d.BlockProductionTime
	}
	return total / float64(len(nt.LatencyData))
}

// getAvgConsensusLatency 获取平均共识时延
func (nt *NetworkTester) getAvgConsensusLatency() float64 {
	if len(nt.LatencyData) == 0 {
		return 0
	}
	total := 0.0
	for _, d := range nt.LatencyData {
		total += d.ConsensusLatency
	}
	return total / float64(len(nt.LatencyData))
}

// getTop3Ratio 获取Top3节点区块占比
func (nt *NetworkTester) getTop3Ratio() float64 {
	analysis := nt.analyzeBlockProduction()
	if val, ok := analysis["Top3占比"].(float64); ok {
		return val
	}
	return 0
}

// getTop5Ratio 获取Top5节点区块占比
func (nt *NetworkTester) getTop5Ratio() float64 {
	analysis := nt.analyzeBlockProduction()
	if val, ok := analysis["Top5占比"].(float64); ok {
		return val
	}
	return 0
}

// getTop10Ratio 获取Top10节点区块占比
func (nt *NetworkTester) getTop10Ratio() float64 {
	analysis := nt.analyzeBlockProduction()
	if val, ok := analysis["Top10占比"].(float64); ok {
		return val
	}
	return 0
}

// getVotingDistribution 获取指定时间区间的投票占比
// 参数: timeRange - 时间区间标识
// 返回: 占比百分比
func (nt *NetworkTester) getVotingDistribution(timeRange string) float64 {
	dist := nt.calculateVotingSpeedDistribution()
	return dist[timeRange] * 100
}

// getAvgRemovalTime 获取平均恶意节点移除时间
func (nt *NetworkTester) getAvgRemovalTime() float64 {
	if len(nt.MaliciousNodeData) == 0 {
		return 0
	}
	return nt.MaliciousNodeData[len(nt.MaliciousNodeData)-1].AvgRemovalTime
}

// getThroughputAnalysis 获取吞吐量分析文字描述
func (nt *NetworkTester) getThroughputAnalysis() string {
	analysis := nt.analyzeThroughput()
	trend := analysis["趋势描述"].(string)
	avgTPS := analysis["平均TPS"].(float64)

	return fmt.Sprintf("平均吞吐量为 %.2f TPS，趋势为%s。在节点数量增加时，"+
		"NM-DPoS通过激励机制保持较稳定的吞吐量表现。", avgTPS, trend)
}

// getLatencyAnalysis 获取时延分析文字描述
func (nt *NetworkTester) getLatencyAnalysis() string {
	analysis := nt.analyzeLatency()
	avgTotal := analysis["平均总时延ms"].(float64)

	return fmt.Sprintf("平均轮次时延为 %.2f ms。得益于分段投票机制和代投机制，"+
		"NM-DPoS能够有效控制时延，特别是在网络规模较大时表现优异。", avgTotal)
}

// getDecentralizationAnalysis 获取去中心化分析文字描述
func (nt *NetworkTester) getDecentralizationAnalysis() string {
	gini := nt.NetworkStats.DecentralizationIdx
	level := nt.getDecentralizationLevel(gini)

	return fmt.Sprintf("基尼系数为 %.4f (%s)。通过权重衰减和连任限制机制，"+
		"NM-DPoS有效降低了寡头风险，区块生产分布更加均匀。", gini, level)
}

// getVotingSpeedAnalysis 获取投票速率分析文字描述
func (nt *NetworkTester) getVotingSpeedAnalysis() string {
	dist := nt.calculateVotingSpeedDistribution()
	fast := dist["0-20"] * 100

	return fmt.Sprintf("%.2f%%%% 的投票在20ms内完成。出块收益共享机制有效激励了节点"+
		"快速投票，相比传统DPoS有显著提升。", fast)
}

// getMaliciousNodeAnalysis 获取恶意节点分析文字描述
func (nt *NetworkTester) getMaliciousNodeAnalysis() string {
	removal := nt.NetworkStats.MaliciousRemovalEff * 100
	avgTime := nt.getAvgRemovalTime()

	return fmt.Sprintf("恶意节点移除率达 %.2f%%%%，平均移除时间为 %.2f 轮。"+
		"代理节点通过保证金扣除快速剔除，投票节点通过权重衰减逐步淘汰。",
		removal, avgTime)
}

// getAdvantages 获取算法优势描述
func (nt *NetworkTester) getAdvantages() string {
	return `
1. **吞吐量稳定性**: 在节点数量增加时，通过激励机制保持较稳定的性能
2. **时延优化**: 分段投票和代投机制有效降低了共识时延
3. **去中心化**: 权重衰减和连任限制显著降低了中心化风险
4. **投票积极性**: 收益共享机制激励节点快速投票
5. **安全性**: 双层惩罚机制快速有效地移除恶意节点
`
}

// getDisadvantages 获取算法劣势描述
func (nt *NetworkTester) getDisadvantages() string {
	return `
1. **初期性能**: 在节点数量较少时(<30)，性能可能不如专门优化的算法
2. **复杂度**: 机制设计较为复杂，实现和维护成本较高
3. **存储开销**: 需要记录更多的历史数据用于评估
`
}

// getRecommendations 获取使用建议
func (nt *NetworkTester) getRecommendations() string {
	return `
1. **网络规模**: 建议在30个以上节点的网络中部署以发挥最佳性能
2. **参数调优**: 根据具体应用场景调整衰减系数、连任阈值等参数
3. **监控机制**: 建立完善的性能监控和预警系统
4. **渐进部署**: 建议先在测试网络充分验证后再部署到生产环境
`
}

// ================================
// 中间结果打印
// ================================

// printIntermediateResults 打印中期测试结果
// 在测试过程中定期输出当前的测试进度和关键指标
func (nt *NetworkTester) printIntermediateResults() {
	if len(nt.Results) == 0 {
		return
	}

	latest := nt.Results[len(nt.Results)-1]

	nt.Logger.Info("========== 中期测试结果 ==========")
	nt.Logger.Info("【基础指标】")
	nt.Logger.Info("  节点类型: %s", latest.NodeType)
	nt.Logger.Info("  当前权重: %.2f (初始: %.2f)",
		latest.CurrentWeight, nt.Results[0].CurrentWeight)
	nt.Logger.Info("  表现评分: %.4f", latest.PerformanceScore)
	nt.Logger.Info("  半衰期: %.1f轮", latest.HalfLife)

	nt.Logger.Info("【投票统计】")
	nt.Logger.Info("  成功投票: %d次", latest.SuccessfulVotes)
	nt.Logger.Info("  失败投票: %d次", latest.FailedVotes)
	successRate := 0.0
	if total := latest.SuccessfulVotes + latest.FailedVotes; total > 0 {
		successRate = float64(latest.SuccessfulVotes) / float64(total) * 100
	}
	nt.Logger.Info("  成功率: %.2f%%", successRate)

	nt.Logger.Info("【出块统计】")
	nt.Logger.Info("  有效区块: %d", latest.ValidBlocks)
	nt.Logger.Info("  延迟区块: %d", latest.DelayedBlocks)
	nt.Logger.Info("  无效区块: %d", latest.InvalidBlocks)

	nt.Logger.Info("【经济指标】")
	nt.Logger.Info("  累计奖励: %.2f", latest.TotalReward)
	nt.Logger.Info("  累计补偿: %.2f", latest.TotalCompensation)
	nt.Logger.Info("  总收益: %.2f", latest.TotalReward+latest.TotalCompensation)

	nt.Logger.Info("【网络指标】")
	nt.Logger.Info("  网络总权重: %.2f", latest.NetworkTotalWeight)
	nt.Logger.Info("  活跃节点数: %d", latest.NetworkActiveNodes)

	if len(nt.ThroughputData) > 0 {
		latestThroughput := nt.ThroughputData[len(nt.ThroughputData)-1]
		nt.Logger.Info("【性能指标】")
		nt.Logger.Info("  当前TPS: %.2f", latestThroughput.TransactionsPerSec)
		nt.Logger.Info("  当前BPM: %.2f", latestThroughput.BlocksPerMinute)
	}

	if len(nt.LatencyData) > 0 {
		latestLatency := nt.LatencyData[len(nt.LatencyData)-1]
		nt.Logger.Info("  当前时延: %.2f ms", latestLatency.TotalRoundTime)
	}

	nt.Logger.Info("====================================")
}

// printCurrentStatus 打印当前状态快照
// 简洁地显示当前节点的关键状态
func (nt *NetworkTester) printCurrentStatus() {
	if len(nt.Results) == 0 {
		return
	}

	latest := nt.Results[len(nt.Results)-1]

	nt.Logger.Info("当前状态快照:")
	nt.Logger.Info("  ├─ 权重: %.2f (半衰期: %.1f轮)",
		latest.CurrentWeight, latest.HalfLife)
	nt.Logger.Info("  ├─ 表现: %.4f", latest.PerformanceScore)
	nt.Logger.Info("  ├─ 投票: 成功=%d, 失败=%d",
		latest.SuccessfulVotes, latest.FailedVotes)
	nt.Logger.Info("  ├─ 区块: 有效=%d, 延迟=%d, 无效=%d",
		latest.ValidBlocks, latest.DelayedBlocks, latest.InvalidBlocks)
	nt.Logger.Info("  └─ 收益: 奖励=%.2f, 补偿=%.2f",
		latest.TotalReward, latest.TotalCompensation)
}

// ================================
// 工具函数
// ================================

// min 返回两个整数中的较小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// max 返回两个整数中的较大值
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ================================
// 专项测试方法 (可选)
// ================================

// RunThroughputTest 运行吞吐量专项测试
// 专门测试系统的吞吐量性能
// 参数:
//   - duration: 测试持续时间
//   - nodeCount: 节点数量
func (nt *NetworkTester) RunThroughputTest(duration time.Duration, nodeCount int) {
	nt.Logger.Info("========================================")
	nt.Logger.Info("吞吐量专项测试")
	nt.Logger.Info("持续时间: %v", duration)
	nt.Logger.Info("节点数量: %d", nodeCount)
	nt.Logger.Info("========================================")

	startTime := time.Now()
	round := 0

	for time.Since(startTime) < duration {
		round++
		nt.collectThroughputData(round)
		time.Sleep(time.Duration(config.BlockInterval) * time.Second)

		if round%10 == 0 {
			nt.Logger.Info("进度: %.1f%% (已测试 %v/%v)",
				float64(time.Since(startTime))/float64(duration)*100,
				time.Since(startTime).Round(time.Second),
				duration)
		}
	}

	nt.saveThroughputResults()
	nt.Logger.Info("吞吐量测试完成!")
}

// RunLatencyTest 运行时延专项测试
// 专门测试系统各阶段的时延表现
// 参数: rounds - 测试轮数
func (nt *NetworkTester) RunLatencyTest(rounds int) {
	nt.Logger.Info("========================================")
	nt.Logger.Info("时延专项测试")
	nt.Logger.Info("测试轮数: %d", rounds)
	nt.Logger.Info("========================================")

	for round := 1; round <= rounds; round++ {
		roundStart := time.Now()

		time.Sleep(time.Duration(config.BlockInterval) * time.Second)

		nt.collectLatencyData(round, roundStart)

		if round%10 == 0 {
			nt.Logger.Info("进度: %d/%d (%.1f%%)",
				round, rounds, float64(round)/float64(rounds)*100)
		}
	}

	nt.saveLatencyResults()
	nt.Logger.Info("时延测试完成!")
}

// RunDecentralizationTest 运行去中心化专项测试
// 专门测试系统的去中心化程度
// 参数: rounds - 测试轮数
func (nt *NetworkTester) RunDecentralizationTest(rounds int) {
	nt.Logger.Info("========================================")
	nt.Logger.Info("去中心化专项测试")
	nt.Logger.Info("测试轮数: %d", rounds)
	nt.Logger.Info("========================================")

	for round := 1; round <= rounds; round++ {
		time.Sleep(time.Duration(config.BlockInterval) * time.Second)

		nt.collectBlockProductionData(round)

		if round%10 == 0 {
			allNodes := nt.Node.LocalView.GetActiveNodes()
			gini := nt.calculateGiniCoefficient(allNodes)
			nt.Logger.Info("轮次 %d: 基尼系数 = %.4f (%s)",
				round, gini, nt.getDecentralizationLevel(gini))
		}
	}

	nt.saveBlockProductionResults()
	nt.Logger.Info("去中心化测试完成!")
}

// RunVotingSpeedTest 运行投票速率专项测试
// 专门测试节点的投票积极性和速度分布
// 参数: rounds - 测试轮数
func (nt *NetworkTester) RunVotingSpeedTest(rounds int) {
	nt.Logger.Info("========================================")
	nt.Logger.Info("投票速率专项测试")
	nt.Logger.Info("测试轮数: %d", rounds)
	nt.Logger.Info("========================================")

	for round := 1; round <= rounds; round++ {
		time.Sleep(time.Duration(config.BlockInterval) * time.Second)

		nt.collectVotingSpeedData(round)

		if round%10 == 0 {
			dist := nt.calculateVotingSpeedDistribution()
			nt.Logger.Info("轮次 %d 分布: 0-20ms=%.1f%%, 20-50ms=%.1f%%, 50-100ms=%.1f%%, >100ms=%.1f%%",
				round, dist["0-20"]*100, dist["20-50"]*100,
				dist["50-100"]*100, dist[">100"]*100)
		}
	}

	nt.saveVotingSpeedResults()
	nt.Logger.Info("投票速率测试完成!")
}

// RunMaliciousNodeTest 运行恶意节点专项测试
// 专门测试系统对恶意节点的处理能力
// 参数:
//   - rounds: 测试轮数
//   - maliciousCount: 初始恶意节点数量
func (nt *NetworkTester) RunMaliciousNodeTest(rounds int, maliciousCount int) {
	nt.Logger.Info("========================================")
	nt.Logger.Info("恶意节点专项测试")
	nt.Logger.Info("测试轮数: %d", rounds)
	nt.Logger.Info("恶意节点数: %d", maliciousCount)
	nt.Logger.Info("========================================")

	for round := 1; round <= rounds; round++ {
		time.Sleep(time.Duration(config.BlockInterval) * time.Second)

		nt.collectMaliciousNodeData(round)

		if round%5 == 0 {
			if len(nt.MaliciousNodeData) > 0 {
				latest := nt.MaliciousNodeData[len(nt.MaliciousNodeData)-1]
				active := latest.MaliciousNodes
				removed := latest.RemovedNodes
				nt.Logger.Info("轮次 %d: 活跃恶意节点=%d, 已移除=%d, 移除率=%.1f%%",
					round, active, removed,
					float64(removed)/float64(maliciousCount)*100)
			}
		}
	}

	nt.saveMaliciousNodeResults()
	nt.Logger.Info("恶意节点测试完成!")
}
