#!/bin/bash

# 停止网络脚本

echo "停止所有 NM-DPoS 节点..."
pkill -f "bin/nm-dpos" || true
pkill -f "cmd/seed/main.go" || true

echo "清理完成"


pkill -9 nm-dpos