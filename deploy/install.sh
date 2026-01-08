#!/bin/bash
# CPA Logger 安装脚本

set -e

INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/cpa-logger"
SERVICE_FILE="/etc/systemd/system/cpa-logger.service"

echo "Installing CPA Logger..."

# 创建配置目录
sudo mkdir -p "$CONFIG_DIR"

# 复制二进制文件
if [ -f "./cpa-logger" ]; then
    sudo cp ./cpa-logger "$INSTALL_DIR/cpa-logger"
    sudo chmod +x "$INSTALL_DIR/cpa-logger"
    echo "✓ Binary installed to $INSTALL_DIR/cpa-logger"
else
    echo "✗ Error: cpa-logger binary not found. Please build first."
    exit 1
fi

# 复制配置文件（如果不存在）
if [ ! -f "$CONFIG_DIR/config.yaml" ]; then
    sudo cp ./deploy/config.yaml "$CONFIG_DIR/config.yaml"
    echo "✓ Config installed to $CONFIG_DIR/config.yaml"
    echo "  Please edit the config file to match your environment"
else
    echo "✓ Config already exists at $CONFIG_DIR/config.yaml"
fi

# 安装 systemd 服务
sudo cp ./deploy/cpa-logger.service "$SERVICE_FILE"
sudo systemctl daemon-reload
echo "✓ Systemd service installed"

echo ""
echo "Installation complete!"
echo ""
echo "Next steps:"
echo "  1. Edit config: sudo nano $CONFIG_DIR/config.yaml"
echo "  2. Start service: sudo systemctl start cpa-logger"
echo "  3. Enable on boot: sudo systemctl enable cpa-logger"
echo "  4. Check status: sudo systemctl status cpa-logger"
echo "  5. View logs: sudo journalctl -u cpa-logger -f"
