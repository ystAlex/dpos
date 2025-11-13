package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	"nm-dpos/network"
	"time"
)

var (
	targetNode = flag.String("node", "localhost:8000", "目标节点地址")
	txCount    = flag.Int("count", 10, "生成交易数量")
	interval   = flag.Int("interval", 1000, "交易间隔(毫秒)")
)

func main() {
	flag.Parse()

	fmt.Printf("开始向 %s 发送交易...\n", *targetNode)

	for i := 0; i < *txCount; i++ {
		tx := generateTransaction(i)

		if err := sendTransaction(*targetNode, tx); err != nil {
			fmt.Printf("❌ 发送交易 %d 失败: %v\n", i, err)
		} else {
			fmt.Printf("✅ 发送交易 %d 成功\n", i)
		}

		time.Sleep(time.Duration(*interval) * time.Millisecond)
	}

	fmt.Println("所有交易已发送")
}

// generateTransaction 生成随机交易
func generateTransaction(nonce int) *network.Transaction {
	from := fmt.Sprintf("user-%d", rand.Intn(100))
	to := fmt.Sprintf("user-%d", rand.Intn(100))
	amount := rand.Float64() * 100
	fee := amount * 0.01

	return &network.Transaction{
		ID:        fmt.Sprintf("tx-%d-%d", time.Now().Unix(), nonce),
		From:      from,
		To:        to,
		Amount:    amount,
		Fee:       fee,
		Timestamp: time.Now(),
		Nonce:     int64(nonce),
	}
}

// sendTransaction 发送交易到节点
func sendTransaction(nodeAddr string, tx *network.Transaction) error {
	url := fmt.Sprintf("http://%s/transaction", nodeAddr)

	jsonData, err := json.Marshal(tx)
	if err != nil {
		return err
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	return nil
}
