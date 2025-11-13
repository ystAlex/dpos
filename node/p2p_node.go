package node

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"nm-dpos/config"
	"nm-dpos/core"
	"nm-dpos/network"
	"nm-dpos/p2p"
	"nm-dpos/types"
	"nm-dpos/utils"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// p2p_node.go
// 完整的P2P分布式节点实现

// P2PNode 分布式P2P节点
type P2PNode struct {
	// 本地网络视图（使用原有的完整逻辑）
	LocalView *network.LocalNetworkView

	// P2P网络层
	PeerManager      *p2p.PeerManager
	DiscoveryService *p2p.DiscoveryService
	Transport        *p2p.HTTPTransport

	// HTTP服务器
	HTTPServer *http.Server

	//出块调度器
	Scheduler *core.BlockScheduler

	// 配置
	ListenAddress string
	SeedNodes     []string

	InitialTime time.Time

	// 运行状态
	IsRunning bool
	StopChan  chan bool
	Logger    *utils.Logger
}

// NewP2PNode 创建P2P节点
func NewP2PNode(
	nodeID string,
	initialWeight, performance, networkDelay float64,
	listenAddress string,
	seedNodes []string,
	initialTime time.Time,
	logger *utils.Logger,
) *P2PNode {
	// 1. 创建本地节点（使用types.Node）
	localNode := types.NewNode(nodeID, initialWeight, performance, networkDelay)

	// 2. 创建本地网络视图（使用完整的LocalNetworkView）
	localView := network.NewLocalNetworkView(localNode, logger)

	// 4. 创建P2P组件
	peerManager := p2p.NewPeerManager(nodeID, config.MaxPeers)
	transport := p2p.NewHTTPTransport(10 * time.Second)
	discoveryService := p2p.NewDiscoveryService(
		seedNodes,
		nodeID,
		listenAddress,
		peerManager,
	)

	node := &P2PNode{
		LocalView:        localView,
		PeerManager:      peerManager,
		DiscoveryService: discoveryService,
		Transport:        transport,
		ListenAddress:    listenAddress,
		Scheduler:        core.NewBlockScheduler(),
		SeedNodes:        seedNodes,
		InitialTime:      initialTime,
		IsRunning:        false,
		StopChan:         make(chan bool),
		Logger:           logger,
	}

	return node
}

// Start 启动节点
func (pn *P2PNode) Start() error {
	pn.Logger.Info("启动节点 %s，监听地址: %s", pn.LocalView.LocalNode.ID, pn.ListenAddress)
	pn.IsRunning = true

	// 1. 启动HTTP服务器
	if err := pn.startHTTPServer(); err != nil {
		return fmt.Errorf("启动HTTP服务器失败: %v", err)
	}

	// ===== 等待HTTP服务器就绪 =====
	time.Sleep(1 * time.Second)
	if !pn.checkHTTPServerReady() {
		return fmt.Errorf("HTTP服务器启动超时")
	}
	pn.Logger.Info("HTTP服务器已就绪")

	// 记录对等节点
	pn.PeerManager.AddPeer(&p2p.PeerInfo{
		NodeID:   pn.LocalView.LocalNode.ID,
		Address:  pn.ListenAddress,
		LastSeen: time.Now(),
		IsActive: true,
	})

	// 2. 启动节点发现
	pn.DiscoveryService.Start()

	// 3. 启动同步管理器的定期清理
	pn.LocalView.SyncManager.StartPeriodicCleanup()

	// 4. 等待20s等所有节点启动并且连接到网络
	time.Sleep(20 * time.Second)

	pn.Logger.Info("节点 %s 启动完成，已连接 %d 个对等节点",
		pn.LocalView.LocalNode.ID,
		pn.PeerManager.GetPeerCount())

	// 刚开始就同步所有节点的情况，更新所有节点的权重
	pn.syncPeerStates()

	// 5. 启动主循环
	go pn.mainLoop()

	return nil
}

// Stop 停止节点
func (pn *P2PNode) Stop() {
	pn.Logger.Info("停止节点 %s", pn.LocalView.LocalNode.ID)
	pn.IsRunning = false
	pn.StopChan <- true

	if pn.HTTPServer != nil {
		pn.HTTPServer.Close()
	}
}

func (pn *P2PNode) mainLoop() {
	// ============ 1. 初始化创世时间 ============
	genesisTime := pn.InitialTime

	// ============ 2. 配置参数 ============
	blockInterval := config.BlockInterval    // 3秒一个区块
	voteBlockInterval := config.NumDelegates // 每N个区块执行一次投票

	// ============ 3. 计算当前区块高度 ============
	now := time.Now().UTC()
	elapsedSinceGenesis := now.Sub(genesisTime)

	if elapsedSinceGenesis < 0 {
		waitDuration := -elapsedSinceGenesis
		pn.Logger.Info("等待创世时间到达，等待时长: %v", waitDuration)
		time.Sleep(waitDuration)
		now = time.Now().UTC()
		elapsedSinceGenesis = now.Sub(genesisTime)
	}

	// 当前理论区块高度
	currentBlockHeight := int(elapsedSinceGenesis.Seconds()) / blockInterval

	// ============ 4. 创建定时器 ============
	// 区块定时器：每3秒触发一次
	nextBlockTime := genesisTime.Add(time.Duration((currentBlockHeight+1)*blockInterval) * time.Second)
	roundTicker := time.NewTimer(time.Until(nextBlockTime))

	// 同步定时器
	syncTicker := time.NewTicker(2 * time.Second)

	defer roundTicker.Stop()
	defer syncTicker.Stop()

	// ============ 5. 投票会话管理 ============
	var currentSession *core.VotingSession

	// 辅助函数：检查是否需要创建新会话
	shouldCreateSession := func(blockHeight int) bool {
		// 在投票轮次的起始区块创建会话
		// 例如：voteBlockInterval=2 时，在区块 0, 2, 4, 6... 创建
		return blockHeight%voteBlockInterval == 0
	}

	// 辅助函数：创建投票会话
	createVotingSession := func(blockHeight int) *core.VotingSession {
		voteRound := blockHeight/voteBlockInterval + 1
		voteStartTime := genesisTime.Add(time.Duration(blockHeight*blockInterval) * time.Second)

		allActiveNodes := pn.LocalView.GetActiveNodes()
		if len(allActiveNodes) == 0 {
			pn.Logger.Warn("活跃节点为空，跳过投票会话创建")
			return nil
		}

		normalPeriod := core.CalculateNormalVotingPeriod(
			allActiveNodes,
			pn.LocalView.SystemDelay,
			len(allActiveNodes),
		)

		pn.Logger.Info("[ ========= 创建投票会话 =========  | 轮次: %d | 区块高度: %d | 开始时间: %v ]",
			voteRound,
			blockHeight-1,
			voteStartTime.Format(time.RFC3339),
		)

		return core.NewVotingSession(
			voteRound,
			normalPeriod,
			allActiveNodes,
			allActiveNodes,
			voteStartTime,
		)
	}

	// ============ 7. 主循环 ============
	pn.Logger.Info("主循环启动 | 当前区块高度: %d", currentBlockHeight)

	for pn.IsRunning {
		select {
		case <-roundTicker.C:
			// ============ 每个区块周期执行 ============
			pn.Logger.Info("[ ========= 处理区块========= | 理论高度: %d | 本地链高度: %d | 最新区块高度: %d ]",
				currentBlockHeight,
				pn.LocalView.SyncManager.BlockPool.GetLatestHeight(),
				pn.LocalView.SyncManager.BlockPool.GetNewestBlockNumber(),
			)

			// ============ 检查是否需要创建新投票会话 ============
			if shouldCreateSession(currentBlockHeight) {
				// 创建新会话
				currentSession = createVotingSession(currentBlockHeight + 1)
			}

			currentBlockHeight++
			// ============ 执行区块处理逻辑 ============
			// processRound 包含：出块、投票、状态更新等
			if currentSession != nil {
				pn.processRound(currentSession)
			} else {
				// 没有会话时，只执行基础区块同步
				pn.Logger.Warn("无活跃投票会话，跳过投票逻辑 | 区块: %d", currentBlockHeight)
			}

			// 重置定时器
			nextBlockTime = genesisTime.Add(time.Duration((currentBlockHeight+1)*blockInterval) * time.Second)
			roundTicker.Reset(time.Until(nextBlockTime))

		case <-syncTicker.C:
			// 定期同步区块
			pn.checkSync()

		case <-pn.StopChan:
			pn.Logger.Info("主循环停止 | 最终区块高度: %d", currentBlockHeight)

			return
		}
	}
}

// ================================
// 共识流程（使用core/算法） 一轮指的是所有的代理节点都出块之后一起进行共识的过程算一轮
// ================================

