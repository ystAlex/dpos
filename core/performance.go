package core

import (
	"nm-dpos/config"
	"nm-dpos/types"
	"nm-dpos/utils"
	"time"
)

// performance.go
// 第3章：节点表现评估体系实现
// 包含投票节点和代理节点的评分算法

// ================================
// 第3.1节：投票节点评估
// ================================

// UpdateVoterPerformance 更新投票节点的综合表现评分
// 公式：R_i = α*V_i + β*B_i + γ*E_j
//
// 参数：
//   - node: 投票节点
//   - votingThreshold: 投票时间阈值（正常投票区段上限）
//   - delegatePerformance: 所投代理节点的表现评分 E_j
//
// 返回：更新后的表现评分 R_i ∈ [0, 1]
func UpdateVoterPerformance(node *types.Node, votingThreshold, delegatePerformance float64) float64 {
	// 计算各项指标
	votingSpeed := CalculateVotingSpeed(node, votingThreshold)
	votingBehavior := CalculateVotingBehavior(node)

	// 更新节点内部状态
	node.VotingSpeed = votingSpeed
	node.VotingBehavior = votingBehavior

	// 加权综合评分
	scores := []float64{votingSpeed, votingBehavior, delegatePerformance}
	weights := []float64{config.AlphaWeight, config.BetaWeight, config.GammaWeight}

	performance := utils.WeightedScore(scores, weights)

	//限制在[0,1]范围内
	node.PerformanceScore = utils.ClampScore(performance)

	return node.PerformanceScore
}

// ================================
// 第3.1.1节：平均投票速率 V_i
// ================================

// CalculateVotingSpeed 计算平均投票速率
// 公式：V_i = Σ(T_threshold - t_vote_i_j) / (n * T_threshold)
//
// 参数：
//   - node: 投票节点
//   - threshold: 投票时间阈值 T_threshold（毫秒）
//
// 返回：投票速率评分 V_i ∈ [0, 1]
//
// 评分规则：
//   - 投票越快，得分越高
//   - 超时投票（t_vote > threshold）记为0分
//   - 平均所有投票的标准化得分
func CalculateVotingSpeed(node *types.Node, threshold float64) float64 {
	if len(node.VoteHistory) == 0 {
		return 0.5 // 默认中等水平
	}

	totalScore := 0.0
	validCount := 0

	for _, record := range node.VoteHistory {
		// 跳过代投记录（不反映自身速度）
		if record.IsProxy {
			continue
		}

		// 计算时间节省量
		timeSaved := threshold - record.TimeCost
		if timeSaved < 0 {
			timeSaved = 0 // 超时记为0分
		}

		// 标准化到[0, 1]
		normalizedScore := timeSaved / threshold
		totalScore += normalizedScore
		validCount++
	}

	if validCount == 0 {
		return 0.5
	}

	return totalScore / float64(validCount)
}

// ================================
// 第3.1.2节：历史投票行为 B_i
// ================================

// CalculateVotingBehavior 计算历史投票行为评分
// 公式：B_i = 1 / (1 + ln( (γ_i + β_i)/γ_i ))
//
// 参数：
//   - node: 投票节点
//
// 返回：行为评分 B_i ∈ [0, 1]
//
// 变量说明：
//   - γ_i: 成功投票次数（node.SuccessfulVotes）
//   - β_i: 失败投票次数（node.FailedVotes）
//
// 特殊情况：
//   - γ = 0：返回0（从未成功）
//   - β = 0：返回1（从未失败）
func CalculateVotingBehavior(node *types.Node) float64 {
	return utils.VotingBehaviorScore(node.SuccessfulVotes, node.FailedVotes)
}

// ================================
// 第3.2节：代理节点评估
// ================================

// UpdateDelegatePerformance 更新代理节点的综合表现评分
// 公式：E_j = α*P_j + β*C_j + γ*O_j
//
// 参数：
//   - node: 代理节点
//
// 返回：更新后的表现评分 E_j ∈ [0, 1]
func UpdateDelegatePerformance(node *types.Node) float64 {
	// 计算各项指标
	blockEfficiency := CalculateBlockEfficiency(node)
	validationEfficiency := CalculateValidationEfficiency(node)
	onlineScore := CalculateOnlineScore(node)

	// 更新节点内部状态
	node.BlockEfficiency = blockEfficiency
	node.ValidationEfficiency = validationEfficiency

	// 加权综合评分
	scores := []float64{blockEfficiency, validationEfficiency, onlineScore}
	weights := []float64{config.AlphaWeight, config.BetaWeight, config.GammaWeight}

	performance := utils.WeightedScore(scores, weights)
	node.PerformanceScore = utils.ClampScore(performance)

	return node.PerformanceScore
}

// ================================
// 第3.2.1节：出块效率 P_j
// ================================

// CalculateBlockEfficiency 计算出块效率
// 公式：P_j = (B_valid_j - B_delayed_j - B_invalid_j) / B_max_j
//
// 参数：
//   - node: 代理节点
//
// 返回：出块效率 P_j ∈ [0, 1]
//
// 评分标准：
//   - 有效区块（及时且正确）：+1分
//   - 延迟区块（超时但有效）：0分
//   - 无效区块（错误或作恶）：-1分
func CalculateBlockEfficiency(node *types.Node) float64 {
	totalBlocks := len(node.BlockHistory)
	if totalBlocks == 0 {
		return 0.5 // 默认中等水平
	}

	// 计算有效分数
	effectiveScore := node.ValidBlocks - node.DelayedBlocks - node.InvalidBlocks

	// 标准化到[0, 1]
	efficiency := float64(effectiveScore) / float64(totalBlocks)

	// 确保非负
	if efficiency < 0 {
		efficiency = 0
	}

	return efficiency
}

