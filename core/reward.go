package core

import (
	"nm-dpos/config"
	"nm-dpos/types"
	"nm-dpos/utils"
)

// reward.go
// 第5章：权重补偿与奖励分配实现
// 包含热补偿、Shapley值分配、保证金管理

// ================================
// 第5.2节：热补偿机制
// ================================

// DistributeHotCompensation 分配热补偿
// 将失败节点流出的权重分配给成功投票的节点
// 公式：S_comp_i = (W_i / Σ W_k) * Pool_hot
//
// 参数：
//   - session: 投票会话
//   - successfulVoters: 成功投票的节点（包括被代投的）
//
// 返回：总补偿金额
func DistributeHotCompensation(session *VotingSession, successfulVoters []*types.Node) float64 {
	// 计算补偿资金池
	// Pool_hot = 扣除的权重 + 没收的保证金
	poolHot := session.TotalDeductedWeight + session.TotalConfiscatedDeposit

	if poolHot <= 0 || len(successfulVoters) == 0 {
		return 0
	}

	// 收集成功投票节点的权重
	weights := make([]float64, len(successfulVoters))
	for i, voter := range successfulVoters {
		weights[i] = voter.CurrentWeight
	}

	// 按权重比例分配
	compensations := utils.DistributeReward(poolHot, weights)

	// 分发补偿
	for i, voter := range successfulVoters {
		compensation := compensations[i]
		voter.CurrentWeight += compensation
		voter.TotalCompensation += compensation

		// 更新最近一次投票记录的补偿字段
		if len(voter.VoteHistory) > 0 {
			lastVote := &voter.VoteHistory[len(voter.VoteHistory)-1]
			lastVote.Compensation = compensation
		}
	}

	session.HotCompensationPool = poolHot
	return poolHot
}

// ================================
// 第5.3节：修正Shapley值分配
// ================================

// DistributeBlockReward 分配区块奖励
// 基于修正Shapley值：φ'_j = φ_j + v(T) * ΔR_j
//
// 参数：
//   - blockReward: 区块奖励总额
//   - participants: 参与节点（出块者+验证者）
//
// 返回：每个节点的奖励金额
func DistributeBlockReward(blockReward float64, participants []*types.Node) map[string]float64 {
	if len(participants) == 0 {
		return make(map[string]float64)
	}

	// 计算平均表现评分
	totalPerformance := 0.0
	for _, node := range participants {
		totalPerformance += node.PerformanceScore
	}
	avgPerformance := totalPerformance / float64(len(participants))

	// 计算基础Shapley值（简化：平均分配）
	baseShapley := utils.ShapleyValue(blockReward, len(participants))

	// 计算修正Shapley值并分配
	rewards := make(map[string]float64)
	totalModified := 0.0

	for _, node := range participants {
		modified := utils.ModifiedShapleyValue(
			baseShapley,
			blockReward,
			node.PerformanceScore,
			avgPerformance,
		)

		// 确保非负
		if modified < 0 {
			modified = 0
		}

		rewards[node.ID] = modified
		totalModified += modified
	}

	// 归一化（确保总和等于blockReward）
	if totalModified > 0 {
		normalizationFactor := blockReward / totalModified
		for nodeID, reward := range rewards {
			rewards[nodeID] = reward * normalizationFactor
		}
	}

	return rewards
}

// ApplyBlockRewards 应用区块奖励
// 将计算出的奖励分配给节点
//
// 参数：
//   - rewards: 奖励映射（节点ID -> 奖励金额）
//   - nodes: 节点映射（节点ID -> 节点对象）
func ApplyBlockRewards(rewards map[string]float64, nodes map[string]*types.Node) {
	for nodeID, reward := range rewards {
		if node, exists := nodes[nodeID]; exists {
			node.CurrentWeight += reward
			node.TotalReward += reward
		}
	}
}

// ================================
// 第5.4节：保证金机制
// ================================

// LockDeposit 锁定节点保证金
// 在投票开始时调用
//
// 参数：
//   - node: 节点
//
// 返回：锁定的保证金金额
func LockDeposit(node *types.Node) float64 {
	// 计算保证金（第5.4节）
	deposit := node.CurrentWeight * config.DepositRatio
	node.Deposit = deposit
	return deposit
}

// ReleaseDeposit 释放保证金
// 第5.1节：情况B和C（投票成功）
//
// 参数：
//   - node: 节点
//
// 返回：释放的保证金金额
func ReleaseDeposit(node *types.Node) float64 {
	deposit := node.Deposit
	// 保证金不返还权重，仅解锁
	node.Deposit = 0
	return deposit
}

// ConfiscateDeposit 没收保证金
// 第5.1节：情况A和D（投票失败或出块失败）
//
// 参数：
//   - node: 节点
//   - ratio: 没收比例（0~1）
//
// 返回：没收的保证金金额
func ConfiscateDeposit(node *types.Node, ratio float64) float64 {
	confiscated := node.Deposit * ratio
	node.Deposit -= confiscated
	return confiscated
}

