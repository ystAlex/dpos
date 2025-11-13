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
	seedNodes    []string // 种子节点地址列表
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
	fmt.Printf("[Discovery] 启动节点发现服务...\n")
	fmt.Printf("[Discovery] 本地地址: %s\n", ds.localAddress)
	fmt.Printf("[Discovery] 种子节点: %v\n", ds.seedNodes)

	// 连接种子节点
	go ds.connectToSeeds()

	// 定期刷新节点列表
	go ds.periodicRefresh()

	// 定期广播自己的存在
	go ds.periodicAnnounce()

	fmt.Printf("[Discovery] 节点发现服务已启动\n")
}

// connectToSeeds 连接到种子节点
func (ds *DiscoveryService) connectToSeeds() {
	if len(ds.seedNodes) == 0 || (len(ds.seedNodes) == 1 && ds.seedNodes[0] == "") {
		fmt.Printf("[Discovery] 没有配置种子节点，作为独立节点运行\n")
		return
	}

	fmt.Printf("[Discovery] 开始连接 %d 个种子节点...\n", len(ds.seedNodes))

	maxRetries := 20
	retryDelay := 2 * time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		fmt.Printf("[Discovery] === 连接尝试 %d/%d ===\n", attempt, maxRetries)

		successCount := 0

		for _, seedAddr := range ds.seedNodes {
			if seedAddr == "" || seedAddr == ds.localAddress {
				continue
			}

			fmt.Printf("[Discovery] 连接种子节点: %s\n", seedAddr)

			// ===== 关键：先发送心跳，注册自己 =====
			if err := ds.sendHeartbeatAndGetPeers(seedAddr); err != nil {
				fmt.Printf("[Discovery]  心跳失败: %v\n", err)
			} else {
				fmt.Printf("[Discovery]  心跳成功，已注册到种子节点\n")
				successCount++
			}

			// ===== 然后请求对等列表 =====
			if err := ds.requestPeerList(seedAddr); err != nil {
				fmt.Printf("[Discovery]  获取对等列表失败: %v\n", err)
			} else {
				fmt.Printf("[Discovery]  成功获取对等列表\n")
			}
		}

		if successCount > 0 {
			fmt.Printf("[Discovery]  已连接 %d/%d 个种子节点\n",
				successCount, len(ds.seedNodes))
			return
		}

		if attempt < maxRetries {
			fmt.Printf("[Discovery] 等待 %v 后重试...\n", retryDelay)
			time.Sleep(retryDelay)
		}
	}

	fmt.Printf("[Discovery]  无法连接到任何种子节点\n")
}

// sendHeartbeatAndGetPeers 发送心跳并获取对等列表（核心方法）
func (ds *DiscoveryService) sendHeartbeatAndGetPeers(targetAddr string) error {
	url := fmt.Sprintf("http://%s/heartbeat", targetAddr)

	// 构造心跳数据
	data := map[string]interface{}{
		"node_id": ds.localNodeID,
		"address": ds.localAddress,
	}

	jsonData, _ := json.Marshal(data)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP错误: %d", resp.StatusCode)
	}

	// ===== 关键：心跳响应中包含对等列表 =====
	var peers []*PeerInfo
	if err := json.NewDecoder(resp.Body).Decode(&peers); err != nil {
		return fmt.Errorf("解析对等列表失败: %v", err)
	}

	// 添加对等节点
	addedCount := 0
	for _, peer := range peers {
		if peer.NodeID != ds.localNodeID {
			ds.peerManager.AddPeer(peer)
			addedCount++
		}
	}

	fmt.Printf("[Discovery] 从心跳响应中发现 %d 个对等节点\n", addedCount)
	return nil
}

// requestPeerList 请求对等节点列表
func (ds *DiscoveryService) requestPeerList(targetAddr string) error {
	url := fmt.Sprintf("http://%s/peers", targetAddr)
	client := &http.Client{Timeout: 5 * time.Second}

	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("HTTP请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP错误: %d", resp.StatusCode)
	}

	var peers []*PeerInfo
	if err := json.NewDecoder(resp.Body).Decode(&peers); err != nil {
		return fmt.Errorf("解析响应失败: %v", err)
	}

	addedCount := 0
	for _, peer := range peers {
		if peer.NodeID != ds.localNodeID {
			ds.peerManager.AddPeer(peer)

			addedCount++
		}
	}

	//fmt.Printf("[Discovery] 从 /peers 端点发现 %d 个对等节点\n", addedCount)
	return nil
}

// periodicRefresh 定期刷新节点列表
func (ds *DiscoveryService) periodicRefresh() {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		// fmt.Printf("[Discovery] === 执行定期刷新 ===\n")

		// 1. 从种子节点刷新
		for _, seedAddr := range ds.seedNodes {
			if seedAddr == "" || seedAddr == ds.localAddress {
				continue
			}
			go ds.requestPeerList(seedAddr)
		}

		// 2. 从已知对等节点刷新
		peers := ds.peerManager.GetActivePeers()
		for _, peer := range peers {
			go ds.requestPeerList(peer.Address)
		}

		// 3. 清理不活跃节点
		pruned := ds.peerManager.PrunePeers(5 * time.Minute)
		if pruned > 0 {
			fmt.Printf("[Discovery] 清理了 %d 个不活跃节点\n", pruned)
		}

		peerCount := ds.peerManager.GetPeerCount()
		fmt.Printf("[Discovery] 当前对等节点数: %d\n", peerCount)
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
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}
