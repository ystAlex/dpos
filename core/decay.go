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
//   - timePeriods: 从上次重置后经过的周期数
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
