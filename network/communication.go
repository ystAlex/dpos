package network

import (
	"fmt"
	"sync"
	"time"
)

// communication.go
// 交易池、区块池

// ================================
// 交易池
// ================================

// Transaction 交易
type Transaction struct {
	ID        string
	From      string
	To        string
	Amount    float64
	Fee       float64
	Timestamp time.Time
	Signature []byte
	Nonce     int64
}

// TransactionPool 交易池
type TransactionPool struct {
	mu           sync.RWMutex
	transactions map[string]*Transaction
	pending      []*Transaction
	confirmed    map[string]bool
}

// NewTransactionPool 创建交易池
func NewTransactionPool() *TransactionPool {
	return &TransactionPool{
		transactions: make(map[string]*Transaction),
		pending:      make([]*Transaction, 0),
		confirmed:    make(map[string]bool),
	}
}

// AddTransaction 添加交易
func (tp *TransactionPool) AddTransaction(tx *Transaction) error {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	if _, exists := tp.transactions[tx.ID]; exists {
		return fmt.Errorf("交易 %s 已存在", tx.ID)
	}

	if tp.confirmed[tx.ID] {
		return fmt.Errorf("交易 %s 已确认", tx.ID)
	}

	tp.transactions[tx.ID] = tx
	tp.pending = append(tp.pending, tx)

	return nil
}

// GetPendingTransactions 获取待打包交易
func (tp *TransactionPool) GetPendingTransactions(limit int) []*Transaction {
	tp.mu.RLock()
	defer tp.mu.RUnlock()

	if limit > len(tp.pending) {
		limit = len(tp.pending)
	}

	txs := make([]*Transaction, limit)
	copy(txs, tp.pending[:limit])

	return txs
}

// RemoveTransactions 移除已打包的交易
func (tp *TransactionPool) RemoveTransactions(txIDs []string) {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	idSet := make(map[string]bool)
	for _, id := range txIDs {
		idSet[id] = true
		delete(tp.transactions, id)
	}

	// 从pending中移除
	newPending := make([]*Transaction, 0)
	for _, tx := range tp.pending {
		if !idSet[tx.ID] {
			newPending = append(newPending, tx)
		}
	}
	tp.pending = newPending
}

// GetPendingCount 获取待处理交易数量
func (tp *TransactionPool) GetPendingCount() int {
	tp.mu.RLock()
	defer tp.mu.RUnlock()
	return len(tp.pending)
}

// GetTransaction 获取交易
func (tp *TransactionPool) GetTransaction(txID string) (*Transaction, bool) {
	tp.mu.RLock()
	defer tp.mu.RUnlock()

	tx, exists := tp.transactions[txID]
	return tx, exists
}

// IsConfirmed 检查交易是否已确认
func (tp *TransactionPool) IsConfirmed(txID string) bool {
	tp.mu.RLock()
	defer tp.mu.RUnlock()
	return tp.confirmed[txID]
}

// ================================
// 区块池
// ================================

// Block 区块
type Block struct {
	Height       int
	ProducerID   string
	Transactions []*Transaction
	Timestamp    time.Time
	PrevHash     string
	Hash         string
	Validators   []string
	Signature    []byte
	StateRoot    string
}

// BlockPool 区块池
type BlockPool struct {
	mu           sync.RWMutex
	blocks       map[int]*Block // height -> block
	latest       *Block
	newestNumber int               // 最新的区块号
	index        map[string]*Block // hash -> block
}

// NewBlockPool 创建区块池
func NewBlockPool() *BlockPool {
	return &BlockPool{
		blocks: make(map[int]*Block),
		index:  make(map[string]*Block),
	}
}

// AddBlock 添加区块
func (bp *BlockPool) AddBlock(block *Block) error {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	// 检查区块高度是否已存在
	if _, exists := bp.blocks[block.Height]; exists {
		return fmt.Errorf("区块高度 %d 已存在", block.Height)
	}

	// 验证区块高度连续性
	if bp.latest != nil && block.Height != bp.latest.Height+1 {
		return fmt.Errorf("区块高度不连续: 期望 %d, 实际 %d", bp.latest.Height+1, block.Height)
	}

	// 验证前一个区块哈希
	if bp.latest != nil && block.PrevHash != bp.latest.Hash {
		return fmt.Errorf("前一个区块哈希不匹配")
	}

	bp.blocks[block.Height] = block
	bp.index[block.Hash] = block

	if bp.latest == nil || block.Height > bp.latest.Height {
		bp.latest = block
	}

	if block.Height > bp.newestNumber {
		//更新最新的区块信息
		bp.newestNumber = block.Height
	}

	return nil
}

// GetBlock 获取区块
func (bp *BlockPool) GetBlock(height int) (*Block, error) {
	bp.mu.RLock()
	defer bp.mu.RUnlock()

	if block, exists := bp.blocks[height]; exists {
		return block, nil
	}

	return nil, fmt.Errorf("区块高度 %d 不存在", height)
}

// GetLatestBlock 获取最新区块
func (bp *BlockPool) GetLatestBlock() *Block {
	bp.mu.RLock()
	defer bp.mu.RUnlock()
	return bp.latest
}

// GetNewestBlockNumber 获取最新全网区块号
func (bp *BlockPool) GetNewestBlockNumber() int {
	return bp.newestNumber
}

// SetNewestBlockNumber 更新最新全网区块号
func (bp *BlockPool) SetNewestBlockNumber(blockNumber int) {
	bp.newestNumber = blockNumber
}

// GetBlockByHash 通过哈希获取区块
func (bp *BlockPool) GetBlockByHash(hash string) (*Block, error) {
	bp.mu.RLock()
	defer bp.mu.RUnlock()

	if block, exists := bp.index[hash]; exists {
		return block, nil
	}

	return nil, fmt.Errorf("区块哈希 %s 不存在", hash)
}

// GetLatestHeight 获取最新高度
func (bp *BlockPool) GetLatestHeight() int {
	bp.mu.RLock()
	defer bp.mu.RUnlock()
	if bp.latest == nil {
		return 0
	}
	return bp.latest.Height
}

// GetBlockRange 获取区块范围
func (bp *BlockPool) GetBlockRange(startHeight, endHeight int) []*Block {
	bp.mu.RLock()
	defer bp.mu.RUnlock()

	blocks := make([]*Block, 0)
	for height := startHeight; height <= endHeight; height++ {
		if block, exists := bp.blocks[height]; exists {
			blocks = append(blocks, block)
		}
	}

	return blocks
}

// HasBlock 检查区块是否存在
func (bp *BlockPool) HasBlock(height int) bool {
	bp.mu.RLock()
	defer bp.mu.RUnlock()
	_, exists := bp.blocks[height]
	return exists
}

// GetBlockCount 获取区块总数
func (bp *BlockPool) GetBlockCount() int {
	bp.mu.RLock()
	defer bp.mu.RUnlock()
	return len(bp.blocks)
}
