package config

// constants.go
// 全局常量配置文件
// 定义系统中使用的所有常量参数

// ================================
// 第2章：权重衰减机制 - 核心参数
// ================================

const (
	// DecayConstant 衰减常数 c 控制权重衰减的基础速度
	// 公式：λ = c * (1 - R_i)
	// 推荐值：0.2（经过实验验证）
	DecayConstant = 0.2

	// EnvironmentWeight 环境权重 W_env
	// 设为0实现纯指数衰减（所有节点向0收敛）
	// 公式：W_env = (总权重 - 最大 - 最小) / (节点数 - 2)
	EnvironmentWeight = 0.0

	// MinActiveWeight 最低活跃权重阈值
	// 权重低于此值的节点将被标记为非活跃
	MinActiveWeight = 0.01
)

// ================================
// 第3章：节点表现评估 - 评分权重
// ================================

const (
	// AlphaWeight 投票节点评分中的投票速率权重
	// 公式：R_i = α*V_i + β*B_i + γ*E_j
	AlphaWeight = 0.4

	// BetaWeight 投票节点评分中的历史行为权重
	BetaWeight = 0.3

	// GammaWeight 投票节点评分中的代理节点表现权重
	GammaWeight = 0.3

	// OnlineTimeTau 在线时长评分的时间常数 τ
	// 公式：O_j = 1 / (1 + e^(-t_j/τ))
	OnlineTimeTau = 100.0
)

// ================================
// 第4章：动态投票机制 - 时间参数
// ================================

const (
	// BaseVotingTime 基础投票时间（毫秒）
	// 公式：T_normal = T_base * 拥塞因子 * 参与度因子
	BaseVotingTime = 50.0

	// InitialNormalPeriod 初始正常投票区段（毫秒）
	InitialNormalPeriod = 50.0

	// ProxyPeriodStart 代投区段开始时间（毫秒）
	// 等于正常投票区段结束时间
	ProxyPeriodStart = 50.0

	// ProxyPeriodEnd 代投区段结束时间（毫秒）
	ProxyPeriodEnd = 100.0

	// MinVotingPeriod 最小投票时间（毫秒）
	// 防止动态调整后时间过短
	MinVotingPeriod = 10.0

	// MaxVotingPeriod 最大投票时间（毫秒）
	// 防止动态调整后时间过长
	MaxVotingPeriod = 200.0
)

// ================================
// 第5章：奖励分配 - 保证金与惩罚
// ================================

const (
	// DepositRatio 保证金比例
	// 保证金 = 当前权重 * 比例
	DepositRatio = 1.0

	// VoteFailurePenalty 投票失败惩罚比例
	// 扣除权重比例（10%）
	VoteFailurePenalty = 0.1

	// BlockFailurePenalty 出块失败保证金扣除比例
	// 扣除保证金比例（30%）
	BlockFailurePenalty = 0.3

	// BlockReward 固定出块奖励
	BlockReward = 100.0

	// TransactionFeeRatio 交易手续费分配比例
	TransactionFeeRatio = 0.8 // 80%给出块者，20%给验证者
)

// ================================
// 网络配置参数
// ================================

const (
	// NumDelegates 代理节点数量，确保小于总节点数
	// 典型值：2，11, 21, 51
	NumDelegates = 3

	// BlockInterval 出块间隔（秒）
	BlockInterval = 3

	// RoundDuration 一轮的持续时间（秒）
	// 一轮 = 所有代理节点各出一个块的时间
	RoundDuration = NumDelegates * BlockInterval

	// MaxConsecutiveTerms 最大连任次数限制
	// 防止永久寡头化（0表示无限制）
	MaxConsecutiveTerms = 0

	// MaxPeers 最大对等节点数
	MaxPeers = 100

	// MaxBlocksPerSync 每次同步最多获取的区块数
	MaxBlocksPerSync = 100
)

// ================================
// 模拟测试参数
// ================================

const (
	// SimulationRounds 模拟运行轮数
	SimulationRounds = 50

	// InitialNodesCount 初始节点数量
	InitialNodesCount = 50

	// LogLevel 日志级别
	// 0: ERROR, 1: WARN, 2: INFO, 3: DEBUG
	LogLevel = 2
)
