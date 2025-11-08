package core

import (
	"nm-dpos/config"
	"nm-dpos/types"
	"nm-dpos/utils"
)

// decay.go
// 第2章：权重衰减机制实现
// 基于牛顿冷却定律的权重动态衰减算法

// ================================
// 第2.1节：衰减公式推导
// ================================

// UpdateNodeWeight 更新单个节点的权重
// 实现牛顿冷却公式：W_i(t) = W_env + (W_i(0) - W_env) * e^[-c*(1-R_i)*t]
// 简化版（W_env=0）：W_i(t) = W_i(0) * e^[-c*(1-R_i)*t]
//
// 参数：
//   - node: 待更新的节点
//   - timePeriods: 经过的时间周期数
//   - envWeight: 环境权重 W_env（本方案设为0）
//
// 返回：更新后的权重值
func UpdateNodeWeight(node *types.Node, timePeriods, envWeight float64) float64 {
	// 计算衰减率 λ = c * (1 - R_i)
	decayRate := CalculateDecayRate(node.PerformanceScore)

	// 应用牛顿冷却公式
	newWeight := utils.NewtonCooling(
		node.InitialWeight,
		envWeight,
		decayRate,
		timePeriods,
	)

	// 更新节点权重
	node.CurrentWeight = newWeight

	// 检查是否需要标记为非活跃
	if newWeight < config.MinActiveWeight {
		node.IsActive = false
	}

	return newWeight
}

// CalculateDecayRate 计算节点衰减率
// 公式：λ = c * (1 - R_i)
//
// 参数：
//   - performanceScore: 节点表现评分 R_i ∈ [0, 1]
//
// 返回：衰减率 λ
//
// 特性分析：
//   - R_i = 1（完美表现）：λ = 0，无衰减
//   - R_i = 0.95（优秀）：λ = 0.01，极慢衰减
//   - R_i = 0.70（良好）：λ = 0.06，中等衰减
//   - R_i = 0.30（差劲）：λ = 0.14，快速衰减
//   - R_i = 0（最差）：λ = 0.2，最快衰减
func CalculateDecayRate(performanceScore float64) float64 {
	return config.DecayConstant * (1.0 - performanceScore)
}

// ================================
// 第2.2节：衰减速度分析
// ================================

// GetDecayFactor 获取单周期衰减因子
// 公式：e^(-λ) = e^[-c*(1-R_i)]
//
// 参数：
//   - performanceScore: 节点表现评分 R_i
//
// 返回：单周期后的权重保留比例
//
// 示例：
//   - R=0.95：返回0.990（保留99.0%，损失1.0%）
//   - R=0.70：返回0.942（保留94.2%，损失5.8%）
//   - R=0.30：返回0.869（保留86.9%，损失13.1%）
func GetDecayFactor(performanceScore float64) float64 {
	decayRate := CalculateDecayRate(performanceScore)
	return utils.ExponentialDecay(decayRate, 1.0)
}

// ================================
// 第2.3节：半衰期计算
// ================================

// CalculateHalfLife 计算节点权重半衰期
// 公式：t_1/2 = ln(2) / λ = ln(2) / [c * (1 - R_i)]
//
// 参数：
//   - performanceScore: 节点表现评分 R_i
//
// 返回：半衰期（周期数）
//
// 物理意义：权重衰减到初始值一半所需的时间
//
// 典型值（c=0.2）：
//   - R=0.95：69.3周期
//   - R=0.70：11.6周期
//   - R=0.30：4.95周期
func CalculateHalfLife(performanceScore float64) float64 {
	decayRate := CalculateDecayRate(performanceScore)
	return utils.CalculateHalfLife(decayRate)
}

// ================================
// 第2.4节：批量权重更新
// ================================

// UpdateAllNodesWeight 更新所有节点的权重
// 批量应用牛顿冷却公式
//
// 参数：
//   - nodes: 节点列表
//   - timePeriods: 经过的时间周期数
//   - envWeight: 环境权重 W_env
//
// 返回：(活跃节点数, 总权重)
func UpdateAllNodesWeight(nodes []*types.Node, timePeriods, envWeight float64) (int, float64) {
	activeCount := 0
	totalWeight := 0.0

	for _, node := range nodes {
		if !node.IsActive {
			continue
		}

		// 更新权重
		newWeight := UpdateNodeWeight(node, timePeriods, envWeight)

		if node.IsActive {
			activeCount++
			totalWeight += newWeight
		}
	}

	return activeCount, totalWeight
}

