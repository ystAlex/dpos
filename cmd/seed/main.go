package main

import (
	"flag"
	"fmt"
	"nm-dpos/config"
	"nm-dpos/node"
	"nm-dpos/test"
	"nm-dpos/utils"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

var (
	// 节点配置
	nodeID        = flag.String("id", "", "节点ID")
	listenAddr    = flag.String("listen", "localhost:8000", "监听地址")
	seedNodes     = flag.String("seeds", "", "种子节点地址列表，逗号分隔")
	initialTime   = flag.String("time", "2006-01-02T15:04:05+07:00", "整个系统初始化的时间")
	initialWeight = flag.Float64("weight", 100.0, "初始权重")
	performance   = flag.Float64("perf", 0.7, "初始表现评分")
	networkDelay  = flag.Float64("delay", 20.0, "网络延迟(ms)")

	// 测试配置
	runTest        = flag.Bool("test", false, "是否运行测试")
	testRounds     = flag.Int("rounds", 20, "测试轮数")
	testOutputDir  = flag.String("output", "./test_results", "测试结果输出目录")
	testMode       = flag.String("mode", "full", "测试模式: full|throughput|latency|decentralization|voting|malicious")
	maliciousCount = flag.Int("malicious", 0, "恶意节点数量(仅用于malicious模式)")
)

func main() {
	flag.Parse()

	// 验证必需参数
	if *nodeID == "" {
		fmt.Println("错误: 必须指定节点ID (-id)")
		flag.Usage()
		os.Exit(1)
	}

	// 创建日志器
	logger := utils.NewLogger(*nodeID)
	logger.Info("========================================")
	logger.Info("启动 NM-DPoS 节点")
	logger.Info("节点ID: %s", *nodeID)
	logger.Info("监听地址: %s", *listenAddr)
	logger.Info("========================================")

	// 解析种子节点
	var seeds []string
	if *seedNodes != "" {
		seeds = strings.Split(*seedNodes, ",")
		logger.Info("种子节点: %v", seeds)
	}

	times, err := time.Parse(time.RFC3339, *initialTime)
	if err != nil {
		logger.Warn(err.Error())
		return
	}

	// 创建P2P节点
	p2pNode := node.NewP2PNode(
		*nodeID,
		*initialWeight,
		*performance,
		*networkDelay,
		*listenAddr,
		seeds,
		times,
		logger,
	)

	// 启动节点
	if err := p2pNode.Start(); err != nil {
		logger.Error("启动节点失败: %v", err)
		os.Exit(1)
	}

	logger.Info("节点启动成功")

	// 如果启用测试模式
	if *runTest {
		logger.Info("========================================")
		logger.Info("启动测试模式")
		logger.Info("测试持续时间: %d 轮", *testRounds)
		logger.Info("结果输出目录: %s", *testOutputDir)
		logger.Info("========================================")

		// 等待网络稳定
		time.Sleep(20 * time.Second)

		// 等待网络稳定
		logger.Info("等待网络稳定...")
		time.Sleep(20 * time.Second)

		// 创建测试器
		tester := test.NewNetworkTester(p2pNode, logger, *testOutputDir)

		// 根据测试模式运行不同的测试
		switch *testMode {
		case "full":
			// 完整测试
			tester.RunFullTest(*testRounds)

		case "throughput":
			// 吞吐量测试
			duration := time.Duration(*testRounds*config.BlockInterval) * time.Second
			tester.RunThroughputTest(duration, p2pNode.PeerManager.GetPeerCount()+1)

		case "latency":
			// 时延测试
			tester.RunLatencyTest(*testRounds)

		case "decentralization":
			// 去中心化测试
			tester.RunDecentralizationTest(*testRounds)

		case "voting":
			// 投票速率测试
			tester.RunVotingSpeedTest(*testRounds)

		case "malicious":
			// 恶意节点测试
			if *maliciousCount == 0 {
				logger.Warn("恶意节点测试需要指定 -malicious 参数")
			} else {
				tester.RunMaliciousNodeTest(*testRounds, *maliciousCount)
			}

		default:
			logger.Error("未知的测试模式: %s", *testMode)
			logger.Info("支持的模式: full, throughput, latency, decentralization, voting, malicious")
		}
	}

	// 等待中断信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	logger.Info("节点正在运行，按 Ctrl+C 停止...")
	<-sigChan

	logger.Info("收到停止信号，正在关闭节点...")
	p2pNode.Stop()
	logger.Info("节点已停止")
}