// processRound 处理一轮共识
func (pn *P2PNode) processRound(session *core.VotingSession) {

	//TODO: 检查最新区块是否同步完成，只有同步完成才会进行投票
	if pn.Scheduler.CurrentHeight == 0 && pn.LocalView.SyncManager.GetSyncStatus()["is_syncing"] == true || pn.LocalView.SyncManager.BlockPool.GetNewestBlockNumber()-pn.Scheduler.CurrentHeight > 1 {
		pn.Logger.Warn("区块没有完成同步, 当前高度 %d 是否正在同步 %v 最新区块高度 %d", pn.Scheduler.CurrentHeight, pn.LocalView.SyncManager.GetSyncStatus()["is_syncing"], pn.LocalView.SyncManager.BlockPool.GetNewestBlockNumber())
		return
	}

	slotInfo := pn.Scheduler.GetSlotInfo()

	pn.Logger.Info("========================================")
	pn.Logger.Info("第 %d 轮 | 时隙 %d | 区块高度 %d",
		pn.LocalView.CurrentRound+1,
		slotInfo["current_slot"],
		slotInfo["current_height"])
	pn.Logger.Info("当前出块节点: %s", slotInfo["current_producer"])
	pn.Logger.Info("当前进度: %.2f %%", slotInfo["round_progress"].(float64)*100.0)
	pn.Logger.Info("========================================")

	// 1. 更新权重（使用core/decay.go）使用权重衰减
	pn.updateWeight()

	// 2. TODO: 投票阶段,当所有的代理节点都出了块就要重新选举了，重新投票(每轮都执行,用于出块)
	if slotInfo["round_progress"] == 1.0 || (slotInfo["current_slot"] == 0 && slotInfo["current_height"] == 0) {
		pn.LocalView.CurrentRound++

		//用于投票
		pn.conductCompleteVotingPhase(session)

		// 根据委托权重重新选举代理节点
		pn.Logger.Info("步骤3: 根据委托权重选代理节点")
		pn.electDelegatesWithReelectionCheck()

		// 在每一轮投票结束后更新
		pn.syncPeerStates()
	}

	// 3. TODO: 出块阶段（只有当前时隙的权重最高的代理节点才可以出块）
	if pn.Scheduler.ShouldProduce(pn.LocalView.LocalNode.ID) {
		pn.Logger.Info(">>> 本节点是当前出块者，开始出块 <<<")
		pn.produceBlock()
	} else {
		currentProducer, _ := pn.Scheduler.GetCurrentProducer()
		if currentProducer != nil {
			pn.Logger.Debug("等待 %s 出块...", currentProducer.ID)
		}
	}

	// 4. 等待出块完成并处理投票结果
	time.Sleep(500 * time.Millisecond) // 等待出块状态广播

	pn.processVoteResultsAfterBlockProduction()

	// 5. 更新表现评分（使用core/performance.go）
	pn.updatePerformance()

	// 7. 前进到下一个时隙 就行轮流出块的时隙
	pn.Scheduler.NextSlot()

	// 8. 广播状态
	pn.broadcastStatus()
}

// ================================
// 完整的投票阶段实现
// ================================

func (pn *P2PNode) conductCompleteVotingPhase(session *core.VotingSession) {
	pn.Logger.Info("【投票阶段开始】")

	// 获取所有活跃节点(包括代理节点)
	allActiveNodes := pn.LocalView.GetActiveNodes()
	//currentDelegates := pn.LocalView.CurrentDelegates

	if len(allActiveNodes) == 0 { //|| len(currentDelegates) == 0
		pn.Logger.Warn("活跃节点为空，跳过投票阶段")
		return
	}

	// 1. 锁定保证金(所有活跃节点,包括代理)
	deposit := core.LockDeposit(pn.LocalView.LocalNode)
	pn.Logger.Info("步骤1: 锁定保证金 = %.2f", deposit)

	// 2. 赋值会话时间
	pn.LocalView.VotingSession = session
	pn.Logger.Info("创建投票会话: 正常区段开始时间=%v, 正常区段结束时间=%v, 代投区段开始时间=%v, 代投区段结束时间=%v,", session.StartTime.Format(time.RFC3339Nano), session.ProxyStartTime.Format(time.RFC3339Nano), session.ProxyStartTime.Format(time.RFC3339Nano), session.EndTime.Format(time.RFC3339Nano))

	// 3. 投票阶段(所有节点都参与)
	pn.Logger.Info("步骤2: 投票阶段开始")
	pn.voteInSession()

	// 4. 等待正常投票期结束
	waitTime := session.ProxyStartTime.Sub(time.Now()).Milliseconds()
	pn.Logger.Info("等待正常投票期结束 等待时间: %v ms", waitTime)
	time.Sleep(time.Duration(waitTime) * time.Millisecond)

	failedVoters := session.ProcessFailures()
	if len(failedVoters) > 0 {
		pn.Logger.Warn("正常投票期结束, 投票失败节点数: %d", len(failedVoters))
	}
	for _, node := range failedVoters {
		if node.ID == pn.LocalView.LocalNode.ID {
			//自身投票失败
			core.RecordVoteFailure(pn.LocalView.LocalNode)
		}
	}

	// 5. 正常投票期结束还没有投票的就需要代投了，要代理节点执行代投
	if pn.LocalView.IsLocalDelegate() {
		//代表本身是代投节点
		//谁没有投递就去帮人vote
		pn.Logger.Info("我是代投节点 处理代投")
		pn.executeProxyVoting()
	}

	// 6. 等待代投期结束
	waitTime = session.EndTime.Sub(time.Now()).Milliseconds()
	pn.Logger.Info("等待代投期结束 等待时间: %v ms", waitTime)
	time.Sleep(time.Duration(waitTime) * time.Millisecond)

	// 7. 处理投票失败节点
	failedVoters = session.ProcessFailures()
	if len(failedVoters) > 0 {
		pn.Logger.Warn("代投期结束, 投票失败节点数: %d", len(failedVoters))
	}

	// 8. 分配热补偿(此时还不处理保证金,等出块后再说)
	if pn.LocalView.LocalNode.Type == types.DelegateNode {
		pn.distributeHotCompensation()
	}

	pn.Logger.Info("【投票阶段结束】")
	stats := session.GetStatistics()
	pn.Logger.Info("  总投票: %d, 成功: %d, 代投: %d, 失败: %d",
		stats.TotalVoters, stats.SuccessfulVoters, stats.ProxiedVoters, stats.FailedVoters)
}

// vote 发送真实投票在会话中投票
func (pn *P2PNode) voteInSession() {
	if !pn.LocalView.LocalNode.IsActive {
		return
	}

	session := pn.LocalView.VotingSession
	if session == nil {
		return
	}

	target := pn.LocalView.GetVotingTarget()
	if target == nil {
		pn.Logger.Warn("没有可投票的节点")
		return
	}

	pn.Logger.Info("  本节点投票目标: %s (权重: %.2f, 表现: %.3f)",
		target.ID, target.CurrentWeight, target.PerformanceScore)

	// TODO：根据节点定义的延迟时间，确定节点是否会随机不投票的随机掉线情况
	votingTime := core.SimulateVotingTime(pn.LocalView.LocalNode)

	// 判断投票结果
	if votingTime <= session.NormalPeriod {
		pn.Logger.Info("本节点正常投票 (耗时: %.2f ms)", votingTime)
	} else if votingTime <= config.ProxyPeriodEnd {
		pn.Logger.Info("本节点超时,需要代投 (耗时: %.2f ms)", votingTime)
		return // 不投票, 等待代投
	} else {
		pn.Logger.Warn("本节点投票失败 (耗时: %.2f ms)", votingTime)
		return // 投票失败
	}

	// 构造投票数据
	voteData := map[string]interface{}{
		"voter_id":    pn.LocalView.LocalNode.ID,
		"target_id":   target.ID,
		"weight":      pn.LocalView.LocalNode.CurrentWeight,
		"voting_time": votingTime,
		"round":       pn.LocalView.CurrentRound,
		"timestamp":   time.Now().Unix(),
	}

	// 应该给所有节点都要 发送投票信息
	if err := pn.sendVoteMessage(voteData); err != nil {
		pn.Logger.Warn("投票发送失败: %v", err)
	} else {
		// 记录到本地
		pn.LocalView.LocalNode.RecordVote(
			target.ID,
			pn.LocalView.LocalNode.CurrentWeight,
			votingTime,
			0,
			true,
			false,
			"",
		)
		pn.Logger.Info("投票已发送: %s -> %s", pn.LocalView.LocalNode.ID, target.ID)
	}

}

// executeProxyVoting 执行代投(代理节点调用)
func (pn *P2PNode) executeProxyVoting() {
	session := pn.LocalView.VotingSession
	if session == nil {
		return
	}

	pn.Logger.Info("【执行代投】")

	// 检查需要代投的节点
	needProxy := session.GetNodesNeedingProxy()
	if len(needProxy) == 0 {
		pn.Logger.Info("无需代投")
		return
	}

	pn.Logger.Info("需要代投的节点数: %d", len(needProxy))

	proxyNodes := session.GetProxyNodes(pn.LocalView.CurrentDelegates)

	if len(proxyNodes) == 0 {
		pn.Logger.Warn("代投节点未找到")
		return
	}

	//for _, proxyNode := range proxyNodes {
	//	fmt.Println("选择出来的代投节点是", proxyNode.ID)
	//}

	// 执行代投并获取补偿

	compensations := pn.ExecuteProxyVoting(needProxy, proxyNodes, session)

	// 应用补偿到本地节点
	if comp, ok := compensations[pn.LocalView.LocalNode.ID]; ok && comp > 0 {
		pn.LocalView.LocalNode.TotalCompensation += comp
		pn.Logger.Info("获得代投补偿: %.2f", comp)
	}

	pn.Logger.Info("代投完成")

}

// distributeHotCompensation 分配热补偿(代理节点调用)
func (pn *P2PNode) distributeHotCompensation() {
	session := pn.LocalView.VotingSession
	if session == nil {
		return
	}

	pn.Logger.Info("【分配热补偿】")

	// ✅ 只有投票给成功出块代理的节点才能获得热补偿
	eligibleVoters := make([]*types.Node, 0)

	for voterID, targetID := range session.VoteRecords {
		// 检查目标代理节点是否成功出块
		var targetDelegate *types.Node
		for _, delegate := range pn.LocalView.CurrentDelegates {
			if delegate.ID == targetID {
				targetDelegate = delegate
				break
			}
		}

		if targetDelegate != nil && targetDelegate.HasProducedBlockInRound(pn.LocalView.CurrentRound) {
			// 找到投票者节点
			for _, node := range session.Voters {
				if node.ID == voterID {
					eligibleVoters = append(eligibleVoters, node)
					break
				}
			}
		}
	}

	if len(eligibleVoters) == 0 {
		pn.Logger.Info("无符合条件的节点，跳过热补偿")
		return
	}

	// 分配热补偿
	hotComp := core.DistributeHotCompensation(session, eligibleVoters)

	pn.Logger.Info("热补偿池: %.2f, 受益节点数: %d", hotComp, len(eligibleVoters))

	// 更新网络状态
	pn.LocalView.Network.HotCompensationPool = hotComp
	pn.LocalView.Network.TotalDeductedWeight = session.TotalDeductedWeight
}

