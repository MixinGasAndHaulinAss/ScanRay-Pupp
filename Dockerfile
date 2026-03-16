# =============================================================================
# ScanRay Pupp - Multi-stage Docker Build
# Standalone image: builds the Go agent, downloads Scanray + Nuclei binaries
# =============================================================================

# Stage 1: Build the Pupp Go agent from source
FROM golang:1.22-alpine AS pupp-builder
RUN apk add --no-cache gcc musl-dev
WORKDIR /build
COPY go.mod ./
COPY go.sum* ./
RUN go mod download 2>/dev/null || true
COPY . .
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o /pupp ./cmd/pupp

# Stage 2: Get Nuclei binary from official image
FROM projectdiscovery/nuclei:latest AS nuclei-bin

# Stage 3: Final image
FROM ubuntu:24.04

LABEL maintainer="NCLGISA"
LABEL description="ScanRay Pupp - Remote Scanning Agent for ScanRay Console"
LABEL org.opencontainers.image.source="https://github.com/NCLGISA/ScanRay-Pupp"

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
    libpcap0.8 \
    jq \
    ca-certificates \
    curl \
    iproute2 \
    unzip \
    && rm -rf /var/lib/apt/lists/*

RUN mkdir -p /opt/scanray/bin /opt/scanray/data

# Copy Pupp agent binary
COPY --from=pupp-builder /pupp /opt/scanray/bin/pupp

# Copy Nuclei binary from official image
COPY --from=nuclei-bin /usr/local/bin/nuclei /opt/scanray/bin/nuclei

# Download latest Scanray binary from GitHub releases
ARG SCANRAY_VERSION=latest
RUN ARCH=$(dpkg --print-architecture) && \
    if [ "$SCANRAY_VERSION" = "latest" ]; then \
        DOWNLOAD_URL=$(curl -sSL https://api.github.com/repos/NCLGISA/The-ScanRay-Console/releases/latest \
            | grep "browser_download_url.*scanray-linux-${ARCH}" \
            | head -1 | cut -d '"' -f 4); \
    else \
        DOWNLOAD_URL="https://github.com/NCLGISA/The-ScanRay-Console/releases/download/${SCANRAY_VERSION}/scanray-linux-${ARCH}"; \
    fi && \
    if [ -n "$DOWNLOAD_URL" ]; then \
        curl -sSL -o /opt/scanray/bin/scanray "$DOWNLOAD_URL"; \
    else \
        echo "WARNING: Could not determine Scanray download URL. Place binary manually at /opt/scanray/bin/scanray"; \
        touch /opt/scanray/bin/scanray; \
    fi

RUN chmod +x /opt/scanray/bin/*

RUN groupadd -g 1500 scanray && \
    useradd -m -u 1001 -G scanray pupp && \
    chown -R pupp:scanray /opt/scanray && \
    chmod -R 775 /opt/scanray

USER pupp

# Pre-download Nuclei templates
RUN /opt/scanray/bin/nuclei -update-templates 2>/dev/null || true

ENV SCANRAY_BINARY=/opt/scanray/bin/scanray
ENV NUCLEI_BINARY=/opt/scanray/bin/nuclei
ENV PUPP_DATA_DIR=/opt/scanray/data

ENTRYPOINT ["/opt/scanray/bin/pupp"]
