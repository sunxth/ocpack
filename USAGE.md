# ocpack 使用指南

这是一个用于 OpenShift Container Platform (OCP) 部署的工具集，支持离线安装和镜像管理。

## 快速开始

### 1. 环境准备

确保您的系统满足以下要求：
- Linux x86_64 或 ARM64
- 至少 50GB 可用磁盘空间
- 网络连接（用于下载镜像和工具）

### 2. 创建集群项目

```bash
# 创建一个新的集群项目
./ocpack new cluster demo

# 这将创建 demo 目录，包含配置文件模板
cd demo
```

### 3. 编辑配置文件

编辑 `config.toml` 文件，根据您的环境调整配置：

```toml
[cluster_info]
cluster_id = "demo"
domain = "example.com"
openshift_version = "4.16.0"

[network]
cluster_network = "10.128.0.0/14"
service_network = "172.30.0.0/16"
machine_network = "192.168.1.0/24"

[registry]
registry_user = "admin"
ip = "192.168.1.100"
# 其他配置...
```

## 主要功能

### 1. 一键部署（推荐）

```bash
# ISO 模式一键部署
./ocpack all demo

# PXE 模式一键部署
./ocpack all demo --mode=pxe
```

### 2. 分步执行

#### 步骤 1：配置下载
```bash
./ocpack download demo
```

#### 步骤 2：保存镜像
```bash
./ocpack save-image demo
```

#### 步骤 3：部署 Registry
```bash
./ocpack deploy-registry demo
```

#### 步骤 4：加载镜像
```bash
./ocpack load-image demo
```

#### 步骤 5：生成安装文件
```bash
# ISO 模式
./ocpack generate-iso demo

# PXE 模式
./ocpack generate-pxe demo
```

### 3. 镜像加载认证配置

在运行 `load-image` 命令之前，需要确保正确配置了认证信息：

#### 3.1 准备 pull-secret.txt
从 Red Hat 官网下载您的 pull secret 文件，并将其放置在集群目录中：
```bash
# 将下载的 pull-secret.txt 放在集群目录
cp /path/to/your/pull-secret.txt demo/pull-secret.txt
```

#### 3.2 配置 CA 证书信任（如果使用自签名证书）
如果您的 registry 使用自签名证书，需要配置 CA 证书信任：

**在 CentOS/RHEL 系统上：**
```bash
# 复制 CA 证书到系统信任目录
sudo cp demo/registry/10.10.195.98/rootCA.pem /etc/pki/ca-trust/source/anchors/
sudo update-ca-trust
```

**在 Ubuntu/Debian 系统上：**
```bash
# 复制 CA 证书到系统信任目录
sudo cp demo/registry/10.10.195.98/rootCA.pem /usr/local/share/ca-certificates/ocpack-registry.crt
sudo update-ca-certificates
```

#### 3.3 验证 Registry 连接
在加载镜像之前，可以验证 registry 连接：
```bash
# 使用 podman 测试连接（需要先配置证书信任）
podman login registry.demo.example.com:8443
```

#### 3.4 运行镜像加载
配置完成后，运行镜像加载命令：
```bash
./ocpack load-image demo
```

**注意事项：**
- 命令会自动创建合并认证文件 `demo/registry/merged-auth.json`
- 如果遇到认证错误，请检查 pull-secret.txt 格式是否正确
- 如果遇到 SSL 证书错误，请确保已正确配置 CA 证书信任

### 4. 高级选项

#### 启用重试机制
```bash
./ocpack load-image demo --enable-retry --max-retries=5
```

#### 干运行模式
```bash
./ocpack load-image demo --dry-run
```

#### 调试模式
```bash
./ocpack load-image demo --log-level=debug
```

## 故障排除

### 1. 镜像加载失败

**问题：** `authentication required` 错误
**解决方案：**
1. 检查 `pull-secret.txt` 文件是否存在且格式正确
2. 确认 registry 服务正在运行
3. 验证 CA 证书是否已正确配置

**问题：** SSL 证书错误
**解决方案：**
1. 配置 CA 证书信任（参见上文 3.2 节）
2. 或者在测试环境中使用 `--src-tls-verify=false --dest-tls-verify=false` 参数

### 2. 网络连接问题

**问题：** 下载超时
**解决方案：**
1. 检查网络连接
2. 配置代理设置（如需要）
3. 增加超时时间

### 3. 磁盘空间不足

**问题：** 镜像保存失败
**解决方案：**
1. 清理不必要的文件
2. 增加存储空间
3. 调整镜像集配置，减少包含的 operator

## 配置文件详解

### config.toml 配置项说明

```toml
[cluster_info]
cluster_id = "demo"           # 集群名称
domain = "example.com"        # 集群域名
openshift_version = "4.16.0"  # OpenShift 版本

[network]
cluster_network = "10.128.0.0/14"    # Pod 网络
service_network = "172.30.0.0/16"    # 服务网络
machine_network = "192.168.1.0/24"   # 节点网络

[registry]
registry_user = "admin"        # Registry 用户名
ip = "192.168.1.100"          # Registry IP 地址
username = "root"             # SSH 用户名
password = "your_password"    # SSH 密码（可选）
ssh_key_path = "/path/to/key" # SSH 密钥路径（可选）

[pxe]
ip = "192.168.1.101"         # PXE 服务器 IP
username = "root"            # SSH 用户名
password = "your_password"   # SSH 密码（可选）
ssh_key_path = "/path/to/key" # SSH 密钥路径（可选）

[bastion]
ip = "192.168.1.102"         # 堡垒机 IP
username = "root"            # SSH 用户名
password = "your_password"   # SSH 密码（可选）
ssh_key_path = "/path/to/key" # SSH 密钥路径（可选）

[save_image]
include_operators = true      # 是否包含 Operator
ops = [                      # 要包含的 Operator 列表
    "local-storage-operator",
    "elasticsearch-operator",
    "cluster-logging"
]
additional_images = [        # 额外镜像列表
    "registry.redhat.io/ubi8/ubi:latest"
]
```

## 文件结构

```
demo/                           # 集群项目目录
├── config.toml                 # 主配置文件
├── pull-secret.txt            # Red Hat pull secret
├── downloads/                 # 下载的文件
│   ├── bin/                   # 二进制工具
│   ├── images/                # ISO 镜像
│   └── others/                # 其他文件
├── images/                    # 保存的容器镜像
│   ├── mirror_000001.tar     # 镜像存档
│   └── working-dir/          # oc-mirror 工作目录
├── registry/                  # Registry 配置和证书
│   └── 192.168.1.100/        # Registry IP 目录
│       ├── config.yaml       # Registry 配置
│       ├── rootCA.pem        # CA 证书
│       ├── ssl.cert          # SSL 证书
│       ├── ssl.key           # SSL 私钥
│       └── merged-auth.json  # 合并认证文件
├── iso/                       # 生成的 ISO 文件
└── pxe/                       # 生成的 PXE 文件
```

## 常见问题

### Q: 如何更新镜像？
A: 重新运行 `save-image` 和 `load-image` 命令即可。

### Q: 如何添加新的 Operator？
A: 编辑 `config.toml` 文件中的 `ops` 列表，然后重新运行相关命令。

### Q: 如何备份项目？
A: 直接打包整个集群目录即可，包含所有配置和镜像数据。

### Q: 如何在不同环境间迁移？
A: 复制整个集群目录到目标环境，根据需要调整 `config.toml` 中的 IP 地址。

## 支持

如果遇到问题，请：
1. 检查日志文件（在相应的工作目录中）
2. 使用 `--log-level=debug` 获取详细信息
3. 查看项目文档或提交 issue 