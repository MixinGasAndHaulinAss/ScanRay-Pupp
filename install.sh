#!/bin/bash
set -e

# ScanRay Pupp - Bare-metal Install Script
# Usage: curl -sSL https://raw.githubusercontent.com/NCLGISA/ScanRay-Pupp/main/install.sh | bash -s -- --id <PUPP_ID> --token <TOKEN> --url <WSS_URL>

INSTALL_DIR="/opt/scanray"
PUPP_ID=""
PUPP_TOKEN=""
CONSOLE_URL=""
PUPP_DOWNLOAD_BASE="https://github.com/NCLGISA/ScanRay-Pupp/releases/latest/download"
SCANRAY_DOWNLOAD_BASE="https://github.com/NCLGISA/The-ScanRay-Console/releases/latest/download"

while [[ $# -gt 0 ]]; do
    case $1 in
        --id) PUPP_ID="$2"; shift 2 ;;
        --token) PUPP_TOKEN="$2"; shift 2 ;;
        --url) CONSOLE_URL="$2"; shift 2 ;;
        --pupp-download-base) PUPP_DOWNLOAD_BASE="$2"; shift 2 ;;
        --scanray-download-base) SCANRAY_DOWNLOAD_BASE="$2"; shift 2 ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

if [ -z "$PUPP_ID" ] || [ -z "$PUPP_TOKEN" ] || [ -z "$CONSOLE_URL" ]; then
    echo "ERROR: --id, --token, and --url are required"
    echo "Usage: $0 --id <PUPP_ID> --token <TOKEN> --url <WSS_URL>"
    exit 1
fi

echo "=== ScanRay Pupp Installer ==="
echo "Pupp ID:  $PUPP_ID"
echo "Console:  $CONSOLE_URL"

ARCH=$(uname -m)
case $ARCH in
    x86_64)  ARCH="amd64" ;;
    aarch64) ARCH="arm64" ;;
    *)       echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
echo "Platform: ${OS}/${ARCH}"

echo "Creating directories..."
sudo mkdir -p "${INSTALL_DIR}/bin" "${INSTALL_DIR}/data"

echo "Downloading ScanRay Pupp agent..."
sudo curl -sSL -o "${INSTALL_DIR}/bin/pupp" \
    "${PUPP_DOWNLOAD_BASE}/pupp-${OS}-${ARCH}" || {
    echo "WARNING: Download failed. Place the pupp binary at ${INSTALL_DIR}/bin/pupp manually."
}

echo "Downloading Scanray scanner..."
sudo curl -sSL -o "${INSTALL_DIR}/bin/scanray" \
    "${SCANRAY_DOWNLOAD_BASE}/scanray-${OS}-${ARCH}" || {
    echo "WARNING: Download failed. Place the scanray binary at ${INSTALL_DIR}/bin/scanray manually."
}

echo "Downloading Nuclei scanner..."
NUCLEI_VER=$(curl -sSL "https://api.github.com/repos/projectdiscovery/nuclei/releases/latest" | grep '"tag_name"' | head -1 | cut -d '"' -f 4)
NUCLEI_VER=${NUCLEI_VER:-v3.3.7}
sudo curl -sSL "https://github.com/projectdiscovery/nuclei/releases/download/${NUCLEI_VER}/nuclei_${NUCLEI_VER#v}_${OS}_${ARCH}.zip" -o /tmp/nuclei.zip
sudo unzip -o /tmp/nuclei.zip -d "${INSTALL_DIR}/bin/" nuclei 2>/dev/null || {
    echo "WARNING: Nuclei download/extract failed. Install manually."
}
rm -f /tmp/nuclei.zip

sudo chmod +x "${INSTALL_DIR}/bin/"*

sudo groupadd -f -g 1500 scanray 2>/dev/null || true
sudo useradd -r -s /bin/false -G scanray scanray 2>/dev/null || true
sudo chown -R scanray:scanray "${INSTALL_DIR}"
sudo chmod -R 775 "${INSTALL_DIR}"

echo "Updating Nuclei templates..."
sudo -u scanray "${INSTALL_DIR}/bin/nuclei" -update-templates 2>/dev/null || echo "Template update skipped"

echo "Creating environment file..."
sudo tee /etc/scanray-pupp.env > /dev/null <<ENVEOF
PUPP_ID=${PUPP_ID}
PUPP_AUTH_TOKEN=${PUPP_TOKEN}
PUPP_CONSOLE_URL=${CONSOLE_URL}
SCANRAY_BINARY=${INSTALL_DIR}/bin/scanray
NUCLEI_BINARY=${INSTALL_DIR}/bin/nuclei
PUPP_DATA_DIR=${INSTALL_DIR}/data
ENVEOF
sudo chmod 600 /etc/scanray-pupp.env

echo "Creating systemd service..."
sudo tee /etc/systemd/system/scanray-pupp.service > /dev/null <<SVCEOF
[Unit]
Description=ScanRay Pupp - Remote Scanning Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=scanray
Group=scanray
EnvironmentFile=/etc/scanray-pupp.env
ExecStart=${INSTALL_DIR}/bin/pupp
Restart=always
RestartSec=10
AmbientCapabilities=CAP_NET_RAW

[Install]
WantedBy=multi-user.target
SVCEOF

echo "Enabling and starting service..."
sudo systemctl daemon-reload
sudo systemctl enable scanray-pupp
sudo systemctl start scanray-pupp

echo ""
echo "=== ScanRay Pupp Installed ==="
echo "Status:  sudo systemctl status scanray-pupp"
echo "Logs:    sudo journalctl -u scanray-pupp -f"
echo "Config:  /etc/scanray-pupp.env"
