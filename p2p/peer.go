package p2p

import (
	"sync"
	"time"
)

// peer.go
// 对等节点管理

// PeerInfo 对等节点信息
type PeerInfo struct {
	NodeID      string    // 节点唯一标识
	Address     string    // 网络地址（IP:Port）
	PublicKey   string    // 公钥（用于验证）
	LastSeen    time.Time // 最后在线时间
	IsActive    bool      // 是否活跃
	Latency     float64   // 网络延迟（ms）
	IsDelegate  bool      // 是否为代理节点
	Weight      float64   // 节点权重
	Performance float64   // 表现评分
}

// PeerManager 对等节点管理器
type PeerManager struct {
	mu          sync.RWMutex
	peers       map[string]*PeerInfo // nodeID -> PeerInfo
	localNodeID string
	maxPeers    int
}

// NewPeerManager 创建对等节点管理器
func NewPeerManager(localNodeID string, maxPeers int) *PeerManager {
	return &PeerManager{
		peers:       make(map[string]*PeerInfo),
		localNodeID: localNodeID,
		maxPeers:    maxPeers,
	}
}

// AddPeer 添加对等节点
func (pm *PeerManager) AddPeer(peer *PeerInfo) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if peer.NodeID == pm.localNodeID {
		return nil // 不添加自己
	}

	pm.peers[peer.NodeID] = peer
	return nil
}

// RemovePeer 移除对等节点
func (pm *PeerManager) RemovePeer(nodeID string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	delete(pm.peers, nodeID)
}

// GetPeer 获取对等节点信息
func (pm *PeerManager) GetPeer(nodeID string) (*PeerInfo, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	peer, exists := pm.peers[nodeID]
	return peer, exists
}

// GetAllPeers 获取所有对等节点
func (pm *PeerManager) GetAllPeers() []*PeerInfo {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	peers := make([]*PeerInfo, 0, len(pm.peers))
	for _, peer := range pm.peers {
		peers = append(peers, peer)
	}
	return peers
}

// GetActivePeers 获取活跃对等节点
func (pm *PeerManager) GetActivePeers() []*PeerInfo {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	peers := make([]*PeerInfo, 0)
	for _, peer := range pm.peers {
		if peer.IsActive {
			peers = append(peers, peer)
		}
	}
	return peers
}

// UpdatePeerStatus 更新对等节点状态
func (pm *PeerManager) UpdatePeerStatus(nodeID string, isActive bool, weight, performance float64) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if peer, exists := pm.peers[nodeID]; exists {
		peer.IsActive = isActive
		peer.Weight = weight
		peer.Performance = performance
		peer.LastSeen = time.Now()
	}
}

// GetDelegates 获取代理节点列表
func (pm *PeerManager) GetDelegates() []*PeerInfo {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	delegates := make([]*PeerInfo, 0)
	for _, peer := range pm.peers {
		if peer.IsDelegate && peer.IsActive {
			delegates = append(delegates, peer)
		}
	}
	return delegates
}

// PrunePeers 清理不活跃的节点
func (pm *PeerManager) PrunePeers(timeout time.Duration) int {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	now := time.Now()
	pruned := 0

	for nodeID, peer := range pm.peers {
		if now.Sub(peer.LastSeen) > timeout {
			delete(pm.peers, nodeID)
			pruned++
		}
	}

	return pruned
}

// GetPeerCount 获取节点总数
func (pm *PeerManager) GetPeerCount() int {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return len(pm.peers)
}
