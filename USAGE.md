# ocpack 使用指南

## 快速开始 - 一键部署

### 1. 创建集群项目

```bash
ocpack new cluster my-cluster
```

这将创建一个名为 `my-cluster` 的目录，包含默认的配置文件。

### 2. 配置集群信息

编辑 `my-cluster/config.toml` 文件，填写以下关键信息：

```toml
[cluster_info]
name = "my-cluster"
domain = "example.com"
openshift_version = "4.14.0"

[bastion]
ip = "192.168.1.10"
username = "root"
password = "your-password"

[registry]
ip = "192.168.1.11"
username = "root"
password = "your-password"

[[cluster.control_plane]]
name = "master-0"
ip = "192.168.1.21"
mac = "52:54:00:12:34:56"

[[cluster.worker]]
name = "worker-0"
ip = "192.168.1.31"
mac = "52:54:00:12:34:59"

[cluster.network]
cluster_network = "10.128.0.0/14"
service_network = "172.30.0.0/16"
machine_network = "192.168.1.0/24"
```

### 3. 准备 Pull Secret

从 [Red Hat Console](https://console.redhat.com/openshift/install/pull-secret) 获取 pull-secret，保存到 `my-cluster/pull-secret.txt` 文件。

### 4. 一键部署

```bash
ocpack all my-cluster
```

这个命令将自动执行以下步骤：

1. **下载安装介质** - 下载 OpenShift 客户端工具、安装程序、oc-mirror 工具等
2. **部署 Bastion 节点** - 配置 DNS 服务器和 HAProxy 负载均衡器
3. **部署 Registry 节点** - 安装和配置 Quay 镜像仓库
4. **保存镜像到本地** - 使用 oc-mirror 下载 OpenShift 镜像
5. **加载镜像到 Registry** - 将镜像推送到 Quay 仓库
6. **生成安装 ISO** - 生成包含 ignition 配置的安装 ISO

### 5. 部署完成

部署完成后，您将看到类似以下的输出：

```
🎉 OpenShift 集群 'my-cluster' 一键部署完成！
⏰ 总耗时: 45分30秒
📁 集群文件位置: /path/to/my-cluster

📋 部署结果摘要:
   • Bastion 节点: 192.168.1.10 (DNS + HAProxy)
   • Registry 节点: 192.168.1.11 (Quay 镜像仓库)
   • API 服务器: https://192.168.1.10:6443
   • 应用入口: https://192.168.1.10
   • HAProxy 统计: http://192.168.1.10:9000/stats
   • Quay 控制台: https://192.168.1.11:8443
   • 安装 ISO: /path/to/my-cluster/installation/iso/

🔧 下一步操作:
   1. 使用生成的 ISO 文件启动集群节点
   2. 监控安装进度: ocpack mon my-cluster
   3. 使用 oc 命令行工具管理集群
```

## 分步部署（高级用户）

如果您需要更精细的控制或某个步骤失败需要重试，可以使用分步部署：

```bash
# 1. 创建集群项目
ocpack new cluster my-cluster

# 2. 编辑配置文件（手动操作）
# 编辑 my-cluster/config.toml 和 my-cluster/pull-secret.txt

# 3. 下载安装介质
ocpack download my-cluster

# 4. 保存镜像到本地
ocpack save-image my-cluster

# 5. 部署 Bastion 节点
ocpack deploy-bastion my-cluster

# 6. 部署 Registry 节点
ocpack deploy-registry my-cluster

# 7. 加载镜像到 Registry
ocpack load-image my-cluster

# 8. 生成安装 ISO
ocpack generate-iso my-cluster

# 9. 使用 ISO 启动虚拟机后，监控安装进度
ocpack mon my-cluster
```

## 监控集群安装

在使用生成的 ISO 启动虚拟机后，您可以使用监控功能来跟踪安装进度：

### 基本监控

```bash
# 监控安装进度（直接透传 openshift-install 输出）
ocpack mon my-cluster
```

监控命令会显示完整的安装进度，包括：
- 安装状态更新
- 集群初始化进度
- 安装完成后的访问信息（kubeconfig 路径、密码、控制台 URL 等）

### 监控功能说明

`ocpack mon` 命令会直接执行 `openshift-install agent wait-for install-complete` 命令，并将其原始输出透传给用户。这意味着您将看到与手动执行该命令完全相同的输出。

**命令等效于**：
```bash
cd my-cluster/installation/ignition
openshift-install agent wait-for install-complete --dir .
```

**输出特点**：
- 显示 openshift-install 的原始输出
- 包括详细的安装进度信息
- 实时显示状态更新和日志
- 安装完成后显示集群访问信息

**使用前提**：
- 已经生成了 ISO 文件 (`ocpack generate-iso`)
- 已经使用 ISO 启动虚拟机并开始安装
- 在项目根目录下执行命令

## 故障排除

### 如果某个步骤失败

1. **查看错误信息** - 仔细阅读错误输出，了解失败原因
2. **修复问题** - 根据错误信息修复配置或环境问题
3. **重新执行** - 可以重新运行 `ocpack all` 或单独执行失败的步骤

### 常见问题

1. **SSH 连接失败**
   - 检查 Bastion 和 Registry 节点的 IP 地址、用户名和密码
   - 确保网络连通性

2. **下载失败**
   - 检查网络连接
   - 确认 OpenShift 版本号正确

3. **镜像操作失败**
   - 检查 pull-secret.txt 文件格式
   - 确认有足够的磁盘空间

4. **ISO 生成失败**
   - 检查所有节点的 MAC 地址是否正确填写
   - 确认 Registry 中的镜像已正确加载

## 高级选项

### 自定义镜像保存

```bash
# 包含 Operator 镜像
ocpack save-image my-cluster --include-operators

# 包含额外镜像
ocpack save-image my-cluster --additional-images image1,image2
```

### 跳过验证

```bash
# 生成 ISO 时跳过镜像验证
ocpack generate-iso my-cluster --skip-verify
```

## 前置要求

- **操作系统**: Linux (推荐 RHEL 8/9 或 CentOS Stream)
- **网络**: 确保能够访问互联网下载镜像
- **存储**: 至少 100GB 可用空间用于存储镜像
- **内存**: 建议 8GB 以上
- **Ansible**: 系统需要安装 Ansible (用于自动化部署)
- **SSH**: 确保能够 SSH 到 Bastion 和 Registry 节点

## 支持的 OpenShift 版本

- OpenShift 4.14.0 及以上版本（支持 oc-mirror 工具）
- 推荐使用最新的稳定版本 