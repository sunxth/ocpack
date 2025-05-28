# ocpack

ocpack 是一个用于离线环境中部署 OpenShift 集群的 Go 语言命令行工具，主要用于配置和部署 Bastion 和 Registry 节点，并处理相关介质下载。

## 功能特点

- 项目初始化：创建集群目录和配置文件
- 配置管理：使用简洁的 TOML 格式配置文件
- Bastion 节点部署：使用嵌入式 Ansible playbook 自动配置跳板机、DNS (bind) 和负载均衡器 (HAProxy)
- Registry 节点部署：自动配置私有镜像仓库
- 集群节点配置：支持 Control Plane 和 Worker 节点配置
- Ignition 文件生成：为离线环境生成包含 ignition 文件的 ISO 镜像
- 介质下载：自动下载离线环境所需的安装介质
  - OpenShift 客户端工具 (oc, kubectl)
  - OpenShift 安装程序 (openshift-install)
  - oc-mirror 工具 (4.14.0+ 版本支持)
  - butane 工具 (Ignition 配置生成)
  - Quay 镜像仓库安装包 (mirror-registry)

## 安装

### 从源码编译

```bash
git clone https://github.com/yourusername/ocpack.git
cd ocpack
go build -o ocpack cmd/ocpack/main.go
```

### 下载预编译二进制文件

