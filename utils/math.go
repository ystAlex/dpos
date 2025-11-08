package utils

import (
	"math"
	"math/rand"
	"nm-dpos/types"
	"sort"
	"time"
)

// math.go
// 数学工具函数
// 提供各种数学计算的辅助函数

// ================================
// 第2章：权重衰减 - 指数计算
// ================================

// ExponentialDecay 计算指数衰减因子
// 公式：e^(-λ*t)
// 参数：
//   - lambda: 衰减率 λ = c * (1 - R_i)
//   - timePeriods: 时间周期数 t
//
// 返回：衰减因子
func ExponentialDecay(lambda, timePeriods float64) float64 {
	return math.Exp(-lambda * timePeriods)
}

// CalculateHalfLife 计算半衰期
// 公式：t_1/2 = ln(2) / λ
// 参数：
//   - decayRate: 衰减率 λ
//
// 返回：半衰期（周期数）
func CalculateHalfLife(decayRate float64) float64 {
	if decayRate <= 0 {
		return math.Inf(1) // 无限大
	}
	return math.Log(2.0) / decayRate
}

// NewtonCooling 牛顿冷却公式计算
// 公式：W(t) = W_env + (W_0 - W_env) * e^(-λ*t)
// 参数：
//   - initialWeight: 初始权重 W_0
//   - envWeight: 环境权重 W_env
//   - decayRate: 衰减率 λ
//   - timePeriods: 时间周期 t
//
// 返回：当前权重 W(t)
func NewtonCooling(initialWeight, envWeight, decayRate, timePeriods float64) float64 {
	exponentialFactor := ExponentialDecay(decayRate, timePeriods)
	return envWeight + (initialWeight-envWeight)*exponentialFactor
}

// ================================
// 第3章：节点表现评估 - 评分计算
// ================================

// SigmoidFunction Sigmoid函数
// 公式：f(x) = 1 / (1 + e^(-x/τ))
// 用于在线时长评分 O_j
// 参数：
//   - x: 输入值
//   - tau: 时间常数 τ
//
// 返回：sigmoid值 [0, 1]
func SigmoidFunction(x, tau float64) float64 {
	return 1.0 / (1.0 + math.Exp(-x/tau))
}

// VotingBehaviorScore 计算历史投票行为评分
// 公式：B_i = 1 / (1 + ln(γ/(γ+β)))
// 参数：
//   - successfulVotes: 成功投票次数 γ
//   - failedVotes: 失败投票次数 β
//
// 返回：行为评分 [0, 1]
func VotingBehaviorScore(successfulVotes, failedVotes int) float64 {
	gamma := float64(successfulVotes)
	beta := float64(failedVotes)

	// 特殊情况处理
	if gamma == 0 {
		return 0.0
	}
	if beta == 0 {
		return 1.0
	}

	// 计算成功率
	ratio := gamma / (gamma + beta)

	// 应用公式
	return 1.0 / (1.0 + math.Log(1.0/ratio))
}

// WeightedScore 计算加权综合评分
// 公式：Score = α*A + β*B + γ*C（α+β+γ=1）
// 参数：
//   - scores: 各项评分切片
//   - weights: 对应权重切片
//
// 返回：加权综合评分
func WeightedScore(scores, weights []float64) float64 {
	if len(scores) != len(weights) {
		panic("scores and weights must have same length")
	}

	total := 0.0
	for i := range scores {
		total += scores[i] * weights[i]
	}

	return total
}

// ClampScore 将评分限制在[0, 1]范围内
func ClampScore(score float64) float64 {
	if score < 0 {
		return 0
	}
	if score > 1 {
		return 1
	}
	return score
}

// ================================
// 第3.3章：连任惩罚 - 指数增长
// ================================

// RequiredDelegatedWeight 计算连任所需委托权重
// 公式：G_j(r_t) = G_j * (2 - E_j)^r_t
// 参数：
//   - baseWeight: 基础权重 G_j
//   - performance: 表现评分 E_j
//   - consecutiveTerms: 连任次数 r_t
//
// 返回：所需委托权重
func RequiredDelegatedWeight(baseWeight, performance float64, consecutiveTerms int) float64 {
	growthFactor := math.Pow(2.0-performance, float64(consecutiveTerms))
	return baseWeight * growthFactor
}

// ================================
// 统计计算函数
// ================================

// Mean 计算平均值
func Mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

// Median 计算中位数
func Median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	// 创建副本并排序
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	n := len(sorted)
	if n%2 == 0 {
		// 偶数个元素，取中间两个的平均值
		return (sorted[n/2-1] + sorted[n/2]) / 2.0
	}
	// 奇数个元素，取中间的
	return sorted[n/2]
}

// StandardDeviation 计算标准差
func StandardDeviation(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	mean := Mean(values)
	sumSquares := 0.0
	for _, v := range values {
		diff := v - mean
		sumSquares += diff * diff
	}

	variance := sumSquares / float64(len(values))
	return math.Sqrt(variance)
}

