package simulation

import (
	"fmt"
	"math/rand"
	"nm-dpos/config"
	"nm-dpos/core"
	"nm-dpos/network"
	"nm-dpos/types"
	"nm-dpos/utils"
	"time"
)

// simulator.go
// 完整的集中式性能测试框架（修正版）

// Simulator 模拟器
type Simulator struct {
	// 使用LocalNetworkView进行集中式模拟
	LocalView      *network.LocalNetworkView
	Logger         *utils.Logger
	RoundSummaries []*types.RoundSummary
	StartTime      time.Time

	// 性能测试统计
	PerformanceMetrics *PerformanceMetrics
}

// PerformanceMetrics 性能指标
type PerformanceMetrics struct {
	TotalRounds        int
	TotalExecutionTime time.Duration
	AverageRoundTime   time.Duration
	MinRoundTime       time.Duration
	MaxRoundTime       time.Duration
	TotalBlocks        int
	TotalTransactions  int
	TPS                float64
	AverageLatency     float64
	MemoryUsageMB      float64
}

// NewSimulator 创建模拟器
func NewSimulator(logLevel utils.LogLevel) *Simulator {
	logger := utils.NewLogger(logLevel)

	// 创建虚拟本地节点（模拟器控制器）
	controllerNode := types.NewNode("Simulator-Controller", 0, 1.0, 0)

	// 创建消息总线
	messageBus := network.NewMessageBus()

	// 创建LocalNetworkView
	localView := network.NewLocalNetworkView(controllerNode, messageBus, logger)

	return &Simulator{
		LocalView:          localView,
		Logger:             logger,
		RoundSummaries:     make([]*types.RoundSummary, 0),
		PerformanceMetrics: &PerformanceMetrics{},
	}
}

// ================================
// 网络初始化
// ================================

// InitializeNetwork 初始化网络（创建6种典型节点）
func (sim *Simulator) InitializeNetwork() {
	sim.Logger.LogSection("初始化测试网络")

	// 第2.4节：6种典型节点类型
	nodeTypes := []struct {
		prefix      string
		count       int
		weight      float64
		performance float64
		description string
	}{
		{"H-G", 5, 150.0, 0.95, "高权重优秀"},
		{"H-M", 5, 150.0, 0.70, "高权重良好"},
		{"H-B", 5, 150.0, 0.30, "高权重差劲"},
		{"L-G", 10, 50.0, 0.95, "低权重优秀"},
		{"L-M", 15, 50.0, 0.70, "低权重良好"},
		{"L-B", 10, 50.0, 0.30, "低权重差劲"},
	}

	totalNodes := 0
	for _, nodeType := range nodeTypes {
		for i := 0; i < nodeType.count; i++ {
			nodeID := fmt.Sprintf("%s-%02d", nodeType.prefix, i+1)
			networkDelay := 10.0 + rand.Float64()*40.0

			node := types.NewNode(
				nodeID,
				nodeType.weight,
				nodeType.performance,
				networkDelay,
			)

			// 添加到LocalView
			sim.LocalView.UpdatePeerInfo(nodeID, node)
			totalNodes++
		}

		sim.Logger.Info("创建 %d 个%s节点 (权重=%.0f, 表现=%.2f)",
			nodeType.count, nodeType.description, nodeType.weight, nodeType.performance)
	}

	sim.Logger.Info("\n总计创建 %d 个节点", totalNodes)
	sim.LocalView.Network.Nodes = sim.LocalView.GetAllKnownNodes()

	sim.Logger.Info("初始总权重: %.2f", sim.LocalView.TotalWeight)
	sim.Logger.Info("环境权重: %.2f\n", sim.LocalView.EnvironmentWeight)
}

// InitializeNetworkWithCount 初始化指定数量的节点
func (sim *Simulator) InitializeNetworkWithCount(nodeCount int) {
	sim.Logger.LogSection(fmt.Sprintf("初始化测试网络 (%d个节点)", nodeCount))

	for i := 0; i < nodeCount; i++ {
		nodeID := fmt.Sprintf("Node-%04d", i)
		weight := 50.0 + rand.Float64()*100.0
		performance := 0.3 + rand.Float64()*0.6
		delay := 10.0 + rand.Float64()*40.0

		node := types.NewNode(nodeID, weight, performance, delay)
		sim.LocalView.UpdatePeerInfo(nodeID, node)
	}

	sim.Logger.Info("创建 %d 个节点完成", nodeCount)
	sim.LocalView.Network.Nodes = sim.LocalView.GetAllKnownNodes()
}

