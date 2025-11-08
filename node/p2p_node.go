package node

import (
	"encoding/json"
	"fmt"
	"net/http"
	"nm-dpos/config"
	"nm-dpos/core"
	"nm-dpos/network"
	"nm-dpos/p2p"
	"nm-dpos/types"
	"nm-dpos/utils"
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

	// 配置
	ListenAddress string
	SeedNodes     []string

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
		SeedNodes:        seedNodes,
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

	// 2. 启动节点发现
	pn.DiscoveryService.Start()

	// 3. 启动同步管理器的定期清理
	pn.LocalView.SyncManager.StartPeriodicCleanup()

	// 4. 等待连接到网络
	time.Sleep(3 * time.Second)

	// 5. 启动主循环
	go pn.mainLoop()

	pn.Logger.Info("节点 %s 启动完成，已连接 %d 个对等节点",
		pn.LocalView.LocalNode.ID,
		pn.PeerManager.GetPeerCount())

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

// mainLoop 主循环
func (pn *P2PNode) mainLoop() {
	roundTicker := time.NewTicker(time.Duration(config.BlockInterval) * time.Second)
	syncTicker := time.NewTicker(10 * time.Second)
	defer roundTicker.Stop()
	defer syncTicker.Stop()

	for pn.IsRunning {
		select {
		case <-roundTicker.C:
			pn.processRound()

		case <-syncTicker.C:
			pn.checkSync()

		case <-pn.StopChan:
			return
		}
	}
}

// ================================
// 共识流程（使用core/算法）
// ================================

// processRound 处理一轮共识
func (pn *P2PNode) processRound() {
	pn.LocalView.CurrentRound++
	pn.Logger.Debug("节点 %s 处理第 %d 轮", pn.LocalView.LocalNode.ID, pn.LocalView.CurrentRound)

	// 1. 同步网络状态（从P2P网络获取）
	pn.syncPeerStates()

	// 2. 更新权重（使用core/decay.go）
	pn.updateWeight()

	// 3. 选举代理节点（基于本地视图）
	delegates := pn.LocalView.SelectDelegatesLocally()
	pn.Logger.Debug("选出 %d 个代理节点", len(delegates))

	// 4. 参与投票（如果是投票节点）
	if !pn.LocalView.IsLocalDelegate() {
		pn.vote()
	}

	// 5. 出块（如果是代理节点）
	if pn.LocalView.IsLocalDelegate() {
		pn.produceBlock()
	}

	// 6. 更新表现评分（使用core/performance.go）
	pn.updatePerformance()

	// 7. 广播自己的状态
	pn.broadcastStatus()
}

// syncPeerStates 同步对等节点状态
func (pn *P2PNode) syncPeerStates() {
	peers := pn.PeerManager.GetActivePeers()

	for _, peer := range peers {
		go func(p *p2p.PeerInfo) {
			status, err := pn.requestNodeStatus(p.Address)
			if err != nil {
				return
			}

			// 更新本地视图
			node := types.NewNode(
				status.NodeID,
				status.InitialWeight,
				status.Performance,
				20.0,
			)
			node.CurrentWeight = status.CurrentWeight
			node.IsActive = status.IsActive
			node.Type = status.NodeType

			pn.LocalView.UpdatePeerInfo(status.NodeID, node)

			// 更新PeerManager
			pn.PeerManager.UpdatePeerStatus(
				status.NodeID,
				status.IsActive,
				status.CurrentWeight,
				status.Performance,
			)
		}(peer)
	}
}

// updateWeight 更新权重（使用core/decay.go）
func (pn *P2PNode) updateWeight() {
	pn.LocalView.UpdateLocalWeight()

	pn.Logger.Debug("节点 %s 权重: %.2f (表现: %.3f, 半衰期: %.1f)",
		pn.LocalView.LocalNode.ID,
		pn.LocalView.LocalNode.CurrentWeight,
		pn.LocalView.LocalNode.PerformanceScore,
		pn.LocalView.LocalNode.GetHalfLife())
}

// vote 参与投票
func (pn *P2PNode) vote() {
	if !pn.LocalView.LocalNode.IsActive {
		return
	}

	// 选择投票目标
	target := pn.LocalView.GetVotingTarget()
	if target == nil {
		pn.Logger.Warn("没有可投票的代理节点")
		return
	}

	// 构造投票数据
	voteData := map[string]interface{}{
		"voter_id":  pn.LocalView.LocalNode.ID,
		"target_id": target.ID,
		"weight":    pn.LocalView.LocalNode.CurrentWeight,
		"round":     pn.LocalView.CurrentRound,
		"timestamp": time.Now().Unix(),
	}

	// 发送投票
	if err := pn.sendVoteMessage(target.ID, voteData); err != nil {
		pn.Logger.Warn("投票发送失败: %v", err)
		pn.LocalView.LocalNode.RecordVote("", pn.LocalView.LocalNode.CurrentWeight, 0, 0, false, false, "")
	} else {
		pn.Logger.Info("节点 %s 投票给 %s (权重: %.2f)",
			pn.LocalView.LocalNode.ID, target.ID, pn.LocalView.LocalNode.CurrentWeight)
		pn.LocalView.LocalNode.RecordVote(target.ID, pn.LocalView.LocalNode.CurrentWeight, 0, 0, true, false, "")
	}
}

// produceBlock 生产区块
func (pn *P2PNode) produceBlock() {
	if !pn.LocalView.LocalNode.IsActive {
		return
	}

	pn.Logger.Info("节点 %s 开始出块", pn.LocalView.LocalNode.ID)

	// 1. 从交易池获取交易
	txs := pn.LocalView.TxPool.GetPendingTransactions(100)

	// 2. 创建区块
	prevBlock := pn.LocalView.SyncManager.BlockPool.GetLatestBlock()
	prevHash := ""
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

	// 3. 添加到本地区块池
	if err := pn.LocalView.SyncManager.BlockPool.AddBlock(block); err != nil {
		pn.Logger.Warn("添加区块失败: %v", err)
		core.RecordBlockProduction(pn.LocalView.LocalNode, block.Height, false, false, true)
		return
	}

	// 4. 广播区块
	if err := pn.broadcastBlock(block); err != nil {
		pn.Logger.Warn("广播区块失败: %v", err)
	} else {
		pn.Logger.Info("节点 %s 成功出块 #%d (包含 %d 笔交易)",
			pn.LocalView.LocalNode.ID, block.Height, len(txs))
		core.RecordBlockProduction(pn.LocalView.LocalNode, block.Height, true, false, false)
	}

	// 5. 移除已打包的交易
	txIDs := extractTxIDs(txs)
	pn.LocalView.TxPool.RemoveTransactions(txIDs)
}

// updatePerformance 更新表现评分（使用core/performance.go）
func (pn *P2PNode) updatePerformance() {
	pn.LocalView.UpdateLocalPerformance()
}

// broadcastStatus 广播节点状态
func (pn *P2PNode) broadcastStatus() {
	statusData := map[string]interface{}{
		"node_id":        pn.LocalView.LocalNode.ID,
		"is_active":      pn.LocalView.LocalNode.IsActive,
		"current_weight": pn.LocalView.LocalNode.CurrentWeight,
		"initial_weight": pn.LocalView.LocalNode.InitialWeight,
		"performance":    pn.LocalView.LocalNode.PerformanceScore,
		"node_type":      pn.LocalView.LocalNode.Type,
		"is_delegate":    pn.LocalView.IsLocalDelegate(),
		"round":          pn.LocalView.CurrentRound,
		"timestamp":      time.Now().Unix(),
	}

	peers := pn.PeerManager.GetActivePeers()
	for _, peer := range peers {
		go pn.sendStatusUpdate(peer.Address, statusData)
	}
}

// checkSync 检查并执行同步
func (pn *P2PNode) checkSync() {
	localHeight := pn.LocalView.SyncManager.BlockPool.GetLatestHeight()

	peerIDs := make([]string, 0)
	for _, peer := range pn.PeerManager.GetActivePeers() {
		peerIDs = append(peerIDs, peer.NodeID)
	}

	synced, err := pn.LocalView.SyncManager.CheckAndSync(
		pn.LocalView.LocalNode.ID,
		peerIDs,
		localHeight,
		pn.Transport,
		pn.PeerManager,
	)

	if err != nil {
		pn.Logger.Warn("同步检查失败: %v", err)
	}

	if synced {
		pn.Logger.Info("节点 %s 开始同步区块", pn.LocalView.LocalNode.ID)
	}
}

// ================================
// HTTP服务器（接收其他节点的消息）
// ================================

func (pn *P2PNode) startHTTPServer() error {
	mux := http.NewServeMux()

	// 处理投票消息
	mux.HandleFunc("/vote", pn.handleVoteRequest)
	// 处理区块消息
	mux.HandleFunc("/block", pn.handleBlockRequest)
	// 处理状态查询
	mux.HandleFunc("/status", pn.handleStatusRequest)
	// 处理状态更新
	mux.HandleFunc("/status_update", pn.handleStatusUpdate)

	mux.HandleFunc("/transaction", pn.handleTransactionRequest)

	// 其他端点
	mux.HandleFunc("/status", pn.handleStatusRequest)
	mux.HandleFunc("/peers", pn.handlePeersRequest)
	mux.HandleFunc("/heartbeat", pn.handleHeartbeat)

	pn.HTTPServer = &http.Server{
		Addr:    pn.ListenAddress,
		Handler: mux,
	}

	go func() {
		if err := pn.HTTPServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			pn.Logger.Error("HTTP服务器错误: %v", err)
		}
	}()

	return nil
}