请访问 [Release 页面](https://github.com/yourusername/ocpack/releases) 下载最新版本。

## 快速开始

### 1. 创建新集群

```bash
ocpack new cluster my-cluster
```

这将创建一个名为 `my-cluster` 的目录，并生成默认配置文件。

### 2. 编辑配置文件

编辑 `my-cluster/config.toml` 文件，填写必要的配置信息：
- Bastion 和 Registry 节点的 IP 地址、用户名、密码
- Control Plane 和 Worker 节点的 IP 地址和 MAC 地址
- 集群网络配置

### 3. 执行部署流程

```bash
# 下载安装介质
ocpack download -c my-cluster/config.toml

# 部署 Bastion 节点
ocpack deploy-bastion -c my-cluster/config.toml

# 部署 Registry 节点
ocpack deploy-registry -c my-cluster/config.toml

# 加载镜像到 Registry
ocpack load-image my-cluster

# 生成安装 ISO
ocpack generate-iso my-cluster
```

## 配置文件说明

配置文件使用 TOML 格式，主要包含以下几个部分：

### 集群基本信息

```toml
[cluster_info]
name = "my-cluster"            # 集群名称
domain = "example.com"         # 集群域名
cluster_id = "mycluster"       # 集群ID
openshift_version = "4.14.0"   # OpenShift 版本 (需要 4.14.0+ 才支持 oc-mirror)
```

### Bastion 节点配置

```toml
[bastion]
ip = "192.168.1.10"            # Bastion 节点 IP 地址
username = "root"              # SSH 用户名
ssh_key_path = "~/.ssh/id_rsa" # SSH 私钥路径 (可选)
password = "password"          # SSH 密码 (如未提供 SSH 私钥则必填)
```

### Registry 节点配置

```toml
[registry]
ip = "192.168.1.11"            # Registry 节点 IP 地址
username = "root"              # SSH 用户名
ssh_key_path = "~/.ssh/id_rsa" # SSH 私钥路径 (可选)
password = "password"          # SSH 密码 (如未提供 SSH 私钥则必填)
storage_path = "/var/lib/registry" # 镜像存储路径
```

### 集群节点配置

用于生成 ignition 文件和配置 Bastion 节点的 DNS/HAProxy 服务：

```toml
[cluster]
# Control Plane 节点 (Master 节点)
[[cluster.control_plane]]
name = "master-0"              # 节点名称
ip = "192.168.1.21"            # 节点 IP 地址
mac = "52:54:00:12:34:56"      # 节点 MAC 地址 (用于生成 ignition 文件)

[[cluster.control_plane]]
name = "master-1"
ip = "192.168.1.22"
mac = "52:54:00:12:34:57"

[[cluster.control_plane]]
name = "master-2"
ip = "192.168.1.23"
mac = "52:54:00:12:34:58"

# Worker 节点
[[cluster.worker]]
name = "worker-0"              # 节点名称
ip = "192.168.1.31"            # 节点 IP 地址
mac = "52:54:00:12:34:59"      # 节点 MAC 地址 (用于生成 ignition 文件)

[[cluster.worker]]
name = "worker-1"
ip = "192.168.1.32"
mac = "52:54:00:12:34:5a"

# 集群网络配置
[cluster.network]
cluster_network = "10.128.0.0/14"    # 集群网络 CIDR
service_network = "172.30.0.0/16"    # 服务网络 CIDR
machine_network = "192.168.1.0/24"   # 机器网络 CIDR
```

**说明**：
- **Bastion 节点**：作为负载均衡器，提供 API Server 和 Ingress 服务
- **MAC 地址**：用于生成特定节点的 ignition 配置文件
- **网络 CIDR**：用于 OpenShift 集群的网络规划

## 下载的工具说明

`ocpack download` 命令会自动下载以下工具和文件：

### 核心工具
- **openshift-client-linux-{version}.tar.gz**: OpenShift 客户端工具包，包含 `oc` 和 `kubectl` 命令
- **openshift-install-linux-{version}.tar.gz**: OpenShift 安装程序，用于生成集群配置和安装集群

### 版本相关工具
- **oc-mirror-{version}.tar.gz**: 镜像同步工具 (仅 OpenShift 4.14.0+ 版本支持)
  - 用于将 OpenShift 镜像同步到离线环境

### 通用工具
- **butane-{arch}**: Ignition 配置生成工具
  - 用于生成 CoreOS/RHCOS 的 Ignition 配置文件
  - 支持多种系统架构 (aarch64, x86_64)

- **mirror-registry-amd64.tar.gz**: Quay 镜像仓库安装包
  - 用于在离线环境中部署私有镜像仓库
  - 基于 Red Hat Quay 技术

### 版本兼容性
- OpenShift 4.14.0 以下版本：下载 openshift-client、openshift-install、butane、mirror-registry
- OpenShift 4.14.0 及以上版本：下载所有工具，包括 oc-mirror

## 开发

### 目录结构

```
ocpack/
├── cmd/                # 命令行工具入口
│   └── ocpack/        # ocpack 命令
│       ├── cmd/       # 子命令实现
│       └── main.go    # 主程序入口
├── pkg/                # 功能包
│   ├── config/        # 配置处理
│   ├── deploy/        # 部署功能
│   ├── download/      # 下载功能
│   └── utils/         # 通用工具
└── README.md          # 项目说明
```

## 技术实现

### Ansible 集成

ocpack 使用 Go 的 `embed` 包将 Ansible playbook 嵌入到二进制文件中，实现了以下特性：

- **嵌入式 Playbook**：所有 Ansible 配置文件都嵌入在二进制文件中，无需外部依赖
- **运行时提取**：在执行时自动提取到临时目录
- **模板化配置**：使用 Go template 动态生成 Ansible inventory 和变量文件
- **自动清理**：执行完成后自动清理临时文件

### 部署架构

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   ocpack 工具   │    │  Bastion 节点   │    │  Registry 节点  │
│                 │    │                 │    │                 │
│ • 配置管理      │───▶│ • DNS (bind)    │    │ • 私有镜像仓库  │
│ • 文件下载      │    │ • HAProxy       │    │ • 镜像存储      │
│ • Ansible 执行  │    │ • 负载均衡      │    │                 │
└─────────────────┘    └─────────────────┘    └─────────────────┘
                              │
                              ▼
                    ┌─────────────────┐
                    │  OpenShift 集群 │
                    │                 │
                    │ • Control Plane │
                    │ • Worker 节点   │
                    │ • 应用负载      │
                    └─────────────────┘
```

### 文件结构

```
pkg/deploy/ansible/bastion/
├── playbook.yml              # 主 playbook
├── inventory.ini             # Inventory 模板
└── templates/
    ├── named.conf.j2         # DNS 主配置
    ├── forward.zone.j2       # DNS 正向解析
    ├── reverse.zone.j2       # DNS 反向解析
    └── haproxy.cfg.j2        # HAProxy 配置
```

## 许可证

MIT 