// ================================
// 模拟运行
// ================================

// Run 运行模拟
func (sim *Simulator) Run(rounds int) {
	sim.Logger.LogSection(fmt.Sprintf("开始模拟 (%d轮)", rounds))

	sim.StartTime = time.Now()
	sim.PerformanceMetrics.TotalRounds = rounds

	minRoundTime := time.Duration(999999 * time.Hour)
	maxRoundTime := time.Duration(0)
	totalRoundTime := time.Duration(0)

	for round := 1; round <= rounds; round++ {
		roundStart := time.Now()

		// 执行一轮共识（使用LocalView的完整逻辑）
		summary := sim.LocalView.ExecuteRound()
		sim.RoundSummaries = append(sim.RoundSummaries, summary)

		roundTime := time.Since(roundStart)
		totalRoundTime += roundTime

		if roundTime < minRoundTime {
			minRoundTime = roundTime
		}
		if roundTime > maxRoundTime {
			maxRoundTime = roundTime
		}

		// 统计区块和交易
		sim.PerformanceMetrics.TotalBlocks += summary.BlockStats.TotalBlocks

		// 每10轮输出一次进度
		if round%10 == 0 {
			sim.Logger.Info("进度: %d/%d 轮完成 (%.1f%%), 平均耗时: %v",
				round, rounds, float64(round)/float64(rounds)*100, totalRoundTime/time.Duration(round))
		}

		// 检查网络健康度
		if sim.LocalView.Network.ActiveNodesCount < config.NumDelegates {
			sim.Logger.Warn("警告：活跃节点数不足，提前终止模拟")
			sim.PerformanceMetrics.TotalRounds = round
			break
		}
	}

	// 计算性能指标
	sim.PerformanceMetrics.TotalExecutionTime = time.Since(sim.StartTime)
	sim.PerformanceMetrics.AverageRoundTime = totalRoundTime / time.Duration(sim.PerformanceMetrics.TotalRounds)
	sim.PerformanceMetrics.MinRoundTime = minRoundTime
	sim.PerformanceMetrics.MaxRoundTime = maxRoundTime

	// 计算TPS
	totalTime := sim.PerformanceMetrics.TotalExecutionTime.Seconds()
	if totalTime > 0 {
		sim.PerformanceMetrics.TPS = float64(sim.PerformanceMetrics.TotalTransactions) / totalTime
	}

	sim.Logger.Info("\n模拟完成，总耗时: %v", sim.PerformanceMetrics.TotalExecutionTime)

	// 生成最终报告
	sim.GenerateFinalReport()
}

// ================================
// 测试用例（保持不变）
// ================================

