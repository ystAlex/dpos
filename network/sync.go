package network

import (
	"encoding/json"
	"fmt"
	"nm-dpos/p2p"
	"nm-dpos/types"
	"nm-dpos/utils"
	"sync"
	"time"
)

// sync.go
// 完整的区块同步机制

// ================================
// 同步请求和响应数据结构
// ================================

// SyncRequest 同步请求数据
type SyncRequest struct {
	FromID     string    // 发起请求的节点
	ToID       string    //请求到的节点
	FromHeight int       // 请求的起始区块高度
	ToHeight   int       // 请求的结束区块高度（0表示最新）
	RequestID  string    // 请求唯一标识
	Timestamp  time.Time // 请求时间
}

// SyncResponse 同步响应数据
type SyncResponse struct {
	FromID       string //响应的节点
	ToID         string
	RequestID    string    // 对应的请求ID
	Blocks       []*Block  // 返回的区块列表
	LatestHeight int       // 当前最新区块高度
	HasMore      bool      // 是否还有更多区块
	Timestamp    time.Time // 响应时间
}

// SyncManager 同步管理器
type SyncManager struct {
	LocalNode       *types.Node
	BlockPool       *BlockPool
	Logger          *utils.Logger
	PendingRequests map[string]*SyncRequest // requestID -> request
	SyncTimeout     time.Duration           // 同步超时时间
	mu              sync.RWMutex
}

// NewSyncManager 创建同步管理器
func NewSyncManager(localNode *types.Node, blockPool *BlockPool, logger *utils.Logger) *SyncManager {
	return &SyncManager{
		LocalNode:       localNode,
		BlockPool:       blockPool,
		Logger:          logger,
		PendingRequests: make(map[string]*SyncRequest),
		SyncTimeout:     30 * time.Second,
	}
}

// ================================
// 同步请求处理（节点A请求数据）
// ================================

// RequestSync 请求同步区块
func (sm *SyncManager) RequestSync(nodeID, targetNodeID string, fromHeight, toHeight int, transport *p2p.HTTPTransport, peerManager *p2p.PeerManager) (string, error) {
	// 创建同步请求
	requestID := fmt.Sprintf("sync-%s-%d", nodeID, time.Now().Unix())

	syncReq := &SyncRequest{
		FromID:     nodeID,
		ToID:       targetNodeID,
		FromHeight: fromHeight,
		ToHeight:   toHeight,
		RequestID:  requestID,
		Timestamp:  time.Now(),
	}

	// 序列化请求数据
	payload, err := json.Marshal(syncReq)
	if err != nil {
		return "", fmt.Errorf("序列化同步请求失败: %v", err)
	}

	peer, exists := peerManager.GetPeer(targetNodeID)
	if !exists {
		return "", fmt.Errorf("目标节点 %s 不存在", targetNodeID)
	}

	if err = transport.SendJSON(peer.Address, "/sync", payload); err != nil {
		return "", fmt.Errorf("发送同步请求失败: %v", err)
	}

	// 记录待处理请求
	sm.mu.Lock()
	sm.PendingRequests[requestID] = syncReq
	sm.mu.Unlock()

	sm.Logger.Info("节点 %s 向 %s 请求同步区块 [%d - %d]",
		nodeID, targetNodeID, fromHeight, toHeight)

	return requestID, nil
}

// HandleSyncRequest 处理收到的同步请求
func (sm *SyncManager) HandleSyncRequest(syncReq SyncRequest) ([]byte, error) {

	// 从区块池获取请求的区块
	blocks, latestHeight, hasMore := sm.getBlocksForSync(&syncReq)

	// 创建响应
	syncResp := &SyncResponse{
		FromID:       sm.LocalNode.ID,
		ToID:         syncReq.FromID,
		RequestID:    syncReq.RequestID,
		Blocks:       blocks,
		LatestHeight: latestHeight,
		HasMore:      hasMore,
		Timestamp:    time.Now(),
	}

	// 序列化响应数据
	payload, err := json.Marshal(syncResp)
	if err != nil {
		return nil, fmt.Errorf("序列化同步响应失败: %v", err)
	}

	sm.Logger.Info("向 %s 发送同步响应，包含 %d 个区块", syncReq.FromID, len(blocks))

	return payload, nil
}

// getBlocksForSync 获取用于同步的区块
func (sm *SyncManager) getBlocksForSync(req *SyncRequest) ([]*Block, int, bool) {
	const maxBlocksPerResponse = 100 // 每次最多返回100个区块

	blocks := make([]*Block, 0)
	latestBlock := sm.BlockPool.GetLatestBlock()

	if latestBlock == nil {
		return blocks, 0, false
	}

	latestHeight := latestBlock.Height

	// 确定实际的结束高度
	endHeight := req.ToHeight
	if endHeight == 0 || endHeight > latestHeight {
		endHeight = latestHeight
	}

	// 限制返回的区块数量
	if endHeight-req.FromHeight > maxBlocksPerResponse {
		endHeight = req.FromHeight + maxBlocksPerResponse
	}

	// 获取区块
	for height := req.FromHeight; height <= endHeight; height++ {
		block, err := sm.BlockPool.GetBlock(height)
		if err != nil {
			sm.Logger.Warn("获取区块 %d 失败: %v", height, err)
			break
		}
		blocks = append(blocks, block)
	}

	// 判断是否还有更多区块
	hasMore := endHeight < latestHeight

	return blocks, latestHeight, hasMore
}

// ================================
// 同步响应处理（节点A接收数据）
// ================================

