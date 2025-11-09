package test

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"nm-dpos/config"
	"nm-dpos/node"
	"nm-dpos/utils"
	"os"
	"path/filepath"
	"time"
)

// NetworkTester 网络测试器
type NetworkTester struct {
	Node      *node.P2PNode
	Logger    *utils.Logger
	OutputDir string
	Results   []TestResult
}

// TestResult 测试结果
type TestResult struct {
	Round               int
	Timestamp           time.Time
	NodeID              string
	NodeType            string
	CurrentWeight       float64
	PerformanceScore    float64
	HalfLife            float64
	SuccessfulVotes     int
	FailedVotes         int
	ValidBlocks         int
	TotalReward         float64
	TotalCompensation   float64
	NetworkTotalWeight  float64
	NetworkActiveNodes  int
	VotingParticipation float64
}

// NewNetworkTester 创建测试器
func NewNetworkTester(p2pNode *node.P2PNode, logger *utils.Logger, outputDir string) *NetworkTester {
	return &NetworkTester{
		Node:      p2pNode,
		Logger:    logger,
		OutputDir: outputDir,
		Results:   make([]TestResult, 0),
	}
}

// RunFullTest 运行完整测试
func (nt *NetworkTester) RunFullTest(rounds int) {
	nt.Logger.Info("========================================")
	nt.Logger.Info("开始网络测试")
	nt.Logger.Info("测试轮数: %d", rounds)
	nt.Logger.Info("========================================")

	// 创建输出目录
	if err := os.MkdirAll(nt.OutputDir, 0755); err != nil {
		nt.Logger.Error("创建输出目录失败: %v", err)
		return
	}

	// 记录初始状态
	nt.recordRound(0)

	// 运行测试轮次
	for round := 1; round <= rounds; round++ {
		nt.Logger.Info("--- 第 %d 轮 ---", round)

		// 等待一个区块周期
		time.Sleep(time.Duration(config.BlockInterval) * time.Second)

		// 记录当前轮次数据
		nt.recordRound(round)

		// 每10轮输出一次进度
		if round%10 == 0 {
			nt.Logger.Info("已完成 %d/%d 轮", round, rounds)
			nt.printCurrentStatus()
		}
	}

	// 保存结果
	nt.saveResults()

	// 生成分析报告
	nt.generateReport()

	nt.Logger.Info("========================================")
	nt.Logger.Info("测试完成")
	nt.Logger.Info("========================================")
}

// recordRound 记录轮次数据
func (nt *NetworkTester) recordRound(round int) {
	localNode := nt.Node.LocalView.LocalNode
	snapshot := nt.Node.LocalView.GetNetworkSnapshot()

	result := TestResult{
		Round:               round,
		Timestamp:           time.Now(),
		NodeID:              localNode.ID,
		NodeType:            localNode.Type.String(),
		CurrentWeight:       localNode.CurrentWeight,
		PerformanceScore:    localNode.PerformanceScore,
		HalfLife:            localNode.GetHalfLife(),
		SuccessfulVotes:     localNode.SuccessfulVotes,
		FailedVotes:         localNode.FailedVotes,
		ValidBlocks:         localNode.ValidBlocks,
		TotalReward:         localNode.TotalReward,
		TotalCompensation:   localNode.TotalCompensation,
		NetworkTotalWeight:  snapshot.TotalWeight,
		NetworkActiveNodes:  snapshot.ActiveNodes,
		VotingParticipation: snapshot.VotingParticipation,
	}

	nt.Results = append(nt.Results, result)
}

// printCurrentStatus 打印当前状态
func (nt *NetworkTester) printCurrentStatus() {
	if len(nt.Results) == 0 {
		return
	}

	latest := nt.Results[len(nt.Results)-1]

	nt.Logger.Info("当前状态:")
	nt.Logger.Info("  权重: %.2f (半衰期: %.1f)", latest.CurrentWeight, latest.HalfLife)
	nt.Logger.Info("  表现: %.3f", latest.PerformanceScore)
	nt.Logger.Info("  投票: 成功=%d, 失败=%d", latest.SuccessfulVotes, latest.FailedVotes)
	nt.Logger.Info("  奖励: %.2f, 补偿: %.2f", latest.TotalReward, latest.TotalCompensation)
}

