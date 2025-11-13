package core

import (
	"nm-dpos/types"
	"sync"
)

// BlockScheduler 出块调度器
// 负责管理代理节点的出块时隙分配
type BlockScheduler struct {
	mu sync.RWMutex

	// 当前代理节点列表（按权重排序）
	Delegates []*types.Node

	// 当前时隙索引
	CurrentSlot int

	// 每轮的区块数（= 代理节点数）
	BlocksPerRound int

	// 当前区块高度
	CurrentHeight int
}

// NewBlockScheduler 创建调度器
func NewBlockScheduler() *BlockScheduler {
	return &BlockScheduler{
		Delegates:      make([]*types.Node, 0),
		CurrentSlot:    0,
		BlocksPerRound: 0,
		CurrentHeight:  0,
	}
}

// UpdateDelegates 更新代理节点列表
// 在每轮选举后调用
func (bs *BlockScheduler) UpdateDelegates(delegates []*types.Node) {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	bs.Delegates = delegates
	//走完一次代理算是一轮
	bs.BlocksPerRound = len(delegates)
	bs.CurrentSlot = 0 // 重置时隙
}

// GetCurrentProducer 从代理节点中可以获取当前时隙的出块者
// 返回：(出块节点, 是否是本轮的出块者)
func (bs *BlockScheduler) GetCurrentProducer() (*types.Node, bool) {
	bs.mu.RLock()
	defer bs.mu.RUnlock()

	if len(bs.Delegates) == 0 {
		return nil, false
	}

	// 循环分配时隙
	slotIndex := bs.CurrentSlot % len(bs.Delegates)
	producer := bs.Delegates[slotIndex]

	return producer, true
}

// NextSlot 前进到下一个时隙
func (bs *BlockScheduler) NextSlot() {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	bs.CurrentSlot++
}

// ShouldProduce 判断指定节点是否应该在当前时隙出块
func (bs *BlockScheduler) ShouldProduce(nodeID string) bool {
	producer, ok := bs.GetCurrentProducer()
	if !ok {
		return false
	}

	return producer.ID == nodeID
}

// GetSlotInfo 获取当前时隙信息（用于日志）
func (bs *BlockScheduler) GetSlotInfo() map[string]interface{} {
	bs.mu.RLock()
	defer bs.mu.RUnlock()

	producer, _ := bs.GetCurrentProducer()
	producerID := ""
	if producer != nil {
		producerID = producer.ID
	}

	roundProgress := 0.0
	if bs.BlocksPerRound > 0 {
		roundProgress = float64(bs.CurrentSlot) / float64(bs.BlocksPerRound)
		//fmt.Println("1111111111111111", roundProgress)
	}
	//else {
	//	//如果没有选举出任何的代理者则认为进度是1
	//	roundProgress = 1.0
	//	fmt.Println("2222222222222222", roundProgress)
	//}

	return map[string]interface{}{
		"current_slot":     bs.CurrentSlot,
		"current_height":   bs.CurrentHeight,
		"blocks_per_round": bs.BlocksPerRound,
		"current_producer": producerID,
		"round_progress":   roundProgress,
	}
}

// IsReady 检查调度器是否已初始化
func (bs *BlockScheduler) IsReady() bool {
	bs.mu.RLock()
	defer bs.mu.RUnlock()
	return len(bs.Delegates) > 0
}
