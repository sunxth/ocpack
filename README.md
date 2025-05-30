# ocpack

ocpack 是一个用于离线环境中部署 OpenShift 集群的 Go 语言命令行工具，主要用于配置和部署 Bastion 和 Registry 节点，并处理相关介质下载。

## 功能特点

- 项目初始化：创建集群目录和配置文件
- 配置管理：使用简洁的 TOML 格式配置文件
- Bastion 节点部署：使用嵌入式 Ansible playbook 自动配置跳板机、DNS (bind) 和负载均衡器 (HAProxy)
- Registry 节点部署：自动配置私有镜像仓库
- 集群节点配置：支持 Control Plane 和 Worker 节点配置
- ISO 生成：生成包含 ignition 配置的 OpenShift 安装 ISO 镜像
- 介质下载：自动下载离线环境所需的安装介质
  - OpenShift 客户端工具 (oc, kubectl)
  - OpenShift 安装程序 (openshift-install)
  - oc-mirror 工具 (4.14.0+ 版本支持)
  - butane 工具 (Ignition 配置生成)
  - Quay 镜像仓库安装包 (mirror-registry)
- 镜像管理：分离的镜像保存和加载功能
  - **save-image**: 使用 oc-mirror 保存镜像到本地磁盘
  - **load-image**: 从本地磁盘加载镜像到 registry

## 安装

### 使用 Makefile 构建

推荐使用项目提供的 Makefile 进行构建，支持多平台交叉编译：

```bash
# 构建当前平台
make build

# 构建所有支持的平台
make build-all

# 构建特定平台
make linux/amd64
make darwin/arm64
make windows/amd64
```

详细的构建说明请参考：[BUILD.md](BUILD.md)

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

# 保存镜像到本地磁盘
ocpack save-image my-cluster

# 加载镜像到 Registry
ocpack load-image my-cluster

# 生成安装 ISO
ocpack generate-iso my-cluster
```

**注意**：在执行 `save-image` 命令之前，需要先获取 Red Hat pull-secret：

1. 访问 [Red Hat OpenShift Pull Secret](https://console.redhat.com/openshift/install/pull-secret)
2. 登录您的 Red Hat 账户
3. 下载 pull-secret 文件
4. 将文件保存为 `my-cluster/pull-secret.txt`

pull-secret 文件格式示例：
```json
{"auths":{"cloud.openshift.com":{"auth":"...","email":"..."},"quay.io":{"auth":"...","email":"..."},"registry.connect.redhat.com":{"auth":"...","email":"..."},"registry.redhat.io":{"auth":"...","email":"..."}}}
```

## ISO 生成功能

ocpack 提供了完整的 ISO 生成功能，用于创建 OpenShift 集群的安装镜像：

### 生成安装 ISO

```bash
# 基本用法
ocpack generate-iso my-cluster

# 指定输出目录
ocpack generate-iso my-cluster --output /path/to/output

# 只生成特定类型的节点 ISO
ocpack generate-iso my-cluster --master-only
ocpack generate-iso my-cluster --worker-only
```

### 生成的文件结构

执行 `generate-iso` 命令后，将在集群目录下创建以下结构：

```
my-cluster/
└── installation/
    ├── install-config.yaml      # OpenShift 安装配置
    ├── agent-config.yaml        # Agent 安装配置
    ├── ignition/                # Ignition 配置文件
    │   ├── auth/               # 集群认证信息
    │   ├── .openshift_install.log
    │   └── .openshift_install_state.json
    └── iso/                     # 生成的 ISO 文件
        └── my-cluster-agent.x86_64.iso
