# =============================================================================
# ScanRay Pupp - Multi-stage Docker Build
# Builds Pupp agent + Scanray scanner from source, pulls Nuclei from official image
# =============================================================================

# Stage 1: Build Scanray scanner from source
FROM golang:1.22-alpine AS scanray-builder
RUN apk add --no-cache gcc musl-dev libpcap-dev
WORKDIR /build
COPY scanray-src/ ./
RUN go mod download
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags "-s -w" -o /scanray .

# Stage 2: Build the Pupp Go agent from source
FROM golang:1.22-alpine AS pupp-builder
RUN apk add --no-cache gcc musl-dev
WORKDIR /build
COPY go.mod ./
COPY go.sum* ./
RUN go mod download 2>/dev/null || true
COPY . .
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags "-s -w" -o /pupp ./cmd/pupp

# Stage 3: Get Nuclei binary from official image
FROM projectdiscovery/nuclei:latest AS nuclei-bin

# Stage 4: Final image
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
    && rm -rf /var/lib/apt/lists/*

RUN mkdir -p /opt/scanray/bin /opt/scanray/data

COPY --from=pupp-builder /pupp /opt/scanray/bin/pupp
COPY --from=nuclei-bin /usr/local/bin/nuclei /opt/scanray/bin/nuclei
COPY --from=scanray-builder /scanray /opt/scanray/bin/scanray

RUN chmod +x /opt/scanray/bin/*

RUN groupadd -g 1500 scanray && \
    useradd -m -u 1001 -G scanray pupp && \
    chown -R pupp:scanray /opt/scanray && \
    chmod -R 775 /opt/scanray

USER pupp

RUN /opt/scanray/bin/nuclei -update-templates 2>/dev/null || true

ENV SCANRAY_BINARY=/opt/scanray/bin/scanray
ENV NUCLEI_BINARY=/opt/scanray/bin/nuclei
ENV PUPP_DATA_DIR=/opt/scanray/data

ENTRYPOINT ["/opt/scanray/bin/pupp"]