// HandleSyncResponse 处理收到的同步响应
func (sm *SyncManager) HandleSyncResponse(syncResp SyncResponse, transport *p2p.HTTPTransport, peerManager *p2p.PeerManager) error {
	sm.Logger.Info("收到来自 %s 的同步响应，包含 %d 个区块",
		syncResp.FromID, len(syncResp.Blocks))

	// 验证请求是否存在
	sm.mu.RLock()
	req, exists := sm.PendingRequests[syncResp.RequestID]
	sm.mu.RUnlock()

	if !exists {
		sm.Logger.Warn("收到未知请求ID的同步响应: %s", syncResp.RequestID)
		return nil
	}

	// 应用收到的区块
	successCount := 0
	for _, block := range syncResp.Blocks {
		if err := sm.applyBlock(block); err != nil {
			sm.Logger.Warn("应用区块 %d 失败: %v", block.Height, err)
			continue
		}
		successCount++
	}

	sm.Logger.Info("成功同步 %d/%d 个区块", successCount, len(syncResp.Blocks))

	// 如果还有更多区块，继续请求
	if syncResp.HasMore {
		nextFromHeight := req.FromHeight + len(syncResp.Blocks)
		sm.Logger.Info("继续请求更多区块，起始高度: %d", nextFromHeight)

		_, err := sm.RequestSync(
			syncResp.ToID,
			syncResp.FromID,
			nextFromHeight,
			req.ToHeight,
			transport,
			peerManager,
		)
		if err != nil {
			sm.Logger.Warn("继续同步失败: %v", err)
		}
	} else {
		sm.Logger.Info("区块同步完成，当前高度: %d", syncResp.LatestHeight)
	}

	// 清理已完成的请求
	sm.mu.Lock()
	delete(sm.PendingRequests, syncResp.RequestID)
	sm.mu.Unlock()

	return nil
}

// applyBlock 应用区块到本地区块池
func (sm *SyncManager) applyBlock(block *Block) error {
	// 验证区块
	if err := sm.validateBlock(block); err != nil {
		return fmt.Errorf("区块验证失败: %v", err)
	}

	// 添加到区块池
	if err := sm.BlockPool.AddBlock(block); err != nil {
		return fmt.Errorf("添加区块失败: %v", err)
	}

	sm.Logger.Debug("成功应用区块 #%d (生产者: %s)", block.Height, block.ProducerID)

	return nil
}

// validateBlock 验证区块有效性
func (sm *SyncManager) validateBlock(block *Block) error {
	// 1. 验证区块高度
	latestBlock := sm.BlockPool.GetLatestBlock()
	if latestBlock != nil && block.Height != latestBlock.Height+1 {
		return fmt.Errorf("区块高度不连续: 期望 %d, 实际 %d",
			latestBlock.Height+1, block.Height)
	}

	// 2. 验证前一个区块哈希
	if latestBlock != nil && block.PrevHash != latestBlock.Hash {
		return fmt.Errorf("前一个区块哈希不匹配")
	}

	// 3. 验证时间戳
	if block.Timestamp.After(time.Now().Add(time.Minute)) {
		return fmt.Errorf("区块时间戳异常：在未来")
	}

	// 4. 验证生产者
	if block.ProducerID == "" {
		return fmt.Errorf("区块缺少生产者ID")
	}

	return nil
}

// ================================
// 自动同步检测
// ================================

// CheckAndSync 检查是否需要同步并自动执行
func (sm *SyncManager) CheckAndSync(nodeID string, peerNodeIDs []string, localHeight int, transport *p2p.HTTPTransport, peerManager *p2p.PeerManager) (bool, error) {
	// 估算网络最新高度（简化实现，实际应查询多个节点）
	networkLatestHeight := localHeight + 5 // 假设网络可能领先5个区块

	const syncThreshold = 10
	if networkLatestHeight-localHeight > syncThreshold {
		sm.Logger.Info("节点 %s 落后 %d 个区块，触发同步",
			nodeID, networkLatestHeight-localHeight)

		if len(peerNodeIDs) == 0 {
			return false, fmt.Errorf("没有可用的对等节点")
		}

		// 选择第一个对等节点
		targetPeer := peerNodeIDs[0]

		// 发起同步请求
		_, err := sm.RequestSync(nodeID, targetPeer, localHeight+1, 0, transport, peerManager)
		if err != nil {
			return false, err
		}

		return true, nil
	}

	return false, nil
}

// ================================
// 同步状态查询
// ================================

// GetSyncStatus 获取同步状态
func (sm *SyncManager) GetSyncStatus() map[string]interface{} {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	localLatest := sm.BlockPool.GetLatestBlock()
	localHeight := 0
	if localLatest != nil {
		localHeight = localLatest.Height
	}

	return map[string]interface{}{
		"local_height":     localHeight,
		"pending_requests": len(sm.PendingRequests),
		"is_syncing":       len(sm.PendingRequests) > 0,
	}
}

// CleanupTimedOutRequests 清理超时的同步请求
func (sm *SyncManager) CleanupTimedOutRequests() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	now := time.Now()
	for reqID, req := range sm.PendingRequests {
		if now.Sub(req.Timestamp) > sm.SyncTimeout {
			sm.Logger.Warn("同步请求 %s 超时，清理", reqID)
			delete(sm.PendingRequests, reqID)
		}
	}
}

// StartPeriodicCleanup 启动定期清理
func (sm *SyncManager) StartPeriodicCleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	go func() {
		for range ticker.C {
			sm.CleanupTimedOutRequests()
		}
	}()
}