```

### 配置文件说明

#### install-config.yaml
包含 OpenShift 集群的基本配置：
- 集群名称和域名
- 节点数量和架构
- 网络配置
- 平台配置（baremetal 或 none）
- Pull Secret 和 SSH 密钥
- 额外的信任证书（如果使用私有 registry）

#### agent-config.yaml
包含 Agent 安装的主机配置：
- 每个节点的主机名、角色、MAC 地址
- 网络接口配置
- IP 地址和网络设置
- DNS 和路由配置

### 使用生成的 ISO

1. **刻录 ISO**: 将生成的 ISO 文件刻录到 USB 或 DVD
2. **启动节点**: 使用 ISO 启动各个集群节点
3. **自动安装**: Agent 会自动根据 MAC 地址匹配配置并安装系统
4. **集群形成**: 所有节点启动后会自动形成 OpenShift 集群

### 前置条件

在运行 `generate-iso` 之前，请确保：

1. **已下载工具**: 运行 `ocpack download` 下载 `openshift-install` 等工具
2. **配置完整**: 集群配置文件中的所有必需字段都已填写
3. **Pull Secret**: `pull-secret.txt` 文件存在于集群目录中
4. **网络规划**: 确保 MAC 地址、IP 地址等网络配置正确
5. **Registry 就绪**: 如果使用私有 registry，请确保已运行 `ocpack load-image` 加载镜像

### 智能二进制提取

`generate-iso` 命令具有智能的二进制文件提取功能：

1. **版本验证**: 自动验证配置文件中的 OpenShift 版本与 `openshift-install` 工具版本是否匹配
2. **Registry 验证**: 检查私有 registry 中是否存在对应版本的 release image
3. **自动提取**: 从 registry 中提取与镜像版本完全匹配的 `openshift-install` 二进制文件
4. **版本兼容**: 
   - OpenShift 4.14+ 使用 IDMS (ImageDigestMirrorSet) 配置
   - OpenShift 4.13 及以下使用 ICSP (ImageContentSourcePolicy) 配置
5. **回退机制**: 如果从 registry 提取失败，自动回退到使用下载的工具

这确保了生成的 ISO 与 registry 中的镜像版本完全一致，避免版本不匹配问题。

### 故障排除

如果 ISO 生成失败，请检查：

1. **工具完整性**: 确保 `openshift-install` 工具已正确下载
2. **配置有效性**: 验证集群配置文件的语法和内容
3. **权限问题**: 确保有足够的权限创建文件和目录
4. **磁盘空间**: 确保有足够的磁盘空间存储生成的文件
5. **版本匹配**: 确保配置文件中的 OpenShift 版本与下载的工具版本一致
6. **Registry 连接**: 如果使用私有 registry，确保网络连接正常
7. **镜像完整性**: 验证 registry 中的 release image 是否完整
8. **依赖工具**: 确保系统中安装了 `skopeo` 工具（用于镜像验证）

#### 常见错误及解决方案

**错误**: "registry 中缺少 release image"
- **解决**: 运行 `ocpack load-image` 确保镜像已正确加载到 registry

**错误**: "版本不匹配"
- **解决**: 检查配置文件中的 `openshift_version` 是否与下载的工具版本一致

**错误**: "提取 openshift-install 失败"
- **解决**: 这在 4.14 以下版本是正常的，工具会自动回退到使用下载的版本

**错误**: "skopeo 命令不存在"
- **解决**: 安装 skopeo 工具：
  ```bash
  # RHEL/CentOS
  sudo yum install skopeo
  
  # Ubuntu/Debian
  sudo apt-get install skopeo
  
  # macOS
  brew install skopeo
  ```

## 镜像管理工作流

ocpack 提供了分离的镜像管理命令，支持灵活的离线部署场景：

### 1. 保存镜像 (save-image)

在有网络连接的环境中，使用 `save-image` 命令下载并保存 OpenShift 镜像：

```bash
# 基本用法
ocpack save-image my-cluster

# 包含 Operator 镜像
ocpack save-image my-cluster --include-operators

# 包含 Helm Charts
ocpack save-image my-cluster --include-helm

# 添加额外镜像
ocpack save-image my-cluster --additional-images registry.redhat.io/ubi8/ubi:latest,registry.redhat.io/ubi9/ubi:latest
```

此命令会：
- 验证 pull-secret 文件
- 生成 ImageSet 配置文件
- 使用 oc-mirror 下载镜像到 `my-cluster/images/` 目录

### 2. 加载镜像 (load-image)

在离线环境中，使用 `load-image` 命令将保存的镜像加载到 registry：

```bash
# 基本用法
ocpack load-image my-cluster

# 指定 registry 参数（可选）
ocpack load-image my-cluster --registry-url registry.example.com:8443 --username admin --password password
```

此命令会：
- 验证本地镜像目录存在
- 配置 CA 证书信任
- 验证 registry 连接
- 配置认证信息
- 将镜像推送到 registry

### 3. 工作流示例

**场景 1: 同一环境**
```bash
# 一次性完成保存和加载
ocpack save-image my-cluster
ocpack load-image my-cluster
```

**场景 2: 跨环境部署**
```bash
# 在有网络的环境中
ocpack save-image my-cluster --include-operators

# 将 my-cluster/images/ 目录传输到离线环境

# 在离线环境中
ocpack load-image my-cluster
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
│   ├── saveimage/     # 镜像保存功能
│   ├── loadimage/     # 镜像加载功能
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

### 镜像管理架构

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│  save-image     │    │   本地磁盘      │    │  load-image     │
│                 │    │                 │    │                 │
│ • pull-secret   │───▶│ • images/       │───▶│ • CA 证书配置   │
│ • oc-mirror     │    │ • workspace/    │    │ • 认证配置      │
│ • ImageSet 配置 │    │ • 镜像文件      │    │ • registry 推送 │
└─────────────────┘    └─────────────────┘    └─────────────────┘
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

pkg/saveimage/
├── saver.go                  # 镜像保存逻辑
└── templates/
    └── imageset-config.yaml  # ImageSet 配置模板

pkg/loadimage/
└── loader.go                 # 镜像加载逻辑
```

## 许可证

MIT 

## 依赖
需要 ansible 和 sshpass 以及 nmstate