// handleVote 处理投票消息
func (pn *P2PNode) handleVoteRequest(w http.ResponseWriter, r *http.Request) {
	var voteData struct {
		VoterID   string  `json:"voter_id"`
		TargetID  string  `json:"target_id"`
		Weight    float64 `json:"weight"`
		Round     int     `json:"round"`
		Timestamp int64   `json:"timestamp"`
	}

	if err := json.NewDecoder(r.Body).Decode(&voteData); err != nil {
		pn.Logger.Warn("解析投票数据失败: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	pn.Logger.Info("收到来自 %s 的投票，目标: %s，权重: %.2f",
		voteData.VoterID, voteData.TargetID, voteData.Weight)

	//如果是代理节点，记录已经收到的投票信息
	if pn.LocalView.IsLocalDelegate() {
		pn.LocalView.LocalNode.DelegatedWeight += voteData.Weight
	}
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

	nodeID := statusData["node_id"].(string)

	// 更新本地视图
	node := types.NewNode(
		nodeID,
		statusData["initial_weight"].(float64),
		statusData["performance"].(float64),
		20.0,
	)
	node.CurrentWeight = statusData["current_weight"].(float64)
	node.IsActive = statusData["is_active"].(bool)

	if nodeTypeFloat, ok := statusData["node_type"].(float64); ok {
		node.Type = types.NodeType(int(nodeTypeFloat))
	}

	pn.LocalView.UpdatePeerInfo(nodeID, node)

	w.WriteHeader(http.StatusOK)
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

	nodeID := heartbeatData["node_id"].(string)
	address := heartbeatData["address"].(string)

	pn.PeerManager.AddPeer(&p2p.PeerInfo{
		NodeID:   nodeID,
		Address:  address,
		LastSeen: time.Now(),
		IsActive: true,
	})

	w.WriteHeader(http.StatusOK)
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

	msg, err := pn.LocalView.SyncManager.HandleSyncRequest(syncReq)
	// 使用SyncManager处理
	if err != nil {
		pn.Logger.Warn("处理同步请求失败: %v", err)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(msg)
}

// handleSyncResponseMessage 处理同步响应消息
func (pn *P2PNode) handleSyncResponseMessage(w http.ResponseWriter, r *http.Request) {
	var syncResp network.SyncResponse
	if err := json.NewDecoder(r.Body).Decode(&syncResp); err != nil {
		pn.Logger.Warn("解析处理同步请求数据失败: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := pn.LocalView.SyncManager.HandleSyncResponse(syncResp, pn.Transport, pn.PeerManager); err != nil {
		pn.Logger.Warn("处理同步响应失败: %v", err)
	}
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
func (pn *P2PNode) sendVoteMessage(targetID string, voteData map[string]interface{}) error {
	peer, exists := pn.PeerManager.GetPeer(targetID)
	if !exists {
		return fmt.Errorf("目标节点 %s 不存在", targetID)
	}

	return pn.Transport.SendJSON(peer.Address, "/vote", voteData)
}

// broadcastBlock 广播区块
func (pn *P2PNode) broadcastBlock(block *network.Block) error {
	blockData := map[string]interface{}{
		"producer_id": block.ProducerID,
		"height":      block.Height,
		"hash":        block.Hash,
		"prev_hash":   block.PrevHash,
		"timestamp":   block.Timestamp.Unix(),
		"tx_count":    len(block.Transactions),
	}

	peers := pn.PeerManager.GetActivePeers()
	return pn.Transport.BroadcastJSON(peers, "/block", blockData)
}

// sendStatusUpdate 发送状态更新
func (pn *P2PNode) sendStatusUpdate(address string, statusData map[string]interface{}) error {
	return pn.Transport.SendJSON(address, "/status_update", statusData)
}

// GetStatus 获取节点状态
func (pn *P2PNode) GetStatus() map[string]interface{} {
	return map[string]interface{}{
		"id":                 pn.LocalView.LocalNode.ID,
		"type":               pn.LocalView.LocalNode.Type.String(),
		"is_active":          pn.LocalView.LocalNode.IsActive,
		"current_weight":     pn.LocalView.LocalNode.CurrentWeight,
		"initial_weight":     pn.LocalView.LocalNode.InitialWeight,
		"performance_score":  pn.LocalView.LocalNode.PerformanceScore,
		"half_life":          pn.LocalView.LocalNode.GetHalfLife(),
		"successful_votes":   pn.LocalView.LocalNode.SuccessfulVotes,
		"failed_votes":       pn.LocalView.LocalNode.FailedVotes,
		"total_blocks":       len(pn.LocalView.LocalNode.BlockHistory),
		"valid_blocks":       pn.LocalView.LocalNode.ValidBlocks,
		"total_reward":       pn.LocalView.LocalNode.TotalReward,
		"total_compensation": pn.LocalView.LocalNode.TotalCompensation,
		"is_running":         pn.IsRunning,
		"connected_peers":    pn.PeerManager.GetPeerCount(),
		"current_round":      pn.LocalView.CurrentRound,
		"block_height":       pn.LocalView.SyncManager.BlockPool.GetLatestHeight(),
		"pending_tx_count":   pn.LocalView.TxPool.GetPendingCount(),
	}
}

// NodeStatus 节点状态结构
type NodeStatus struct {
	NodeID        string
	IsActive      bool
	CurrentWeight float64
	InitialWeight float64
	Performance   float64
	NodeType      types.NodeType
}

// 辅助函数
func extractTxIDs(txs []*network.Transaction) []string {
	ids := make([]string, len(txs))
	for i, tx := range txs {
		ids[i] = tx.ID
	}
	return ids
}