// ================================
// 第2.5节：公平性验证
// ================================

// VerifyFairness 验证衰减机制的公平性
// 检查相同表现评分的节点是否具有相同的衰减率
//
// 参数：
//   - node1, node2: 待比较的两个节点
//
// 返回：(是否公平, 衰减率差异)
//
// 公平性定义：
//
//	若 R_i = R_j，则 W_i(t)/W_i(0) = W_j(t)/W_j(0)
func VerifyFairness(node1, node2 *types.Node) (bool, float64) {
	// 如果表现评分不同，不适用公平性检验
	if node1.PerformanceScore != node2.PerformanceScore {
		return false, -1
	}

	// 计算各自的相对权重（当前/初始）
	ratio1 := node1.CurrentWeight / node1.InitialWeight
	ratio2 := node2.CurrentWeight / node2.InitialWeight

	// 计算差异
	difference := ratio1 - ratio2

	// 允许微小的浮点误差（1e-6）
	isFair := (difference < 1e-6 && difference > -1e-6)

	return isFair, difference
}

// ================================
// 辅助函数
// ================================

// PredictWeightAfterPeriods 预测未来权重
// 不修改节点状态，仅用于模拟和预测
//
// 参数：
//   - initialWeight: 初始权重 W_i(0)
//   - performanceScore: 表现评分 R_i
//   - periods: 未来的时间周期数
//   - envWeight: 环境权重 W_env
//
// 返回：预测的未来权重
func PredictWeightAfterPeriods(initialWeight, performanceScore float64, periods int, envWeight float64) float64 {
	decayRate := CalculateDecayRate(performanceScore)
	return utils.NewtonCooling(initialWeight, envWeight, decayRate, float64(periods))
}

// CalculateRequiredPerformance 计算维持权重所需的表现评分
// 反向计算：给定目标权重，计算需要的R值
//
// 参数：
//   - initialWeight: 初始权重
//   - targetWeight: 目标权重
//   - periods: 时间周期数
//   - envWeight: 环境权重
//
// 返回：所需的表现评分 R_i
//
// 公式推导：
//
//	W(t) = W_env + (W_0 - W_env) * e^[-c*(1-R)*t]
//	(W(t) - W_env) / (W_0 - W_env) = e^[-c*(1-R)*t]
//	ln[(W(t) - W_env) / (W_0 - W_env)] = -c*(1-R)*t
//	R = 1 - ln[(W(t) - W_env) / (W_0 - W_env)] / (-c*t)
func CalculateRequiredPerformance(initialWeight, targetWeight float64, periods int, envWeight float64) float64 {
	if periods == 0 || initialWeight == envWeight {
		return 1.0
	}

	ratio := (targetWeight - envWeight) / (initialWeight - envWeight)
	if ratio <= 0 {
		return 0.0 // 无法达到
	}

	lnRatio := utils.Log(ratio)
	required := 1.0 - lnRatio/(-config.DecayConstant*float64(periods))

	// 限制在[0, 1]范围内
	return utils.ClampScore(required)
}

// GetWeightEvolution 获取权重演化序列
// 用于分析和可视化权重变化趋势
//
// 参数：
//   - initialWeight: 初始权重
//   - performanceScore: 表现评分
//   - maxPeriods: 最大周期数
//   - envWeight: 环境权重
//
// 返回：权重演化序列（每个周期的权重值）
func GetWeightEvolution(initialWeight, performanceScore float64, maxPeriods int, envWeight float64) []float64 {
	evolution := make([]float64, maxPeriods+1)
	decayRate := CalculateDecayRate(performanceScore)

	for t := 0; t <= maxPeriods; t++ {
		evolution[t] = utils.NewtonCooling(initialWeight, envWeight, decayRate, float64(t))
	}

	return evolution
}