// ================================
// 第5.1节：四种投票结果处理
// ================================

// HandleVoteResultA 处理情况A：未投票
// 惩罚：扣除权重 + 没收保证金
func HandleVoteResultA(node *types.Node) float64 {
	// 扣除权重
	deductedWeight := node.CurrentWeight * config.VoteFailurePenalty
	node.CurrentWeight -= deductedWeight

	// 没收全部保证金
	confiscated := ConfiscateDeposit(node, 1.0)

	return deductedWeight + confiscated
}

// HandleVoteResultB 处理情况B：投票但候选节点未当选
// 结果：退回保证金
func HandleVoteResultB(node *types.Node) float64 {
	return ReleaseDeposit(node)
}

// HandleVoteResultC 处理情况C：投票且代理节点成功出块
// 结果：退回保证金 + 热补偿
// 热补偿在 DistributeHotCompensation 函数中处理
func HandleVoteResultC(node *types.Node, compensation float64) float64 {
	deposit := ReleaseDeposit(node)
	node.CurrentWeight += compensation
	node.TotalCompensation += compensation
	return deposit + compensation
}

// HandleVoteResultD 处理情况D：投票但代理节点未出块
// 结果：部分扣除保证金
func HandleVoteResultD(node *types.Node) float64 {
	// 扣除部分保证金（30%）
	confiscated := ConfiscateDeposit(node, config.BlockFailurePenalty)

	// 退回剩余保证金
	released := ReleaseDeposit(node)

	return released - confiscated
}

// ================================
// 出块奖励相关
// ================================

// CalculateBlockProducerReward 计算出块者奖励
// 出块者获得固定奖励 + 80%交易手续费
//
// 参数：
//   - blockReward: 固定出块奖励
//   - transactionFees: 交易手续费总额
//
// 返回：出块者奖励
func CalculateBlockProducerReward(blockReward, transactionFees float64) float64 {
	return blockReward + transactionFees*config.TransactionFeeRatio
}

// CalculateValidatorReward 计算验证者奖励
// 验证者平分20%交易手续费
//
// 参数：
//   - transactionFees: 交易手续费总额
//   - numValidators: 验证者数量
//
// 返回：单个验证者奖励
func CalculateValidatorReward(transactionFees float64, numValidators int) float64 {
	if numValidators == 0 {
		return 0
	}
	validatorPool := transactionFees * (1.0 - config.TransactionFeeRatio)
	return validatorPool / float64(numValidators)
}

// DistributeBlockRewards 分配区块产生的所有奖励
// 包括出块奖励、交易手续费、验证奖励
//
// 参数：
//   - blockProducer: 出块节点
//   - validators: 验证节点列表
//   - transactionFees: 交易手续费总额
//
// 返回：奖励分配详情
func DistributeBlockRewards(
	blockProducer *types.Node,
	validators []*types.Node,
	transactionFees float64,
) map[string]float64 {
	rewards := make(map[string]float64)

	// 1. 出块者奖励
	producerReward := CalculateBlockProducerReward(config.BlockReward, transactionFees)
	blockProducer.CurrentWeight += producerReward
	blockProducer.TotalReward += producerReward
	rewards[blockProducer.ID] = producerReward

	// 2. 验证者奖励
	validatorReward := CalculateValidatorReward(transactionFees, len(validators))
	for _, validator := range validators {
		validator.CurrentWeight += validatorReward
		validator.TotalReward += validatorReward
		rewards[validator.ID] = validatorReward
	}

	return rewards
}

// ================================
// 统计与报告
// ================================

// CalculateTotalRewardPool 计算总奖励池
// 包括区块奖励、交易手续费、热补偿
func CalculateTotalRewardPool(numBlocks int, avgTransactionFees, hotCompensation float64) float64 {
	blockRewards := float64(numBlocks) * config.BlockReward
	transactionFeesTotal := avgTransactionFees * float64(numBlocks)
	return blockRewards + transactionFeesTotal + hotCompensation
}

// GetRewardDistributionSummary 获取奖励分配摘要
// 统计一轮中所有节点的奖励情况
func GetRewardDistributionSummary(nodes []*types.Node) map[string]interface{} {
	totalReward := 0.0
	totalCompensation := 0.0
	maxReward := 0.0
	minReward := 999999.0

	for _, node := range nodes {
		totalReward += node.TotalReward
		totalCompensation += node.TotalCompensation

		if node.TotalReward > maxReward {
			maxReward = node.TotalReward
		}
		if node.TotalReward < minReward && node.TotalReward > 0 {
			minReward = node.TotalReward
		}
	}

	avgReward := 0.0
	if len(nodes) > 0 {
		avgReward = totalReward / float64(len(nodes))
	}

	return map[string]interface{}{
		"total_reward":       totalReward,
		"total_compensation": totalCompensation,
		"average_reward":     avgReward,
		"max_reward":         maxReward,
		"min_reward":         minReward,
		"num_rewarded_nodes": len(nodes),
	}
}