// TestWeightDecay 测试权重衰减特性
func (sim *Simulator) TestWeightDecay() {
	sim.Logger.LogSection("测试：权重衰减特性（第2章验证）")

	testNodes := []*types.Node{
		types.NewNode("Test-Excellent", 100.0, 0.95, 20.0),
		types.NewNode("Test-Good", 100.0, 0.70, 20.0),
		types.NewNode("Test-Poor", 100.0, 0.30, 20.0),
	}

	sim.Logger.Info("\n初始状态:")
	for _, node := range testNodes {
		halfLife := core.CalculateHalfLife(node.PerformanceScore)
		sim.Logger.Info("  %s: 权重=%.2f, 表现=%.2f, 半衰期=%.1f周期",
			node.ID, node.CurrentWeight, node.PerformanceScore, halfLife)
	}

	periods := []int{0, 5, 10, 15, 20, 30, 50}

	sim.Logger.Info("\n权重演化表（验证第2.4节的理论数据）:")
	sim.Logger.Info("  周期    优秀(0.95)    良好(0.70)    差劲(0.30)")
	sim.Logger.Info("  ----    ----------    ----------    ----------")

	for _, period := range periods {
		weights := make([]float64, len(testNodes))

		for i, node := range testNodes {
			decayRate := core.CalculateDecayRate(node.PerformanceScore)
			weights[i] = utils.NewtonCooling(100.0, 0.0, decayRate, float64(period))
		}

		sim.Logger.Info("  %-4d    %10.2f    %10.2f    %10.2f",
			period, weights[0], weights[1], weights[2])
	}

	// 验证公平性
	sim.Logger.Info("\n公平性验证（第2.5节）:")
	types.NewNode("Fair-H", 150.0, 0.95, 20.0)
	types.NewNode("Fair-L", 50.0, 0.95, 20.0)

	decayRate := core.CalculateDecayRate(0.95)
	weight1After50 := utils.NewtonCooling(150.0, 0, decayRate, 50)
	weight2After50 := utils.NewtonCooling(50.0, 0, decayRate, 50)

	ratio1 := weight1After50 / 150.0
	ratio2 := weight2After50 / 50.0

	sim.Logger.Info("  高权重节点 (150 → %.2f): 保留率 %.4f%%", weight1After50, ratio1*100)
	sim.Logger.Info("  低权重节点 (50 → %.2f): 保留率 %.4f%%", weight2After50, ratio2*100)
	sim.Logger.Info("  公平性验证: %v (差异=%.10f)", abs(ratio1-ratio2) < 1e-6, ratio1-ratio2)
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// TestVotingMechanism 测试投票机制
func (sim *Simulator) TestVotingMechanism() {
	sim.Logger.LogSection("测试：动态投票机制（第4章验证）")

	scenarios := []struct {
		name              string
		avgDelay          float64
		participationRate float64
	}{
		{"低延迟高参与", 10.0, 0.9},
		{"中延迟中参与", 30.0, 0.6},
		{"高延迟低参与", 50.0, 0.3},
	}

	sim.Logger.Info("\n投票区段动态调整测试:")
	sim.Logger.Info("  场景              平均延迟    参与率    正常投票区段")
	sim.Logger.Info("  --------------  ----------  --------  --------------")

	for _, scenario := range scenarios {
		nodes := make([]*types.Node, 100)
		for i := range nodes {
			nodes[i] = types.NewNode(
				fmt.Sprintf("Test-%d", i),
				50.0,
				0.7,
				scenario.avgDelay+rand.Float64()*10.0,
			)
		}

		votingPeriod := core.CalculateNormalVotingPeriod(
			nodes,
			scenario.avgDelay,
			int(float64(len(nodes))*scenario.participationRate),
		)

		sim.Logger.Info("  %-14s  %10.1f  %8.1f%%  %14.2f ms",
			scenario.name,
			scenario.avgDelay,
			scenario.participationRate*100,
			votingPeriod,
		)
	}
}

// TestRewardDistribution 测试奖励分配
func (sim *Simulator) TestRewardDistribution() {
	sim.Logger.LogSection("测试：奖励分配机制（第5章验证）")

	participants := []*types.Node{
		types.NewNode("Excellent", 100.0, 0.95, 20.0),
		types.NewNode("Good", 100.0, 0.70, 20.0),
		types.NewNode("Average", 100.0, 0.50, 20.0),
	}

	blockReward := 100.0

	sim.Logger.Info("\n修正Shapley值分配测试:")
	sim.Logger.Info("  区块奖励: %.2f", blockReward)

	rewards := core.DistributeBlockReward(blockReward, participants)

	sim.Logger.Info("\n  节点        表现评分    分配奖励    占比")
	sim.Logger.Info("  ---------  ----------  ----------  ------")

	totalDistributed := 0.0
	for _, node := range participants {
		reward := rewards[node.ID]
		totalDistributed += reward
		sim.Logger.Info("  %-9s  %10.3f  %10.2f  %5.1f%%",
			node.ID,
			node.PerformanceScore,
			reward,
			reward/blockReward*100,
		)
	}

	sim.Logger.Info("\n  总计: %.2f (验证: %v)",
		totalDistributed,
		abs(totalDistributed-blockReward) < 0.01)
}

// ================================
// 压力测试
// ================================

// StressTest 压力测试
func (sim *Simulator) StressTest(nodeCount, rounds int) {
	sim.Logger.LogSection(fmt.Sprintf("压力测试: %d个节点, %d轮", nodeCount, rounds))

	startTime := time.Now()

	sim.InitializeNetworkWithCount(nodeCount)
	initTime := time.Since(startTime)
	sim.Logger.Info("节点初始化完成: %v", initTime)

	sim.Run(rounds)

	sim.Logger.Info("\n压力测试完成")
	sim.PrintPerformanceMetrics()
}

// ================================
// 性能指标输出
// ================================

// PrintPerformanceMetrics 打印性能指标
func (sim *Simulator) PrintPerformanceMetrics() {
	sim.Logger.LogSection("性能测试指标")

	m := sim.PerformanceMetrics

	sim.Logger.Info("执行统计:")
	sim.Logger.Info("  总轮次: %d", m.TotalRounds)
	sim.Logger.Info("  总耗时: %v", m.TotalExecutionTime)
	sim.Logger.Info("  平均每轮: %v", m.AverageRoundTime)
	sim.Logger.Info("  最快一轮: %v", m.MinRoundTime)
	sim.Logger.Info("  最慢一轮: %v", m.MaxRoundTime)

	sim.Logger.Info("\n吞吐量:")
	sim.Logger.Info("  总区块数: %d", m.TotalBlocks)
	sim.Logger.Info("  总交易数: %d", m.TotalTransactions)
	sim.Logger.Info("  TPS: %.2f 笔/秒", m.TPS)

	sim.Logger.Info("\n网络状态:")
	sim.Logger.Info("  最终活跃节点: %d", sim.LocalView.Network.ActiveNodesCount)
	sim.Logger.Info("  最终总权重: %.2f", sim.LocalView.Network.TotalWeight)
	sim.Logger.Info("  平均权重: %.2f", sim.LocalView.Network.AverageWeight)
}

// ================================
// 最终报告
// ================================

// GenerateFinalReport 生成最终报告
func (sim *Simulator) GenerateFinalReport() {
	sim.Logger.LogSection("模拟结束 - 最终报告")

	sim.analyzeWeightEvolution()
	sim.analyzeNodePerformance()
	sim.analyzeVotingBehavior()
	sim.analyzeBlockProduction()
	sim.printTopNodes(10)
}

func (sim *Simulator) analyzeWeightEvolution() {
	sim.Logger.Info("\n【权重演化分析】")

	snapshots := sim.LocalView.Network.SnapshotHistory
	if len(snapshots) == 0 {
		return
	}

	initialWeight := snapshots[0].TotalWeight
	finalWeight := snapshots[len(snapshots)-1].TotalWeight
	decayRate := (initialWeight - finalWeight) / initialWeight

	sim.Logger.Info("  初始总权重: %.2f", initialWeight)
	sim.Logger.Info("  最终总权重: %.2f", finalWeight)
	sim.Logger.Info("  总衰减率: %.2f%%", decayRate*100)
	sim.Logger.Info("  平均每轮衰减: %.2f%%", decayRate/float64(len(snapshots))*100)
}

func (sim *Simulator) analyzeNodePerformance() {
	sim.Logger.Info("\n【节点表现分布】")

	performances := make([]float64, 0)
	excellentCount := 0
	goodCount := 0
	averageCount := 0
	poorCount := 0

	for _, node := range sim.LocalView.Network.Nodes {
		if !node.IsActive {
			continue
		}

		performances = append(performances, node.PerformanceScore)

		switch {
		case node.PerformanceScore >= 0.9:
			excellentCount++
		case node.PerformanceScore >= 0.7:
			goodCount++
		case node.PerformanceScore >= 0.5:
			averageCount++
		default:
			poorCount++
		}
	}

	sim.Logger.Info("  活跃节点数: %d", len(performances))
	sim.Logger.Info("  平均表现: %.3f", utils.Mean(performances))
	sim.Logger.Info("  中位数: %.3f", utils.Median(performances))
	sim.Logger.Info("  标准差: %.3f", utils.StandardDeviation(performances))
	sim.Logger.Info("  优秀节点(R≥0.9): %d", excellentCount)
	sim.Logger.Info("  良好节点(0.7≤R<0.9): %d", goodCount)
	sim.Logger.Info("  一般节点(0.5≤R<0.7): %d", averageCount)
	sim.Logger.Info("  差劲节点(R<0.5): %d", poorCount)
}

func (sim *Simulator) analyzeVotingBehavior() {
	sim.Logger.Info("\n【投票行为分析】")

	totalRounds := len(sim.RoundSummaries)
	if totalRounds == 0 {
		return
	}

	totalVoters := 0
	totalSuccessful := 0
	totalProxied := 0
	totalFailed := 0
	avgParticipation := 0.0

	for _, summary := range sim.RoundSummaries {
		totalVoters += summary.VotingStats.TotalVoters
		totalSuccessful += summary.VotingStats.SuccessfulVoters
		totalProxied += summary.VotingStats.ProxiedVoters
		totalFailed += summary.VotingStats.FailedVoters
		avgParticipation += summary.VotingStats.ParticipationRate
	}

	avgParticipation /= float64(totalRounds)

	sim.Logger.Info("  总轮次: %d", totalRounds)
	sim.Logger.Info("  平均参与率: %.1f%%", avgParticipation*100)
	sim.Logger.Info("  成功率: %.1f%%", float64(totalSuccessful)/float64(totalVoters)*100)
	sim.Logger.Info("  代投率: %.1f%%", float64(totalProxied)/float64(totalVoters)*100)
	sim.Logger.Info("  失败率: %.1f%%", float64(totalFailed)/float64(totalVoters)*100)
}

func (sim *Simulator) analyzeBlockProduction() {
	sim.Logger.Info("\n【出块效率分析】")

	totalBlocks := 0
	totalValid := 0
	totalDelayed := 0
	totalInvalid := 0
	avgSuccessRate := 0.0

	for _, summary := range sim.RoundSummaries {
		totalBlocks += summary.BlockStats.TotalBlocks
		totalValid += summary.BlockStats.ValidBlocks
		totalDelayed += summary.BlockStats.DelayedBlocks
		totalInvalid += summary.BlockStats.InvalidBlocks
		avgSuccessRate += summary.BlockStats.SuccessRate
	}

	rounds := len(sim.RoundSummaries)
	if rounds > 0 {
		avgSuccessRate /= float64(rounds)
	}

	sim.Logger.Info("  总区块数: %d", totalBlocks)
	sim.Logger.Info("  平均成功率: %.1f%%", avgSuccessRate*100)
	sim.Logger.Info("  有效率: %.1f%%", float64(totalValid)/float64(totalBlocks)*100)
	sim.Logger.Info("  延迟率: %.1f%%", float64(totalDelayed)/float64(totalBlocks)*100)
	sim.Logger.Info("  无效率: %.1f%%", float64(totalInvalid)/float64(totalBlocks)*100)
}

func (sim *Simulator) printTopNodes(topN int) {
	sim.Logger.Info("\n【顶尖节点排行榜】")

	activeNodes := make([]*types.Node, 0)
	for _, node := range sim.LocalView.Network.Nodes {
		if node.IsActive {
			activeNodes = append(activeNodes, node)
		}
	}

	// 排序
	for i := 0; i < len(activeNodes); i++ {
		for j := i + 1; j < len(activeNodes); j++ {
			if activeNodes[i].CurrentWeight < activeNodes[j].CurrentWeight {
				activeNodes[i], activeNodes[j] = activeNodes[j], activeNodes[i]
			}
		}
	}

	count := topN
	if len(activeNodes) < count {
		count = len(activeNodes)
	}

	sim.Logger.Info("  排名  节点ID      当前权重    初始权重    表现评分    半衰期    总奖励")
	sim.Logger.Info("  ----  --------  ----------  ----------  ----------  --------  --------")

	for i := 0; i < count; i++ {
		node := activeNodes[i]
		halfLife := node.GetHalfLife()

		sim.Logger.Info("  %-4d  %-8s  %10.2f  %10.2f  %10.3f  %8.1f  %8.2f",
			i+1,
			node.ID,
			node.CurrentWeight,
			node.InitialWeight,
			node.PerformanceScore,
			halfLife,
			node.TotalReward,
		)
	}
}