// ExecuteProxyVoting 执行代投(由代理节点调用)
func (pn *P2PNode) ExecuteProxyVoting(needProxy []*types.Node, proxyNodes []*types.Node, session *core.VotingSession) map[string]float64 {
	compensations := make(map[string]float64)

	for i, proxied := range needProxy {

		proxy := proxyNodes[i/len(proxyNodes)]

		// 只有当前节点是代投者时才执行投票
		if proxy.ID != pn.LocalView.LocalNode.ID {
			//fmt.Println("111111111111111不是代投节点", pn.LocalView.LocalNode.ID, proxy.ID)
			continue
		}

		// 随机选择目标
		if len(session.Candidates) > 0 {
			targetIdx := rand.Intn(len(session.Candidates))
			target := session.Candidates[targetIdx]

			pn.Logger.Info("执行代投: %s -> %s (代投者: %s)",
				proxied.ID, target.ID, proxy.ID)

			// 构造代投数据
			voteData := map[string]interface{}{
				"voter_id":    proxied.ID, // 被代投的节点ID
				"target_id":   target.ID,  // 投票目标
				"weight":      proxied.CurrentWeight,
				"voting_time": config.ProxyPeriodEnd - 100, // 代投时间略小于代投期结束
				"round":       pn.LocalView.CurrentRound,
				"timestamp":   time.Now().Unix(),
				"is_proxy":    true,     // 标记为代投
				"proxy_id":    proxy.ID, // 代投者ID
			}

			// 应该给所有节点都要 发送投票信息
			if err := pn.sendVoteMessage(voteData); err != nil {
				pn.Logger.Warn("代投投票发送失败: %v", err)
			} else {
				// 记录到本地
				pn.LocalView.LocalNode.RecordVote(
					target.ID,
					pn.LocalView.LocalNode.CurrentWeight,
					0,
					0,
					true,
					true,
					proxied.ID,
				)
				pn.Logger.Info("代投投票已发送: %s -> %s (代投者: %s)",
					proxied.ID, target.ID, proxy.ID)

				// TODO: 成功之后才会计算代投补偿
				compensation := core.CalculateProxyCompensation(proxied, session.ProxyPeriod)

				// 记录代投
				session.VoteRecords[proxied.ID] = target.ID
				session.ProxiedVoters[proxied.ID] = proxy.ID
				session.VoteResults[target.ID] += proxied.CurrentWeight

				// 代投者获得补偿
				compensations[proxy.ID] += compensation
			}
		}
	}
	return compensations
}

// syncPeerStates 同步对等节点状态
func (pn *P2PNode) syncPeerStates() {
	peers := pn.PeerManager.GetActivePeers()

	var wg sync.WaitGroup

	for _, peer := range peers {
		wg.Add(1)
		go func(p *p2p.PeerInfo) {
			defer wg.Done()

			status, err := pn.requestNodeStatus(p.Address)
			if err != nil {
				return
			}

			node := types.NewNode(
				status.NodeID,
				status.InitialWeight,
				status.Performance,
				20.0,
			)
			node.CurrentWeight = status.CurrentWeight
			node.IsActive = status.IsActive
			status.NodeType = node.Type.Int(status.NodeType).String()
			node.Type = node.Type.Int(status.NodeType)
			node.Deposit = status.Deposit
			node.ConsecutiveTerms = status.ConsecutiveTerms
			node.CorrectValidations = status.CorrectValidations
			node.DelayedBlocks = status.DelayedBlocks
			node.LastElectedRound = status.LastElectedRound
			node.LastUpdateTime = status.LastUpdateTime
			node.VotingSpeed = status.VotingSpeed
			node.VotingBehavior = status.VotingBehavior
			node.FailedVotes = status.FailedVotes
			node.SuccessfulVotes = status.SuccessfulVotes
			node.BlockEfficiency = status.BlockEfficiency
			node.BlockHistory = status.BlockHistory
			node.PerformanceHistory = status.PerformanceHistory
			node.BlockProductionStatus = status.BlockProductionStatus
			node.InvalidBlocks = status.InvalidBlocks
			node.RequiredWeight = status.RequiredWeight
			node.VoteHistory = status.VoteHistory
			node.DelegatedWeight = status.DelegatedWeight
			node.TotalValidations = status.TotalValidations
			node.ValidationEfficiency = status.ValidationEfficiency
			node.OnlineTime = status.OnlineTime
			node.ValidBlocks = status.ValidBlocks

			// 现在UpdatePeerInfo是线程安全的
			pn.LocalView.UpdatePeerInfo(status.NodeID, node)

			pn.PeerManager.UpdatePeerStatus(
				status.NodeID,
				status.IsActive,
				status.CurrentWeight,
				status.Performance,
			)

			//fmt.Println("55555555555555555555555555", node.ID, node.ConsecutiveTerms, node.Type)

			pn.Logger.Info("节点状态更新: 节点ID %s, 节点状态 %v 节点当前权重 %f 节点表现分 %f", status.NodeID, status.NodeType, status.CurrentWeight, status.Performance)
		}(peer)
	}

	// 等待所有状态同步完成
	wg.Wait()
}

// updateWeight 更新权重（使用core/decay.go）
func (pn *P2PNode) updateWeight() {
	oldWeight := pn.LocalView.LocalNode.CurrentWeight

	pn.LocalView.UpdateLocalWeight()

	newWeight := pn.LocalView.LocalNode.CurrentWeight
	decayRate := core.CalculateDecayRate(pn.LocalView.LocalNode.PerformanceScore)
	halfLife := pn.LocalView.LocalNode.GetHalfLife()

	pn.Logger.Info("【权重衰减】")
	pn.Logger.Info("  旧权重: %.2f → 新权重: %.2f (变化: %.2f)",
		oldWeight, newWeight, newWeight-oldWeight)
	pn.Logger.Info("  表现评分: %.3f | 衰减率: %.4f | 半衰期: %.1f轮",
		pn.LocalView.LocalNode.PerformanceScore, decayRate, halfLife)
}

// produceBlock 生产区块
func (pn *P2PNode) produceBlock() {
	// ===== 开始计时 =====
	startTime := time.Now()

	if !pn.LocalView.LocalNode.IsActive {
		return
	}

	pn.Logger.Info("【出块阶段】")

	pn.Logger.Info("节点 %s 开始出块", pn.LocalView.LocalNode.ID)

	currentRound := pn.LocalView.CurrentRound

	// 1. 从交易池获取交易
	txs := pn.LocalView.TxPool.GetPendingTransactions(100)
	pn.Logger.Info("  从交易池获取 %d 笔交易", len(txs))

	// 2. 创建区块
	prevBlock := pn.LocalView.SyncManager.BlockPool.GetLatestBlock()
	prevHash := "genesis" //默认值
	height := 1
	if prevBlock != nil {
		prevHash = prevBlock.Hash
		height = prevBlock.Height + 1
	}

	block := &network.Block{
		Height:       height,
		ProducerID:   pn.LocalView.LocalNode.ID,
		Transactions: txs,
		Timestamp:    time.Now(),
		PrevHash:     prevHash,
		Hash:         fmt.Sprintf("hash-%d-%s", time.Now().Unix(), pn.LocalView.LocalNode.ID),
	}

	prevHashShort := prevHash
	if len(prevHash) > 8 {
		prevHashShort = prevHash[:8]
	}
	pn.Logger.Info("  创建区块 #%d (前块: %s)", height, prevHashShort)

	// 计算交易手续费
	transactionFees := pn.calculateTransactionFees(txs)
	pn.Logger.Info("  交易手续费总额: %.2f", transactionFees)

	// 3. 添加到本地区块池
	if err := pn.LocalView.SyncManager.BlockPool.AddBlock(block); err != nil {
		pn.Logger.Warn("添加区块失败: %v", err)
		pn.LocalView.LocalNode.RecordBlockProductionStatus(currentRound, false)
		core.RecordBlockProduction(pn.LocalView.LocalNode, block.Height, false, false, true, 0, len(block.Transactions))
		pn.broadcastBlockProductionStatus(currentRound, false)
		return
	}

	// ===== 计算出块耗时 =====
	productionTime := time.Since(startTime)

	// 判断是否延迟(阈值50ms)
	isDelayed := productionTime.Milliseconds() > 50

	if isDelayed {
		pn.Logger.Warn(" 延迟出块 (耗时: %d ms)", productionTime.Milliseconds())
		core.RecordBlockProduction(pn.LocalView.LocalNode, block.Height, true, true, false, productionTime, len(block.Transactions))
	} else {
		pn.Logger.Info(" 及时出块 (耗时: %d ms)", productionTime.Milliseconds())
		core.RecordBlockProduction(pn.LocalView.LocalNode, block.Height, true, false, false, productionTime, len(block.Transactions))
	}

	blockSuccess := true

	pn.Scheduler.CurrentHeight++

	// 4. 广播区块
	if err := pn.broadcastBlock(block); err != nil {
		pn.Logger.Warn("广播区块失败: %v", err)
		blockSuccess = false

		//  记录出块失败
		pn.LocalView.LocalNode.RecordBlockProductionStatus(currentRound, false)

		//  广播出块失败状态
		pn.broadcastBlockProductionStatus(currentRound, false)
		return
	}

	pn.Logger.Info("节点 %s 成功出块 #%d (包含 %d 笔交易)",
		pn.LocalView.LocalNode.ID, block.Height, len(txs))

	// 记录出块成功
	pn.LocalView.LocalNode.RecordBlockProductionStatus(currentRound, blockSuccess)

	// 广播出块成功状态
	pn.broadcastBlockProductionStatus(currentRound, blockSuccess)

	// 分配区块奖励
	pn.distributeBlockRewards(transactionFees)

	// 5. 移除已打包的交易
	txIDs := extractTxIDs(txs)
	pn.LocalView.TxPool.RemoveTransactions(txIDs)
}

