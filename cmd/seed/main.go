package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"nm-dpos/p2p"
	"sync"
	"time"
)

// 种子节点程序
// 用于帮助新节点发现网络中的其他节点

type SeedNode struct {
	mu              sync.RWMutex
	registeredNodes map[string]*p2p.PeerInfo
	listenAddress   string
}

func NewSeedNode(listenAddress string) *SeedNode {
	return &SeedNode{
		registeredNodes: make(map[string]*p2p.PeerInfo),
		listenAddress:   listenAddress,
	}
}

func (sn *SeedNode) Start() error {
	mux := http.NewServeMux()

	// 节点注册
	mux.HandleFunc("/register", sn.handleRegister)

	// 节点列表查询
	mux.HandleFunc("/peers", sn.handlePeers)

	// 心跳
	mux.HandleFunc("/heartbeat", sn.handleHeartbeat)

	// 健康检查
	mux.HandleFunc("/health", sn.handleHealth)

	log.Printf("种子节点启动，监听: %s", sn.listenAddress)
	log.Printf("已注册节点数: %d", len(sn.registeredNodes))

	return http.ListenAndServe(sn.listenAddress, mux)
}

func (sn *SeedNode) handleRegister(w http.ResponseWriter, r *http.Request) {
	var peer p2p.PeerInfo
	if err := json.NewDecoder(r.Body).Decode(&peer); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	peer.LastSeen = time.Now()
	peer.IsActive = true

	sn.mu.Lock()
	sn.registeredNodes[peer.NodeID] = &peer
	sn.mu.Unlock()

	log.Printf("注册节点: %s (%s) [总计: %d]", peer.NodeID, peer.Address, len(sn.registeredNodes))

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "registered"})
}

func (sn *SeedNode) handlePeers(w http.ResponseWriter, r *http.Request) {
	sn.mu.RLock()
	defer sn.mu.RUnlock()

	peers := make([]*p2p.PeerInfo, 0, len(sn.registeredNodes))
	for _, peer := range sn.registeredNodes {
		// 只返回活跃的节点（5分钟内有心跳）
		if time.Since(peer.LastSeen) < 5*time.Minute {
			peers = append(peers, peer)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(peers)

	log.Printf("节点列表请求: 返回 %d 个活跃节点", len(peers))
}

func (sn *SeedNode) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	var data map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	nodeID := data["node_id"].(string)

	sn.mu.Lock()
	if peer, exists := sn.registeredNodes[nodeID]; exists {
		peer.LastSeen = time.Now()
	}
	sn.mu.Unlock()

	w.WriteHeader(http.StatusOK)
}

func (sn *SeedNode) handleHealth(w http.ResponseWriter, r *http.Request) {
	sn.mu.RLock()
	defer sn.mu.RUnlock()

	activeCount := 0
	for _, peer := range sn.registeredNodes {
		if time.Since(peer.LastSeen) < 5*time.Minute {
			activeCount++
		}
	}

	health := map[string]interface{}{
		"status":         "healthy",
		"total_nodes":    len(sn.registeredNodes),
		"active_nodes":   activeCount,
		"uptime_seconds": time.Now().Unix(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(health)
}

func main() {
	port := flag.Int("port", 9000, "监听端口")
	flag.Parse()

	listenAddress := fmt.Sprintf(":%d", *port)
	seedNode := NewSeedNode(listenAddress)

	log.Fatal(seedNode.Start())
}