// ================================
// 第3.2.2节：区块验证效率 C_j
// ================================

// CalculateValidationEfficiency 计算验证效率
// 公式：C_j = B_correct_j / B_allocated_j
//
// 参数：
//   - node: 代理节点
//
// 返回：验证效率 C_j ∈ [0, 1]
//
// 验证规则：
//   - 正确接受有效区块：+1
//   - 正确拒绝无效区块：+1
//   - 错误接受无效区块：0
//   - 错误拒绝有效区块：0
func CalculateValidationEfficiency(node *types.Node) float64 {
	if node.TotalValidations == 0 {
		return 0.5 // 默认中等水平
	}

	return float64(node.CorrectValidations) / float64(node.TotalValidations)
}

// ================================
// 第3.2.3节：在线时长评分 O_j
// ================================

// CalculateOnlineScore 计算在线时长评分
// 公式：O_j = 1 / (1 + e^(-t_j/τ))
//
// 参数：
//   - node: 节点
//
// 返回：在线评分 O_j ∈ (0, 1)
//
// 特性：
//   - Sigmoid函数，缓慢增长后趋于稳定
//   - τ=100周期时：
//   - t=0: O=0.50
//   - t=100: O=0.73
//   - t=300: O=0.95
//   - t=500: O=0.99
func CalculateOnlineScore(node *types.Node) float64 {
	return utils.SigmoidFunction(node.OnlineTime, config.OnlineTimeTau)
}

// ================================
// 第3.3节：连任惩罚机制
// ================================

// CalculateRequiredDelegatedWeight 计算连任所需委托权重
// 公式：G_j(r_t) = G_j * (2 - E_j)^r_t
//
// 参数：
//   - node: 代理节点
//
// 返回：所需委托权重
//
// 设计原理：
//   - 优秀节点（E_j→1）：增长因子→1，几乎无惩罚
//   - 一般节点（E_j=0.7）：增长因子=1.3，温和增长
//   - 差劲节点（E_j→0）：增长因子→2，指数爆炸
func CalculateRequiredDelegatedWeight(node *types.Node) float64 {
	if node.Type != types.DelegateNode {
		return 0
	}

	baseWeight := node.InitialWeight
	performance := node.PerformanceScore
	terms := node.ConsecutiveTerms

	return utils.RequiredDelegatedWeight(baseWeight, performance, terms)
}

// CanBeReelected 检查代理节点是否可以连任
// 比较实际委托权重和所需委托权重
//
// 参数：
//   - node: 代理节点
//
// 返回：(是否可连任, 权重缺口)
func CanBeReelected(node *types.Node) (bool, float64) {
	requiredWeight := CalculateRequiredDelegatedWeight(node)
	actualWeight := node.DelegatedWeight

	gap := requiredWeight - actualWeight
	canReelect := gap <= 0

	return canReelect, gap
}

// ================================
// 辅助函数
// ================================

// RecordVoteSuccess 记录成功投票
// 更新节点的投票统计和历史记录
func RecordVoteSuccess(node *types.Node, votedFor string, timeCost float64) {
	record := types.VoteRecord{
		Timestamp: node.LastUpdateTime,
		VotedFor:  votedFor,
		Weight:    node.CurrentWeight,
		TimeCost:  timeCost,
		Success:   true,
		IsProxy:   false,
	}

	node.VoteHistory = append(node.VoteHistory, record)
	node.SuccessfulVotes++
}

// RecordVoteFailure 记录失败投票
func RecordVoteFailure(node *types.Node) {
	record := types.VoteRecord{
		Timestamp: node.LastUpdateTime,
		Success:   false,
		Weight:    node.CurrentWeight,
	}

	node.VoteHistory = append(node.VoteHistory, record)
	node.FailedVotes++
}

// RecordBlockProduction 记录出块
// 更新代理节点的出块统计和历史记录
func RecordBlockProduction(node *types.Node, blockHeight int, success, delayed, invalid bool, productionTime time.Duration, count int) {
	record := types.BlockRecord{
		Timestamp:        node.LastUpdateTime,
		BlockHeight:      blockHeight,
		Success:          success,
		IsDelayed:        delayed,
		ProductionTime:   productionTime,
		IsInvalid:        invalid,
		TransactionCount: count,
	}

	node.BlockHistory = append(node.BlockHistory, record)

	if success && !delayed && !invalid {
		node.ValidBlocks++
	} else if delayed {
		node.DelayedBlocks++
	} else if invalid {
		node.InvalidBlocks++
	}
}

// UpdateOnlineTime 更新在线时长
// 每个周期调用一次
func UpdateOnlineTime(node *types.Node) {
	if node.IsActive {
		node.OnlineTime++
	}
}

// GetPerformanceLevel 获取表现等级描述
func GetPerformanceLevel(score float64) string {
	switch {
	case score >= 0.9:
		return "优秀"
	case score >= 0.7:
		return "良好"
	case score >= 0.5:
		return "一般"
	case score >= 0.3:
		return "较差"
	default:
		return "差劲"
	}
}