// 计算交易手续费
func (pn *P2PNode) calculateTransactionFees(txs []*network.Transaction) float64 {
	totalFees := 0.0
	for _, tx := range txs {
		// 假设每笔交易手续费是交易金额的0.1%
		totalFees += tx.Amount * 0.001
	}
	return totalFees
}

// 分配区块奖励
func (pn *P2PNode) distributeBlockRewards(transactionFees float64) {
	pn.Logger.Info("【分配区块奖励】")

	// 1. 出块者奖励 (固定奖励 + 80%手续费)
	producerReward := core.CalculateBlockProducerReward(config.BlockReward, transactionFees)
	pn.LocalView.LocalNode.CurrentWeight += producerReward
	pn.LocalView.LocalNode.RecordBlockReward(pn.LocalView.CurrentRound, producerReward)

	pn.Logger.Info("  出块者奖励: %.2f (固定=%.2f + 手续费=%.2f)",
		producerReward, config.BlockReward, transactionFees*config.TransactionFeeRatio)

	validators := pn.getValidators()

	if len(validators) > 0 {
		validatorReward := core.CalculateValidatorReward(transactionFees, len(validators))
		pn.Logger.Info("  单个验证者奖励: %.2f (共%d个验证者)", validatorReward, len(validators))

		// 广播验证者奖励
		for _, validator := range validators {
			pn.broadcastValidatorReward(validator.ID, validatorReward)
		}
	}

	// 使用修正Shapley值分配
	participants := []*types.Node{pn.LocalView.LocalNode}
	participants = append(participants, validators...)
	rewards := core.DistributeBlockReward(config.BlockReward, participants)
	pn.applyBlockRewards(rewards)
}

// getValidators 获取验证者(其他代理节点)
func (pn *P2PNode) getValidators() []*types.Node {
	validators := make([]*types.Node, 0)
	for _, delegate := range pn.LocalView.CurrentDelegates {
		if delegate.ID != pn.LocalView.LocalNode.ID {
			validators = append(validators, delegate)
		}
	}
	return validators
}

// broadcastValidatorReward 广播验证者奖励
func (pn *P2PNode) broadcastValidatorReward(validatorID string, reward float64) {
	rewardData := map[string]interface{}{
		"validator_id": validatorID,
		"reward":       reward,
		"round":        pn.LocalView.CurrentRound,
		"timestamp":    time.Now().Unix(),
	}

	peer, exists := pn.PeerManager.GetPeer(validatorID)
	if exists {
		_, err := pn.Transport.SendJSON(peer.Address, "/validator_reward", rewardData)
		if err != nil {
			return
		}
	}
}

// 应用区块奖励(使用Shapley值)
func (pn *P2PNode) applyBlockRewards(rewards map[string]float64) {
	for nodeID, reward := range rewards {
		if nodeID == pn.LocalView.LocalNode.ID {
			pn.LocalView.LocalNode.CurrentWeight += reward
			pn.LocalView.LocalNode.RecordBlockReward(pn.LocalView.CurrentRound, reward)
			pn.Logger.Info("  应用奖励: %.2f", reward)
		}
	}
}

// 广播出块状态
func (pn *P2PNode) broadcastBlockProductionStatus(roundID int, success bool) {
	statusData := map[string]interface{}{
		"producer_id": pn.LocalView.LocalNode.ID,
		"round":       roundID,
		"success":     success,
		"timestamp":   time.Now().Unix(),
	}

	peers := pn.PeerManager.GetActivePeers()
	for _, peer := range peers {
		pn.Logger.Info("节点 %s 向节点 %s 广播出块:", pn.LocalView.LocalNode.ID, peer.NodeID)
		go pn.Transport.SendJSON(peer.Address, "/block_production_status", statusData)
	}

	pn.Logger.Info("广播出块状态: round=%d, success=%v", roundID, success)
}

// updatePerformance 更新表现评分（使用core/performance.go）
func (pn *P2PNode) updatePerformance() {
	oldPerf := pn.LocalView.LocalNode.PerformanceScore

	pn.LocalView.UpdateLocalPerformance()

	newPerf := pn.LocalView.LocalNode.PerformanceScore

	if pn.LocalView.LocalNode.Type == types.VoterNode {
		pn.Logger.Info("【投票节点表现】")
		pn.Logger.Info("  综合评分: %.3f → %.3f", oldPerf, newPerf)
		pn.Logger.Info("  投票速率 V: %.3f", pn.LocalView.LocalNode.VotingSpeed)
		pn.Logger.Info("  投票行为 B: %.3f", pn.LocalView.LocalNode.VotingBehavior)
		pn.Logger.Info("  成功/失败: %d/%d",
			pn.LocalView.LocalNode.SuccessfulVotes,
			pn.LocalView.LocalNode.FailedVotes)
	} else {
		pn.Logger.Info("【代理节点表现】")
		pn.Logger.Info("  综合评分: %.3f → %.3f", oldPerf, newPerf)
		pn.Logger.Info("  出块效率 P: %.3f", pn.LocalView.LocalNode.BlockEfficiency)
		pn.Logger.Info("  验证效率 C: %.3f", pn.LocalView.LocalNode.ValidationEfficiency)
		pn.Logger.Info("  在线评分 O: %.3f", core.CalculateOnlineScore(pn.LocalView.LocalNode))
		pn.Logger.Info("  有效/延迟/无效: %d/%d/%d",
			pn.LocalView.LocalNode.ValidBlocks,
			pn.LocalView.LocalNode.DelayedBlocks,
			pn.LocalView.LocalNode.InvalidBlocks)
	}
}

// broadcastStatus 广播节点状态
func (pn *P2PNode) broadcastStatus() {
	statusData := map[string]interface{}{
		"is_delegate":             pn.LocalView.IsLocalDelegate(),
		"round":                   pn.LocalView.CurrentRound,
		"timestamp":               time.Now().Unix(),
		"id":                      pn.LocalView.LocalNode.ID,
		"type":                    int(pn.LocalView.LocalNode.Type),
		"is_active":               pn.LocalView.LocalNode.IsActive,
		"current_weight":          pn.LocalView.LocalNode.CurrentWeight,
		"initial_weight":          pn.LocalView.LocalNode.InitialWeight,
		"performance_score":       pn.LocalView.LocalNode.PerformanceScore,
		"successful_votes":        pn.LocalView.LocalNode.SuccessfulVotes,
		"failed_votes":            pn.LocalView.LocalNode.FailedVotes,
		"total_reward":            pn.LocalView.LocalNode.TotalReward,
		"total_compensation":      pn.LocalView.LocalNode.TotalCompensation,
		"connected_peers":         pn.PeerManager.GetPeerCount(),
		"current_round":           pn.LocalView.CurrentRound,
		"block_height":            pn.LocalView.SyncManager.BlockPool.GetLatestHeight(),
		"pending_tx_count":        pn.LocalView.TxPool.GetPendingCount(),
		"deposit":                 pn.LocalView.LocalNode.Deposit,
		"consecutive_terms":       pn.LocalView.LocalNode.ConsecutiveTerms,
		"block_efficiency":        pn.LocalView.LocalNode.BlockEfficiency,
		"total_validations":       pn.LocalView.LocalNode.TotalValidations,
		"block_history":           pn.LocalView.LocalNode.BlockHistory,
		"block_production_status": pn.LocalView.LocalNode.BlockProductionStatus,
		"last_update_time":        pn.LocalView.LocalNode.LastUpdateTime,
		"correct_validations":     pn.LocalView.LocalNode.CorrectValidations,
		"delayed_blocks":          pn.LocalView.LocalNode.DelayedBlocks,
		"last_elected_round":      pn.LocalView.LocalNode.LastElectedRound,
		"invalid_blocks":          pn.LocalView.LocalNode.InvalidBlocks,
		"voting_speed":            pn.LocalView.LocalNode.VotingSpeed,
		"voting_behavior":         pn.LocalView.LocalNode.VotingBehavior,
		"validation_efficiency":   pn.LocalView.LocalNode.ValidationEfficiency,
		"performance_history":     pn.LocalView.LocalNode.PerformanceHistory,
		"required_weight":         pn.LocalView.LocalNode.RequiredWeight,
		"vote_history":            pn.LocalView.LocalNode.VoteHistory,
		"online_time":             pn.LocalView.LocalNode.OnlineTime,
		"delegated_weight":        pn.LocalView.LocalNode.DelegatedWeight,
	}

	peers := pn.PeerManager.GetActivePeers()
	for _, peer := range peers {
		go pn.sendStatusUpdate(peer.Address, statusData)
	}
}

// checkSync 检查并执行同步
func (pn *P2PNode) checkSync() {
	localHeight := pn.LocalView.SyncManager.BlockPool.GetLatestHeight()
	newestHeight := pn.LocalView.SyncManager.BlockPool.GetNewestBlockNumber()

	peerIDs := make([]string, 0)
	for _, peer := range pn.PeerManager.GetActivePeers() {
		peerIDs = append(peerIDs, peer.NodeID)
	}

	syncedNumber, err := pn.LocalView.SyncManager.CheckAndSync(
		pn.LocalView.LocalNode.ID,
		peerIDs,
		localHeight,
		newestHeight,
		pn.Transport,
		pn.PeerManager,
	)

	if err != nil {
		pn.Logger.Warn("同步检查失败: %v", err)
	}

	if syncedNumber > 0 {
		pn.Logger.Info("节点 %s 已经同步到了区块 %d", pn.LocalView.LocalNode.ID, syncedNumber)
		//同步当前区块高度
		pn.Scheduler.CurrentHeight = syncedNumber
	}
}

