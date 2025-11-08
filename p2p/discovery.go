package p2p

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// discovery.go
// 节点发现机制

type DiscoveryService struct {
	mu           sync.RWMutex
	seedNodes    []string          // 种子节点地址列表
	knownNodes   map[string]string // nodeID -> address
	peerManager  *PeerManager
	localAddress string
	localNodeID  string
}

func NewDiscoveryService(seedNodes []string, localNodeID, localAddress string, peerManager *PeerManager) *DiscoveryService {
	return &DiscoveryService{
		seedNodes:    seedNodes,
		peerManager:  peerManager,
		localAddress: localAddress,
		localNodeID:  localNodeID,
	}
}

// Start 启动节点发现
func (ds *DiscoveryService) Start() {
	// 连接种子节点
	go ds.connectToSeeds()

	// 定期刷新节点列表
	go ds.periodicRefresh()

	// 定期广播自己的存在
	go ds.periodicAnnounce()
}

// connectToSeeds 连接到种子节点
func (ds *DiscoveryService) connectToSeeds() {
	for _, seedAddr := range ds.seedNodes {
		go func(addr string) {
			if err := ds.requestPeerList(addr); err != nil {
				fmt.Printf("连接种子节点 %s 失败: %v\n", addr, err)
			}
		}(seedAddr)
	}
}

// requestPeerList 请求对等节点列表
func (ds *DiscoveryService) requestPeerList(targetAddr string) error {
	url := fmt.Sprintf("http://%s/peers", targetAddr)
	client := &http.Client{Timeout: 5 * time.Second}

	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// 解析响应
	var peers []*PeerInfo
	if err := json.NewDecoder(resp.Body).Decode(&peers); err != nil {
		return err
	}

	for _, peer := range peers {
		if peer.NodeID != ds.localNodeID {
			ds.peerManager.AddPeer(peer)
			ds.knownNodes[peer.NodeID] = peer.Address
		}
	}

	fmt.Printf("从 %s 发现 %d 个节点\n", targetAddr, len(peers))
	return nil
}

// periodicRefresh 定期刷新节点列表
func (ds *DiscoveryService) periodicRefresh() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		peers := ds.peerManager.GetActivePeers()
		for _, peer := range peers {
			go ds.requestPeerList(peer.Address)
		}

		// 清理不活跃节点
		pruned := ds.peerManager.PrunePeers(5 * time.Minute)
		if pruned > 0 {
			fmt.Printf("[Discovery] 清理了 %d 个不活跃节点\n", pruned)
		}
	}
}

// periodicAnnounce 定期广播自己的存在
func (ds *DiscoveryService) periodicAnnounce() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		peers := ds.peerManager.GetActivePeers()
		for _, peer := range peers {
			go ds.sendHeartbeat(peer.Address)
		}
	}
}

// sendHeartbeat 发送心跳
func (ds *DiscoveryService) sendHeartbeat(targetAddr string) error {
	url := fmt.Sprintf("http://%s/heartbeat", targetAddr)

	data := map[string]interface{}{
		"node_id": ds.localNodeID,
		"address": ds.localAddress,
	}

	jsonData, _ := json.Marshal(data)
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	_, err := client.Do(req)

	return err
}

// GetKnownPeers 获取已知节点列表
func (ds *DiscoveryService) GetKnownPeers() map[string]string {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	result := make(map[string]string)
	for k, v := range ds.knownNodes {
		result[k] = v
	}
	return result
}