// MinMaxExcluded 计算去除最大最小值后的平均值
// 用于环境权重计算（第1.3节）
// 公式：W_env = (总和 - 最大 - 最小) / (数量 - 2)
func MinMaxExcluded(values []float64) float64 {
	if len(values) < 3 {
		return 0
	}

	// 创建副本并排序
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	// 去除最小和最大值
	sum := 0.0
	for i := 1; i < len(sorted)-1; i++ {
		sum += sorted[i]
	}

	return sum / float64(len(sorted)-2)
}

// ================================
// 第4章：动态投票机制 - 时间计算
// ================================

// CalculateNormalVotingPeriod 计算正常投票区段时间
// 公式：T_normal = T_base * (1 + ΣD_i/(n*D_s)) * ln(1 + N_vote/N_all)
// 参数：
//   - baseTime: 基础时间 T_base
//   - nodeDelays: 所有节点的延迟列表
//   - systemDelay: 系统平均延迟 D_s
//   - votingNodes: 参与投票节点数 N_vote
//   - totalNodes: 总节点数 N_all
//
// 返回：正常投票区段时间（毫秒）
func CalculateNormalVotingPeriod(
	baseTime float64,
	nodeDelays []float64,
	systemDelay float64,
	votingNodes, totalNodes int,
) float64 {
	if len(nodeDelays) == 0 || totalNodes == 0 {
		return baseTime
	}

	// 计算网络拥塞因子
	delaySum := 0.0
	for _, d := range nodeDelays {
		delaySum += d
	}

	congestionFactor := 1.0
	if systemDelay > 0 {
		avgDelayRatio := delaySum / (float64(len(nodeDelays)) * systemDelay)
		congestionFactor = 1.0 + avgDelayRatio/float64(len(nodeDelays))
	}

	// 计算参与度因子
	participationRatio := float64(votingNodes) / float64(totalNodes)
	participationFactor := math.Log(1.0 + participationRatio)

	// 最终计算
	normalPeriod := baseTime * congestionFactor * participationFactor

	// 限制在合理范围内
	if normalPeriod < 10.0 {
		normalPeriod = 10.0
	}
	if normalPeriod > 200.0 {
		normalPeriod = 200.0
	}

	return normalPeriod
}

// ================================
// 第5章：奖励分配 - Shapley值计算
// ================================

// ShapleyValue 计算标准Shapley值
// 简化实现：假设每个参与者贡献相等
// 参数：
//   - totalValue: 总价值 v(N)
//   - numParticipants: 参与者数量 n
//
// 返回：每个参与者的Shapley值
func ShapleyValue(totalValue float64, numParticipants int) float64 {
	if numParticipants == 0 {
		return 0
	}
	return totalValue / float64(numParticipants)
}

// ModifiedShapleyValue 计算修正Shapley值
// 公式：φ'_j = φ_j + v(T) * ΔR_j
// 其中：ΔR_j = R_j - (1/n)*ΣR_k
// 参数：
//   - baseShapley: 基础Shapley值 φ_j
//   - totalValue: 总价值 v(T)
//   - performance: 节点表现评分 R_j
//   - avgPerformance: 平均表现评分
//
// 返回：修正后的Shapley值
func ModifiedShapleyValue(baseShapley, totalValue, performance, avgPerformance float64) float64 {
	delta := performance - avgPerformance
	return baseShapley + totalValue*delta
}

// DistributeReward 分配奖励
// 按权重比例分配
// 参数：
//   - totalReward: 总奖励
//   - weights: 各节点权重
//
// 返回：每个节点的奖励
func DistributeReward(totalReward float64, weights []float64) []float64 {
	if len(weights) == 0 {
		return []float64{}
	}

	// 计算总权重
	totalWeight := 0.0
	for _, w := range weights {
		totalWeight += w
	}

	if totalWeight == 0 {
		return make([]float64, len(weights))
	}

	// 按比例分配
	rewards := make([]float64, len(weights))
	for i, w := range weights {
		rewards[i] = (w / totalWeight) * totalReward
	}

	return rewards
}

// Max 返回切片中的最大值
func Max(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	max := values[0]
	for _, v := range values {
		if v > max {
			max = v
		}
	}
	return max
}

// Min 返回切片中的最小值
func Min(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	min := values[0]
	for _, v := range values {
		if v < min {
			min = v
		}
	}
	return min
}

// RandomFloat 生成[0,1)范围内的随机浮点数
func RandomFloat() float64 {
	return rand.Float64()
}

// Shuffle 随机打乱节点切片
func Shuffle(nodes []*types.Node) {
	rand.Shuffle(len(nodes), func(i, j int) {
		nodes[i], nodes[j] = nodes[j], nodes[i]
	})
}

// Log 自然对数
func Log(x float64) float64 {
	return math.Log(x)
}

// TimeNow 获取当前时间（便于测试时mock）
func TimeNow() time.Time {
	return time.Now()
}