// ================================
// HTTP服务器（接收其他节点的消息）
// ================================

func (pn *P2PNode) startHTTPServer() error {
	mux := http.NewServeMux()

	// ===== 核心共识端点 =====
	mux.HandleFunc("/vote", pn.handleVoteRequest)                              // 投票消息
	mux.HandleFunc("/block", pn.handleBlockRequest)                            // 区块消息
	mux.HandleFunc("/block_production_status", pn.handleBlockProductionStatus) // 区块产生情况的消息
	mux.HandleFunc("/transaction", pn.handleTransactionRequest)                // 交易消息

	// ===== 同步端点 =====
	mux.HandleFunc("/sync_request", pn.handleSyncRequestMessage) // 同步请求

	// ===== 节点管理端点 =====
	mux.HandleFunc("/status", pn.handleStatusRequest)       // 状态查询
	mux.HandleFunc("/status_update", pn.handleStatusUpdate) // 状态更新
	mux.HandleFunc("/peers", pn.handlePeersRequest)         // 节点列表
	mux.HandleFunc("/heartbeat", pn.handleHeartbeat)        // 心跳
	mux.HandleFunc("/validator_reward", pn.handleValidatorReward)

	pn.HTTPServer = &http.Server{
		Addr:    pn.ListenAddress,
		Handler: mux,
	}

	go func() {
		if err := pn.HTTPServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			pn.Logger.Error("HTTP服务器错误: %v", err)
		}
	}()

	return nil
}