// saveResults 保存结果到CSV
func (nt *NetworkTester) saveResults() {
	filename := filepath.Join(nt.OutputDir, fmt.Sprintf("%s_results_%d.csv",
		nt.Node.LocalView.LocalNode.ID, time.Now().Unix()))

	file, err := os.Create(filename)
	if err != nil {
		nt.Logger.Error("创建结果文件失败: %v", err)
		return
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// 写入表头
	header := []string{
		"Round", "Timestamp", "NodeID", "NodeType",
		"CurrentWeight", "PerformanceScore", "HalfLife",
		"SuccessfulVotes", "FailedVotes", "ValidBlocks",
		"TotalReward", "TotalCompensation",
		"NetworkTotalWeight", "NetworkActiveNodes", "VotingParticipation",
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
			fmt.Sprintf("%.2f", result.TotalReward),
			fmt.Sprintf("%.2f", result.TotalCompensation),
			fmt.Sprintf("%.2f", result.NetworkTotalWeight),
			fmt.Sprintf("%d", result.NetworkActiveNodes),
			fmt.Sprintf("%.4f", result.VotingParticipation),
		}
		writer.Write(row)
	}

	nt.Logger.Info("结果已保存到: %s", filename)
}

// generateReport 生成分析报告
func (nt *NetworkTester) generateReport() {
	filename := filepath.Join(nt.OutputDir, fmt.Sprintf("%s_report_%d.json",
		nt.Node.LocalView.LocalNode.ID, time.Now().Unix()))

	report := nt.analyzeResults()

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		nt.Logger.Error("生成报告失败: %v", err)
		return
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		nt.Logger.Error("保存报告失败: %v", err)
		return
	}

	nt.Logger.Info("报告已保存到: %s", filename)
}

// analyzeResults 分析结果
func (nt *NetworkTester) analyzeResults() map[string]interface{} {
	if len(nt.Results) == 0 {
		return map[string]interface{}{"error": "no data"}
	}

	first := nt.Results[0]
	last := nt.Results[len(nt.Results)-1]

	weightChange := last.CurrentWeight - first.CurrentWeight
	weightChangePercent := (weightChange / first.CurrentWeight) * 100

	totalVotes := last.SuccessfulVotes + last.FailedVotes
	voteSuccessRate := 0.0
	if totalVotes > 0 {
		voteSuccessRate = float64(last.SuccessfulVotes) / float64(totalVotes)
	}

	return map[string]interface{}{
		"summary": map[string]interface{}{
			"node_id":       nt.Node.LocalView.LocalNode.ID,
			"node_type":     last.NodeType,
			"total_rounds":  len(nt.Results),
			"test_duration": last.Timestamp.Sub(first.Timestamp).String(),
		},
		"weight_analysis": map[string]interface{}{
			"initial_weight":        first.CurrentWeight,
			"final_weight":          last.CurrentWeight,
			"weight_change":         weightChange,
			"weight_change_percent": weightChangePercent,
			"final_half_life":       last.HalfLife,
		},
		"performance_analysis": map[string]interface{}{
			"initial_performance": first.PerformanceScore,
			"final_performance":   last.PerformanceScore,
			"performance_change":  last.PerformanceScore - first.PerformanceScore,
		},
		"voting_analysis": map[string]interface{}{
			"total_votes":      totalVotes,
			"successful_votes": last.SuccessfulVotes,
			"failed_votes":     last.FailedVotes,
			"success_rate":     voteSuccessRate,
		},
		"reward_analysis": map[string]interface{}{
			"total_reward":       last.TotalReward,
			"total_compensation": last.TotalCompensation,
			"total_earnings":     last.TotalReward + last.TotalCompensation,
		},
		"block_analysis": map[string]interface{}{
			"valid_blocks": last.ValidBlocks,
		},
	}
}
