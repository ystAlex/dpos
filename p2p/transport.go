package p2p

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// transport.go
// 完整的网络传输层

type HTTPTransport struct {
	client  *http.Client
	timeout time.Duration
}

func NewHTTPTransport(timeout time.Duration) *HTTPTransport {
	return &HTTPTransport{
		client: &http.Client{
			Timeout: timeout,
		},
		timeout: timeout,
	}
}

// SendJSON 发送JSON数据
func (t *HTTPTransport) SendJSON(targetAddr, path string, data interface{}) (*http.Response, error) {
	url := fmt.Sprintf("http://%s%s", targetAddr, path)

	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("序列化失败: %v", err)
	}

	resp, err := t.client.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("发送失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("请求失败，状态码: %d", resp.StatusCode)
	}

	return resp, nil
}

// GetJSON 获取JSON数据
func (t *HTTPTransport) GetJSON(targetAddr, path string, result interface{}) error {
	url := fmt.Sprintf("http://%s%s", targetAddr, path)

	resp, err := t.client.Get(url)
	if err != nil {
		return fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("请求失败，状态码: %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return fmt.Errorf("解析失败: %v", err)
	}

	return nil
}

// BroadcastJSON 广播JSON数据到多个节点
func (t *HTTPTransport) BroadcastJSON(peers []*PeerInfo, path string, data interface{}) error {
	successCount := 0
	errorCount := 0

	for _, peer := range peers {
		fmt.Printf("[Transport] 广播新生成区块到节点 %s \n", peer.NodeID)
		if _, err := t.SendJSON(peer.Address, path, data); err != nil {
			errorCount++
		} else {
			successCount++
		}
	}

	if errorCount > 0 {
		return fmt.Errorf("广播完成，成功: %d, 失败: %d", successCount, errorCount)
	}

	return nil
}