// handleVote 处理投票消息
func (pn *P2PNode) handleVoteRequest(w http.ResponseWriter, r *http.Request) {
	var voteData struct {
		VoterID    string  `json:"voter_id"`
		TargetID   string  `json:"target_id"`
		Weight     float64 `json:"weight"`
		VotingTime float64 `json:"voting_time"`
		Round      int     `json:"round"`
		Timestamp  int64   `json:"timestamp"`
		IsProxy    bool    `json:"is_proxy"` // 新增：是否为代投
		ProxyID    string  `json:"proxy_id"` // 新增：代投者ID
	}

	if err := json.NewDecoder(r.Body).Decode(&voteData); err != nil {
		pn.Logger.Warn("解析投票数据失败: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if pn.LocalView.VotingSession != nil && pn.LocalView.VotingSession.RoundID != voteData.Round {
		pn.Logger.Info(" 收到投票过早,等待新的session创建 本地轮次 %d, 接收轮次 %d", pn.LocalView.VotingSession.RoundID, voteData.Round)
		//停100ms在记录防止数据没有记录成功，要先创建会话再去操作
		time.Sleep(20 * time.Millisecond)
	}

	pn.Logger.Info("【收到投票】")
	if voteData.IsProxy {
		pn.Logger.Info("  轮次 %d | 代投: %s 为 %s | 目标: %s | 权重: %.2f",
			voteData.Round, voteData.ProxyID, voteData.VoterID, voteData.TargetID, voteData.Weight)
		pn.LocalView.VotingSession.RecordProxyVote(voteData.VoterID, voteData.ProxyID)
	} else {
		pn.Logger.Info("  轮次 %d | 投票者: %s | 目标: %s | 权重: %.2f | 耗时: %.2fms",
			voteData.Round, voteData.VoterID, voteData.TargetID, voteData.Weight, voteData.VotingTime)
	}

	// ✅ 记录到投票会话
	if pn.LocalView.VotingSession != nil {
		//fmt.Println("1111111111111开始记录投票信息", voteData.VoterID,
		//	voteData.TargetID,
		//	voteData.VotingTime)
		pn.LocalView.VotingSession.RecordVote(
			voteData.VoterID,
			voteData.TargetID,
			voteData.VotingTime,
		)
	}

	// 如果是代理节点,记录到投票会话
	if pn.LocalView.LocalNode.ID == voteData.TargetID {
		pn.LocalView.LocalNode.DelegatedWeight += voteData.Weight
		pn.Logger.Info("  累计委托权重: %.2f", pn.LocalView.LocalNode.DelegatedWeight)
	}

	w.WriteHeader(http.StatusOK)
}

// handleBlockMessage 处理区块消息
func (pn *P2PNode) handleBlockRequest(w http.ResponseWriter, r *http.Request) {

	var block network.Block
	if err := json.NewDecoder(r.Body).Decode(&block); err != nil {
		pn.Logger.Warn("解析区块数据失败: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	pn.Logger.Info("收到来自 %s 的区块 #%d", block.ProducerID, block.Height)

	if pn.Scheduler.CurrentHeight >= block.Height {
		pn.Logger.Warn("区块已经存在 当前高度 %v, 新区块号 %v", pn.Scheduler.CurrentHeight, block.Height)
		return
	}

	pn.LocalView.SyncManager.BlockPool.SetNewestBlockNumber(block.Height)

	if pn.Scheduler.CurrentHeight != block.Height-1 {
		//只有少1个区块才可以进行同步
		pn.Logger.Warn("区块落后太多 当前高度 %v, 新区块号 %v", pn.Scheduler.CurrentHeight, block.Height)
		return
	}

	pn.Scheduler.CurrentHeight = block.Height

	// 验证并添加区块
	if err := pn.LocalView.SyncManager.BlockPool.AddBlock(&block); err != nil {
		pn.Logger.Warn("添加区块失败: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 从交易池移除已确认的交易
	txIDs := make([]string, len(block.Transactions))
	for i, tx := range block.Transactions {
		txIDs[i] = tx.ID
	}
	pn.LocalView.TxPool.RemoveTransactions(txIDs)
}

// handleBlockProductionStatus  HTTP处理器 - 等待多数成功才会改变状态 接收出块状态
func (pn *P2PNode) handleBlockProductionStatus(w http.ResponseWriter, r *http.Request) {
	var statusData struct {
		ProducerID string `json:"producer_id"`
		Round      int    `json:"round"`
		Success    bool   `json:"success"`
		Timestamp  int64  `json:"timestamp"`
	}

	if err := json.NewDecoder(r.Body).Decode(&statusData); err != nil {
		pn.Logger.Warn("解析出块状态失败: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	pn.Logger.Info("【收到出块状态】")
	pn.Logger.Info("  生产者: %s | 轮次: %d | 成功: %v",
		statusData.ProducerID, statusData.Round, statusData.Success)

	if pn.LocalView.CurrentRound < statusData.Round {
		pn.LocalView.CurrentRound = statusData.Round
	}

	var flag = false

	// TODO: 更新对应节点的出块状态
	for _, delegate := range pn.LocalView.CurrentDelegates {
		if delegate.ID == statusData.ProducerID {
			delegate.RecordBlockProductionStatus(statusData.Round, statusData.Success)
			pn.Logger.Info("  更新代理节点 %s 的出块状态", statusData.ProducerID)
			flag = true
			break
		}
	}

	var producerNode *types.Node

	candidates := pn.LocalView.GetActiveNodes()
	for _, candidate := range candidates {
		if candidate.ID == statusData.ProducerID {
			producerNode = candidate
		}
	}

	if !flag && producerNode != nil {
		producerNode.RecordBlockProductionStatus(statusData.Round, statusData.Success)
		pn.Logger.Info("  更新代理节点 %s 的出块状态", statusData.ProducerID)
		pn.LocalView.CurrentDelegates = append(pn.LocalView.CurrentDelegates, producerNode)
	}

	if !flag && producerNode == nil {
		pn.Logger.Warn("没有接收到此轮节点 %s 轮次 %d 的广播状态", statusData.ProducerID, statusData.Round)
	}

	w.WriteHeader(http.StatusOK)
}

// handleTransactionMessage 处理交易消息
func (pn *P2PNode) handleTransactionRequest(w http.ResponseWriter, r *http.Request) {
	var tx network.Transaction

	if err := json.NewDecoder(r.Body).Decode(&tx); err != nil {
		pn.Logger.Warn("解析交易数据失败: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	pn.Logger.Debug("收到交易 %s: %s -> %s (金额: %.2f)",
		tx.ID, tx.From, tx.To, tx.Amount)

	// 添加到交易池
	if err := pn.LocalView.TxPool.AddTransaction(&tx); err != nil {
		pn.Logger.Warn("添加交易失败: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
}

// handleStatusRequest 处理状态查询
func (pn *P2PNode) handleStatusRequest(w http.ResponseWriter, r *http.Request) {
	status := pn.GetStatus()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// handleStatusUpdate 处理状态更新
func (pn *P2PNode) handleStatusUpdate(w http.ResponseWriter, r *http.Request) {
	var statusData map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&statusData); err != nil {
		pn.Logger.Warn("解析状态数据失败: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// ===== 1. 提取基础字段 =====
	nodeID, ok := statusData["id"].(string)
	if !ok {
		pn.Logger.Warn("id类型错误")
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	initialWeight, ok := statusData["initial_weight"].(float64)
	if !ok {
		pn.Logger.Warn("initial_weight类型错误")
		http.Error(w, "invalid initial_weight", http.StatusBadRequest)
		return
	}

	performance, ok := statusData["performance_score"].(float64)
	if !ok {
		pn.Logger.Warn("performance_score类型错误")
		http.Error(w, "invalid performance_score", http.StatusBadRequest)
		return
	}

	currentWeight, ok := statusData["current_weight"].(float64)
	if !ok {
		pn.Logger.Warn("current_weight类型错误")
		http.Error(w, "invalid current_weight", http.StatusBadRequest)
		return
	}

	isActive, ok := statusData["is_active"].(bool)
	if !ok {
		pn.Logger.Warn("is_active类型错误")
		http.Error(w, "invalid is_active", http.StatusBadRequest)
		return
	}

	// ===== 2. 创建节点基础对象 =====
	node := types.NewNode(nodeID, initialWeight, performance, 20.0)
	node.CurrentWeight = currentWeight
	node.IsActive = isActive

	// ===== 3. 安全提取并赋值简单数值字段 =====
	// 使用辅助函数安全地提取数值
	node.SuccessfulVotes = safeGetInt(statusData, "successful_votes")
	node.FailedVotes = safeGetInt(statusData, "failed_votes")
	node.ValidBlocks = safeGetInt(statusData, "valid_blocks")
	node.DelayedBlocks = safeGetInt(statusData, "delayed_blocks")
	node.InvalidBlocks = safeGetInt(statusData, "invalid_blocks")
	node.CorrectValidations = safeGetInt(statusData, "correct_validations")
	node.TotalValidations = safeGetInt(statusData, "total_validations")
	node.ConsecutiveTerms = safeGetInt(statusData, "consecutive_terms")
	node.LastElectedRound = safeGetInt(statusData, "last_elected_round")

	node.TotalReward = safeGetFloat(statusData, "total_reward")
	node.TotalCompensation = safeGetFloat(statusData, "total_compensation")
	node.Deposit = safeGetFloat(statusData, "deposit")
	node.BlockEfficiency = safeGetFloat(statusData, "block_efficiency")
	node.ValidationEfficiency = safeGetFloat(statusData, "validation_efficiency")
	node.VotingSpeed = safeGetFloat(statusData, "voting_speed")
	node.VotingBehavior = safeGetFloat(statusData, "voting_behavior")
	node.RequiredWeight = safeGetFloat(statusData, "required_weight")
	node.OnlineTime = safeGetFloat(statusData, "online_time")
	node.DelegatedWeight = safeGetFloat(statusData, "delegated_weight")

	// ===== 4. 处理节点类型 =====
	if nodeTypeFloat, ok := statusData["type"].(float64); ok {
		node.Type = types.NodeType(int(nodeTypeFloat))
	}

	// ===== 5. 处理时间类型 (LastUpdateTime) =====
	if lastUpdateStr, ok := statusData["last_update_time"].(string); ok {
		if t, err := time.Parse(time.RFC3339, lastUpdateStr); err == nil {
			node.LastUpdateTime = t
		}
	}

	// ===== 6. 处理 map[int]bool 类型 (BlockProductionStatus) =====
	node.BlockProductionStatus = extractBlockProductionStatus(statusData)

	// ===== 7. 处理切片类型 (BlockHistory, VoteHistory, PerformanceHistory) =====
	node.BlockHistory = extractBlockHistory(statusData)
	node.VoteHistory = extractVoteHistory(statusData)
	node.PerformanceHistory = extractPerformanceHistory(statusData)

	// ===== 8. 更新节点信息 =====
	pn.LocalView.UpdatePeerInfo(nodeID, node)

	w.WriteHeader(http.StatusOK)
	pn.Logger.Info("成功更新节点 %s 的状态", nodeID)
}

// ===== 辅助函数：安全提取数值 =====

// safeGetInt 安全地从 map 中提取 int 值
func safeGetInt(data map[string]interface{}, key string) int {
	if val, ok := data[key]; ok {
		// JSON 解码的数字默认是 float64
		if floatVal, ok := val.(float64); ok {
			return int(floatVal)
		}
		// 直接是 int 的情况
		if intVal, ok := val.(int); ok {
			return intVal
		}
	}
	return 0 // 默认值
}

// safeGetFloat 安全地从 map 中提取 float64 值
func safeGetFloat(data map[string]interface{}, key string) float64 {
	if val, ok := data[key]; ok {
		if floatVal, ok := val.(float64); ok {
			return floatVal
		}
		// 如果是 int,转换为 float64
		if intVal, ok := val.(int); ok {
			return float64(intVal)
		}
	}
	return 0.0 // 默认值
}

func safeGetBool(data map[string]interface{}, key string) bool {
	if val, ok := data[key]; ok {
		if boolVal, ok := val.(bool); ok {
			return boolVal
		}
	}
	return false // 默认值
}

// ===== 辅助函数：提取复杂类型 =====

// extractBlockProductionStatus 提取出块状态 map[int]bool
func extractBlockProductionStatus(data map[string]interface{}) map[int]bool {
	result := make(map[int]bool)

	if statusRaw, ok := data["block_production_status"]; ok {
		// JSON 中的对象会被解码为 map[string]interface{}
		if statusMap, ok := statusRaw.(map[string]interface{}); ok {
			for key, val := range statusMap {
				// key 是字符串形式的数字,需要转换
				var roundID int
				fmt.Sscanf(key, "%d", &roundID)

				// val 是 bool
				if boolVal, ok := val.(bool); ok {
					result[roundID] = boolVal
				}
			}
		}
	}

	return result
}

// extractBlockHistory 提取出块历史
func extractBlockHistory(data map[string]interface{}) []types.BlockRecord {
	result := make([]types.BlockRecord, 0)

	if historyRaw, ok := data["block_history"]; ok {
		// JSON 数组会被解码为 []interface{}
		if historySlice, ok := historyRaw.([]interface{}); ok {
			for _, item := range historySlice {
				if recordMap, ok := item.(map[string]interface{}); ok {
					record := parseBlockRecord(recordMap)
					result = append(result, record)
				}
			}
		}
	}

	return result
}

// parseBlockRecord 解析单个 BlockRecord
func parseBlockRecord(data map[string]interface{}) types.BlockRecord {
	record := types.BlockRecord{}

	// 时间戳
	if tsStr, ok := data["timestamp"].(string); ok {
		if t, err := time.Parse(time.RFC3339, tsStr); err == nil {
			record.Timestamp = t
		}
	}

	// 数值字段
	record.BlockHeight = safeGetInt(data, "block_height")
	record.Success = safeGetBool(data, "success")
	record.RoundID = safeGetInt(data, "round_id")
	record.IsDelayed = safeGetBool(data, "is_delayed")
	record.IsInvalid = safeGetBool(data, "is_invalid")
	record.Reward = safeGetFloat(data, "reward")
	record.TransactionCount = safeGetInt(data, "transaction_count")

	// 出块耗时
	if ptFloat, ok := data["production_time"].(float64); ok {
		record.ProductionTime = time.Duration(ptFloat) * time.Millisecond
	}

	return record
}

// extractVoteHistory 提取投票历史
func extractVoteHistory(data map[string]interface{}) []types.VoteRecord {
	result := make([]types.VoteRecord, 0)

	if historyRaw, ok := data["vote_history"]; ok {
		if historySlice, ok := historyRaw.([]interface{}); ok {
			for _, item := range historySlice {
				if recordMap, ok := item.(map[string]interface{}); ok {
					record := parseVoteRecord(recordMap)
					result = append(result, record)
				}
			}
		}
	}

	return result
}

// parseVoteRecord 解析单个 VoteRecord
func parseVoteRecord(data map[string]interface{}) types.VoteRecord {
	record := types.VoteRecord{}

	// 时间戳
	if tsStr, ok := data["timestamp"].(string); ok {
		if t, err := time.Parse(time.RFC3339, tsStr); err == nil {
			record.Timestamp = t
		}
	}

	// 字符串字段
	if votedFor, ok := data["voted_for"].(string); ok {
		record.VotedFor = votedFor
	}
	if proxyFor, ok := data["proxy_for"].(string); ok {
		record.ProxyFor = proxyFor
	}

	record.RoundID = safeGetInt(data, "round_id")
	// 数值字段
	record.Weight = safeGetFloat(data, "weight")
	record.TimeCost = safeGetFloat(data, "time_cost")
	record.Compensation = safeGetFloat(data, "compensation")

	// 布尔字段
	if success, ok := data["success"].(bool); ok {
		record.Success = success
	}
	if isProxy, ok := data["is_proxy"].(bool); ok {
		record.IsProxy = isProxy
	}

	return record
}

// extractPerformanceHistory 提取性能历史
func extractPerformanceHistory(data map[string]interface{}) []types.PerformanceSnapshot {
	result := make([]types.PerformanceSnapshot, 0)

	if historyRaw, ok := data["performance_history"]; ok {
		if historySlice, ok := historyRaw.([]interface{}); ok {
			for _, item := range historySlice {
				if snapshotMap, ok := item.(map[string]interface{}); ok {
					snapshot := parsePerformanceSnapshot(snapshotMap)
					result = append(result, snapshot)
				}
			}
		}
	}

	return result
}

// parsePerformanceSnapshot 解析单个 PerformanceSnapshot
func parsePerformanceSnapshot(data map[string]interface{}) types.PerformanceSnapshot {
	snapshot := types.PerformanceSnapshot{}

	// 时间戳
	if tsStr, ok := data["timestamp"].(string); ok {
		if t, err := time.Parse(time.RFC3339, tsStr); err == nil {
			snapshot.Timestamp = t
		}
	}

	// 数值字段
	snapshot.PerformanceScore = safeGetFloat(data, "performance_score")
	snapshot.VotingSpeed = safeGetFloat(data, "voting_speed")
	snapshot.VotingBehavior = safeGetFloat(data, "voting_behavior")
	snapshot.BlockEfficiency = safeGetFloat(data, "block_efficiency")
	snapshot.OnlineScore = safeGetFloat(data, "online_score")

	return snapshot
}

// handlePeersRequest 处理节点列表请求
func (pn *P2PNode) handlePeersRequest(w http.ResponseWriter, r *http.Request) {
	peers := pn.PeerManager.GetActivePeers()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(peers)
}

// handleHeartbeat 处理心跳
func (pn *P2PNode) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	var heartbeatData map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&heartbeatData); err != nil {
		pn.Logger.Warn("解析心跳数据失败: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	nodeID, ok := heartbeatData["node_id"].(string)
	if !ok {
		http.Error(w, "invalid node_id", http.StatusBadRequest)
		return
	}

	address, ok := heartbeatData["address"].(string)
	if !ok {
		http.Error(w, "invalid address", http.StatusBadRequest)
		return
	}

	// 记录对等节点
	pn.PeerManager.AddPeer(&p2p.PeerInfo{
		NodeID:   nodeID,
		Address:  address,
		LastSeen: time.Now(),
		IsActive: true,
	})

	pn.Logger.Debug("收到来自 %s (%s) 的心跳", nodeID, address)

	// ===== 修复：立即返回当前已知的对等列表 =====
	peers := pn.PeerManager.GetActivePeers()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(peers)
}

// handleSyncRequest 处理同步请求
// handleSyncRequestMessage 处理同步请求消息
func (pn *P2PNode) handleSyncRequestMessage(w http.ResponseWriter, r *http.Request) {
	var syncReq network.SyncRequest
	if err := json.NewDecoder(r.Body).Decode(&syncReq); err != nil {
		pn.Logger.Warn("解析区块同步请求数据失败: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	pn.Logger.Info("收到来自节点的同步请求 [%d - %d]", syncReq.FromHeight, syncReq.ToHeight)

	// 使用SyncManager处理
	msg, err := pn.LocalView.SyncManager.HandleSyncRequest(syncReq)
	if err != nil {
		pn.Logger.Warn("处理同步请求失败: %v", err)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(msg)
}

// ================================
// 辅助方法
// ================================

// requestNodeStatus 请求节点状态
func (pn *P2PNode) requestNodeStatus(address string) (*NodeStatus, error) {
	var status NodeStatus
	err := pn.Transport.GetJSON(address, "/status", &status)
	return &status, err
}

// sendVoteMessage 发送投票消息
func (pn *P2PNode) sendVoteMessage(voteData map[string]interface{}) error {
	var wg sync.WaitGroup
	for _, peers := range pn.PeerManager.GetActivePeers() {
		go func() {
			wg.Add(1)
			defer wg.Done()
			peer, exists := pn.PeerManager.GetPeer(peers.NodeID)
			if !exists {
				pn.Logger.Error("目标节点 %s 不存在", peers.NodeID)
				return
			}
			pn.Logger.Info("发送投票信息给节点 %s, 地址 %s", peers.NodeID, peer.Address)
			_, err := pn.Transport.SendJSON(peer.Address, "/vote", voteData)
			if err != nil {
				pn.Logger.Error("发送投票信息给节点 %s 失败: %v", peers.NodeID, err)
				return
			}
		}()
	}
	wg.Wait()

	return nil
}

// broadcastBlock 广播区块
func (pn *P2PNode) broadcastBlock(block *network.Block) error {
	peers := pn.PeerManager.GetActivePeers()

	// 直接广播完整的区块结构，JSON序列化会自动处理时间格式
	err := pn.Transport.BroadcastJSON(peers, "/block", block)

	if err != nil {
		pn.Logger.Warn("广播区块失败: %v", err)
	} else {
		pn.Logger.Info("成功广播区块 #%d 到 %d 个节点", block.Height, len(peers))
	}

	return err
}

// sendStatusUpdate 发送状态更新
func (pn *P2PNode) sendStatusUpdate(address string, statusData map[string]interface{}) error {
	_, err := pn.Transport.SendJSON(address, "/status_update", statusData)
	return err
}

// GetStatus 获取节点状态
func (pn *P2PNode) GetStatus() map[string]interface{} {
	return map[string]interface{}{
		"id":                      pn.LocalView.LocalNode.ID,
		"type":                    pn.LocalView.LocalNode.Type.String(),
		"is_active":               pn.LocalView.LocalNode.IsActive,
		"current_weight":          pn.LocalView.LocalNode.CurrentWeight,
		"initial_weight":          pn.LocalView.LocalNode.InitialWeight,
		"performance_score":       pn.LocalView.LocalNode.PerformanceScore,
		"half_life":               pn.LocalView.LocalNode.GetHalfLife(),
		"successful_votes":        pn.LocalView.LocalNode.SuccessfulVotes,
		"failed_votes":            pn.LocalView.LocalNode.FailedVotes,
		"total_blocks":            len(pn.LocalView.LocalNode.BlockHistory),
		"valid_blocks":            pn.LocalView.LocalNode.ValidBlocks,
		"total_reward":            pn.LocalView.LocalNode.TotalReward,
		"total_compensation":      pn.LocalView.LocalNode.TotalCompensation,
		"is_running":              pn.IsRunning,
		"connected_peers":         pn.PeerManager.GetPeerCount(),
		"current_round":           pn.LocalView.CurrentRound,
		"block_height":            pn.LocalView.SyncManager.BlockPool.GetLatestHeight(),
		"pending_tx_count":        pn.LocalView.TxPool.GetPendingCount(),
		"deposit":                 pn.LocalView.LocalNode.Deposit,
		"consecutive_terms":       pn.LocalView.LocalNode.ConsecutiveTerms,
		"block_efficiency":        pn.LocalView.LocalNode.BlockEfficiency,
		"total_validations":       pn.LocalView.LocalNode.TotalValidations,
		"block_history":           pn.LocalView.LocalNode.BlockHistory,
		"block_production_status": pn.LocalView.LocalNode.BlockProductionStatus,
		"last_update_time":        pn.LocalView.LocalNode.LastUpdateTime,
		"correct_validations":     pn.LocalView.LocalNode.CorrectValidations,
		"delayed_blocks":          pn.LocalView.LocalNode.DelayedBlocks,
		"last_elected_round":      pn.LocalView.LocalNode.LastElectedRound,
		"invalid_blocks":          pn.LocalView.LocalNode.InvalidBlocks,
		"voting_speed":            pn.LocalView.LocalNode.VotingSpeed,
		"voting_behavior":         pn.LocalView.LocalNode.VotingBehavior,
		"validation_efficiency":   pn.LocalView.LocalNode.ValidationEfficiency,
		"performance_history":     pn.LocalView.LocalNode.PerformanceHistory,
		"required_weight":         pn.LocalView.LocalNode.RequiredWeight,
		"vote_history":            pn.LocalView.LocalNode.VoteHistory,
		"online_time":             pn.LocalView.LocalNode.OnlineTime,
		"delegated_weight":        pn.LocalView.LocalNode.DelegatedWeight,
	}
}

// NodeStatus 节点状态结构
type NodeStatus struct {
	NodeID            string  `json:"id"`
	NodeType          string  `json:"type"`
	IsActive          bool    `json:"is_active"`
	CurrentWeight     float64 `json:"current_weight"`
	InitialWeight     float64 `json:"initial_weight"`
	Performance       float64 `json:"performance_score"`
	HalfLife          float64 `json:"half_life"`
	SuccessfulVotes   int     `json:"successful_votes"`
	FailedVotes       int     `json:"failed_votes"`
	TotalBlocks       int     `json:"total_blocks"`
	ValidBlocks       int     `json:"valid_blocks"`
	TotalReward       float64 `json:"total_reward"`
	TotalCompensation float64 `json:"total_compensation"`
	IsRunning         bool    `json:"is_running"`
	ConnectedPeers    int     `json:"connected_peers"`
	CurrentRound      int     `json:"current_round"`
	BlockHeight       int     `json:"block_height"`
	PendingTxCount    int     `json:"pending_tx_count"`
	//new add
	Deposit               float64                     `json:"deposit"`
	ConsecutiveTerms      int                         `json:"consecutive_terms"`
	BlockEfficiency       float64                     `json:"block_efficiency"`
	TotalValidations      int                         `json:"total_validations"`
	BlockHistory          []types.BlockRecord         `json:"block_history"`
	BlockProductionStatus map[int]bool                `json:"block_production_status"`
	LastUpdateTime        time.Time                   `json:"last_update_time"`
	CorrectValidations    int                         `json:"correct_validations"`
	DelayedBlocks         int                         `json:"delayed_blocks"`
	LastElectedRound      int                         `json:"last_elected_round"`
	InvalidBlocks         int                         `json:"invalid_blocks"`
	VotingSpeed           float64                     `json:"voting_speed"`
	VotingBehavior        float64                     `json:"voting_behavior"`
	ValidationEfficiency  float64                     `json:"validation_efficiency"`
	PerformanceHistory    []types.PerformanceSnapshot `json:"performance_history"`
	RequiredWeight        float64                     `json:"required_weight"`
	VoteHistory           []types.VoteRecord          `json:"vote_history"`
	OnlineTime            float64                     `json:"online_time"`
	DelegatedWeight       float64                     `json:"delegated_weight"`
}

// 辅助函数
func extractTxIDs(txs []*network.Transaction) []string {
	ids := make([]string, len(txs))
	for i, tx := range txs {
		ids[i] = tx.ID
	}
	return ids
}

// checkHTTPServerReady 检查HTTP服务器是否就绪
func (pn *P2PNode) checkHTTPServerReady() bool {
	client := &http.Client{Timeout: 1 * time.Second}

	for i := 0; i < 10; i++ {
		resp, err := client.Get(fmt.Sprintf("http://%s/status", pn.ListenAddress))
		if err == nil {
			resp.Body.Close()
			return true
		}
		time.Sleep(500 * time.Millisecond)
	}

	return false
}

// ================================
// 处理投票结果 - 完整版
// ================================

// 在出块阶段后调用
func (pn *P2PNode) processVoteResultsAfterBlockProduction() {
	session := pn.LocalView.VotingSession
	if session == nil {
		return
	}

	pn.Logger.Info("【处理投票结果 - 出块后】")

	localNode := pn.LocalView.LocalNode

	// 情况A: 未投票 (已在投票阶段处理)
	if len(localNode.VoteHistory) == 0 {
		// 情况A: 未投票
		deducted := core.HandleVoteResultA(localNode)
		pn.Logger.Warn("情况A: 未投票，扣除 %.2f", deducted)
		return
	}

	lastVote := localNode.VoteHistory[len(localNode.VoteHistory)-1]

	// 检查投票的节点是否当选代理
	isElectedDelegate := false
	var votedDelegate *types.Node
	var number = -1
	for i, delegate := range pn.LocalView.CurrentDelegates {
		if delegate.ID == lastVote.VotedFor {
			isElectedDelegate = true
			votedDelegate = delegate
			number = i
			break
		}
	}

	// 情况B: 投票但节点未当选代理
	if !isElectedDelegate {
		// 情况B: 投票但节点未当选代理
		released := core.HandleVoteResultB(localNode)
		pn.Logger.Info("情况B: 投票节点未当选，退回保证金=%.2f", released)
		return
	}

	// 情况B续: 投票但节点未在本轮是出块节点
	if number != -1 && number+1 != pn.Scheduler.CurrentHeight%config.NumDelegates {
		// 情况B: 投票但节点未当选代理
		released := core.HandleVoteResultB(localNode)
		pn.Logger.Info("情况B: 投票节点在本轮未当选出块节点，退回保证金=%.2f", released)
		return
	}

	//  检查代理节点是否成功出块
	blockSuccess := votedDelegate.HasProducedBlockInRound(pn.LocalView.CurrentRound)

	if blockSuccess {
		// 情况C: 投票且代理节点成功出块
		deposit := core.ReleaseDeposit(localNode)
		pn.Logger.Info("情况C: 代理出块成功，退回保证金=%.2f", deposit)

		// 热补偿已在 distributeHotCompensation 中分配
		if localNode.TotalCompensation > 0 {
			pn.Logger.Info("  + 热补偿: %.2f", localNode.TotalCompensation)
		}
	} else {
		// 情况D: 投票但代理节点未出块
		result := core.HandleVoteResultD(localNode)
		pn.Logger.Warn("情况D: 代理出块失败，部分扣除保证金，退回=%.2f", result)
	}

	// 步骤6: Shapley值分配(投票给成功出块代理的节点)
	if blockSuccess {
		pn.distributeShapleyRewards(votedDelegate)
	}
}

// Shapley值奖励分配
func (pn *P2PNode) distributeShapleyRewards(successfulDelegate *types.Node) {
	session := pn.LocalView.VotingSession
	if session == nil {
		return
	}

	pn.Logger.Info("【步骤6: Shapley值分配】")

	// 1. 找出投票给该代理节点的所有节点(联盟成员)
	coalition := make([]*types.Node, 0)
	for voterID, targetID := range session.VoteRecords {
		if targetID == successfulDelegate.ID {
			// 找到投票者节点
			for _, node := range session.Voters {
				if node.ID == voterID {
					coalition = append(coalition, node)
					break
				}
			}
		}
	}

	if len(coalition) == 0 {
		pn.Logger.Info("该代理节点无投票者,跳过Shapley分配")
		return
	}

	pn.Logger.Info("联盟成员数: %d (投票给 %s)", len(coalition), successfulDelegate.ID)

	// 2. 计算该代理节点的区块奖励
	blockReward := config.BlockReward // 简化: 使用固定奖励

	// 3. 使用修正Shapley值分配
	rewards := core.DistributeBlockReward(blockReward, coalition)

	// 4. 应用奖励
	for nodeID, reward := range rewards {
		if nodeID == pn.LocalView.LocalNode.ID {
			pn.LocalView.LocalNode.CurrentWeight += reward
			pn.LocalView.LocalNode.TotalReward += reward
			pn.Logger.Info("  ✅ 获得Shapley奖励: %.2f", reward)
		}
	}
}

// ================================
// 代理节点选举 - 添加连任检查
// ================================

func (pn *P2PNode) electDelegatesWithReelectionCheck() []*types.Node {
	pn.Logger.Info("========================================")
	pn.Logger.Info("【代理节点选举 - 连任检查】")
	pn.Logger.Info("========================================")

	session := pn.LocalView.VotingSession
	if session == nil {
		return nil
	}

	// 1. 按权重排序选出候选人
	candidates := pn.LocalView.GetActiveNodes()

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].CurrentWeight == candidates[j].CurrentWeight {
			if candidates[i].PerformanceScore == candidates[j].PerformanceScore {
				number1, _ := strconv.Atoi(strings.Split(candidates[i].ID, "-")[1])
				number2, _ := strconv.Atoi(strings.Split(candidates[j].ID, "-")[1])
				return number1 > number2
			} else {
				return candidates[i].PerformanceScore > candidates[j].PerformanceScore
			}
		}
		return candidates[i].CurrentWeight > candidates[j].CurrentWeight
	})

	// 2. ✅ 检查连任条件
	eligibleCandidates := make([]*types.Node, 0)
	for _, candidate := range candidates {
		// 根据所有的节点，找到节点是否是当前的代理节点如果是的话就进行后续处理
		// 如果是现任代理节点,检查连任条件
		isCurrentDelegate := false
		for _, delegate := range pn.LocalView.CurrentDelegates {
			if delegate.ID == candidate.ID {
				isCurrentDelegate = true
				break
			}
		}

		if isCurrentDelegate {
			// 检查是否满足连任条件
			canReelect, gap := core.CanBeReelected(candidate)

			pn.Logger.Info("连任检查: %s", candidate.ID)
			pn.Logger.Info("节点类型: %s", candidate.Type.String())
			pn.Logger.Info("  连任次数: %d", candidate.ConsecutiveTerms)
			pn.Logger.Info("  所需权重: %.2f", core.CalculateRequiredDelegatedWeight(candidate))
			pn.Logger.Info("  实际委托: %.2f", candidate.DelegatedWeight)
			pn.Logger.Info("  权重缺口: %.2f", gap)

			if canReelect {
				pn.Logger.Info("  ✅ 满足连任条件")
				eligibleCandidates = append(eligibleCandidates, candidate)
				candidate.IncrementConsecutiveTerms(pn.LocalView.CurrentRound)
				if pn.LocalView.LocalNode.ID == candidate.ID {
					pn.LocalView.LocalNode.IncrementConsecutiveTerms(pn.LocalView.CurrentRound)
				}
			} else {
				pn.Logger.Warn("  ❌ 不满足连任条件,将被淘汰")
				candidate.ResetConsecutiveTerms()
			}
		} else {
			// 新候选人,直接加入
			eligibleCandidates = append(eligibleCandidates, candidate)
		}
	}

	// 3. 从符合条件的候选人中选出前N个
	numDelegates := config.NumDelegates
	if len(eligibleCandidates) < numDelegates {
		numDelegates = len(eligibleCandidates)
	}

	delegates := make([]*types.Node, numDelegates)
	for i := 0; i < numDelegates; i++ {
		delegates[i] = eligibleCandidates[i]
		delegates[i].Type = types.DelegateNode
		delegates[i].DelegatedWeight = 0 // 重置,等待新一轮投票
	}

	// 4. 未当选的节点重置连任次数
	for _, candidate := range candidates {
		isElected := false
		for _, delegate := range delegates {
			if delegate.ID == candidate.ID {
				if pn.LocalView.LocalNode.ID == candidate.ID {
					pn.LocalView.LocalNode.Type = types.DelegateNode
				}
				isElected = true
				candidate.Type = types.DelegateNode
				//fmt.Println("11111111111111111111111111111111111", candidate.ID, candidate.Type.String())
				break
			}
		}
		if !isElected && candidate.Type == types.DelegateNode {
			//fmt.Println("222222222222222222222重置", candidate.ID, candidate.Type.String())
			candidate.ResetConsecutiveTerms()
			if pn.LocalView.LocalNode.ID == candidate.ID {
				pn.LocalView.LocalNode.ResetConsecutiveTerms()
			}
			candidate.Type = types.VoterNode // 降级为投票节点
			if pn.LocalView.LocalNode.ID == candidate.ID {
				pn.LocalView.LocalNode.Type = types.VoterNode
			}
		}
	}

	for _, candidate := range candidates {
		if candidate.ID == pn.LocalView.LocalNode.ID {
			pn.LocalView.LocalNode.Type = candidate.Type
		}
	}

	pn.LocalView.CurrentDelegates = delegates
	pn.LocalView.Network.SetDelegates(delegates)

	//更新调度器
	pn.Scheduler.UpdateDelegates(delegates)

	pn.Logger.Info("选出 %d 个代理节点:", len(delegates))
	for i, d := range delegates {
		pn.Logger.Info("  #%d: %s (权重: %.2f, 连任: %d次)",
			i+1, d.ID, d.CurrentWeight, d.ConsecutiveTerms)
	}
	pn.Logger.Info("========================================")

	return delegates
}

// ================================
// HTTP处理器 - 验证者奖励
// ================================

func (pn *P2PNode) handleValidatorReward(w http.ResponseWriter, r *http.Request) {
	var rewardData struct {
		ValidatorID string  `json:"validator_id"`
		Reward      float64 `json:"reward"`
		Round       int     `json:"round"`
		Timestamp   int64   `json:"timestamp"`
	}

	if err := json.NewDecoder(r.Body).Decode(&rewardData); err != nil {
		pn.Logger.Warn("解析验证者奖励失败: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	pn.Logger.Info("【收到验证者奖励】")
	pn.Logger.Info("  奖励金额: %.2f | 轮次: %d", rewardData.Reward, rewardData.Round)

	if pn.LocalView.LocalNode.ID == rewardData.ValidatorID {
		pn.LocalView.LocalNode.CurrentWeight += rewardData.Reward
		pn.LocalView.LocalNode.RecordBlockReward(rewardData.Round, rewardData.Reward)
		pn.Logger.Info("  ✅ 应用验证者奖励: %.2f", rewardData.Reward)
	}

	w.WriteHeader(http.StatusOK)
}
