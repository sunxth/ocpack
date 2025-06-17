# Ignition 文件和监控说明

## Ignition 文件生成

在 `ocpack generate-iso` 或 `ocpack setup-pxe` 过程中，会生成以下重要文件用于集群安装和监控：

### 生成的文件结构

```
my-cluster/
├── installation/
│   ├── ignition/                    # 监控目录
│   │   ├── auth/                    # 集群认证信息
│   │   │   ├── kubeconfig          # Kubernetes 配置文件
│   │   │   └── kubeadmin-password  # 管理员密码
│   │   ├── .openshift_install.log  # 安装日志
│   │   └── .openshift_install_state.json  # 安装状态
│   ├── iso/                         # ISO 文件目录
│   │   └── my-cluster-agent.x86_64.iso
│   ├── install-config.yaml          # 安装配置
│   └── agent-config.yaml            # Agent 配置
```

### 文件说明

#### 1. install-config.yaml
包含集群的基本配置信息：
- 集群名称和域名
- 网络配置
- 节点规格
- Pull Secret
- SSH 公钥
- 镜像源配置

#### 2. agent-config.yaml
包含 Agent-based 安装的特定配置：
- 主机配置（IP、MAC 地址）
- 网络接口配置
- Rendezvous IP
- DNS 服务器

#### 3. ignition/ 目录
这是监控功能使用的关键目录：
- **auth/**: 包含集群访问凭据
- **.openshift_install.log**: 详细的安装日志
- **.openshift_install_state.json**: 安装状态和进度信息

## 监控工作原理

### 监控命令执行流程

1. **查找 openshift-install 工具**
   - 优先使用从 registry 提取的版本
   - 回退到下载的版本
   - 最后尝试系统路径

2. **读取安装日志文件**
   - 读取 `.openshift_install.log` 文件
   - 显示最近的重要日志条目（最后10条）
   - 分析日志内容确定当前安装阶段

3. **执行监控命令**
   ```bash
   openshift-install agent wait-for install-complete --dir ignition/
   ```

4. **综合分析状态**
   - 结合日志文件和命令输出分析状态
   - 即使命令无新输出也能显示历史信息
   - 提供准确的安装阶段和进度信息

### 监控状态说明

| 状态 | 图标 | 说明 |
|------|------|------|
| 验证中 | 🔍 | 集群正在进行安装前验证 |
| 等待中 | ⏳ | 等待集群初始化或组件启动 |
| 安装中 | 🔄 | 正在安装集群组件 |
| API初始化 | 🔧 | Kubernetes API已初始化 |
| Bootstrap完成 | 🚀 | Bootstrap阶段已完成 |
| 集群初始化 | ⚙️ | 集群正在初始化，显示详细进度 |
| 完成 | ✅ | 集群安装已完成 |
| 错误 | ❌ | 安装过程中出现错误 |

### 日志级别说明

监控过程中会显示不同级别的日志信息：

| 图标 | 级别 | 说明 |
|------|------|------|
| ℹ️ | INFO | 一般信息，如安装进度 |
| ⚠️ | WARNING | 警告信息，通常是验证失败或配置问题 |
| ❌ | ERROR | 错误信息，需要立即关注 |
| 🔄 | STATUS | 主机状态更新信息 |

## 使用监控功能

### 基本用法

```bash
# 持续监控安装进度
ocpack mon my-cluster

# 获取集群凭据
ocpack mon my-cluster --credentials
```

## 故障排除

### 常见问题

1. **ignition 目录不存在**
   ```
   ❌ ignition 目录不存在: /path/to/cluster/installation/ignition
   ```
   **解决方案**: 确保已经运行 `ocpack generate-iso` 生成了 ISO 文件或 `ocpack setup-pxe` 设置了 PXE 环境

2. **找不到 openshift-install 工具**
   ```
   ❌ 找不到 openshift-install 工具
   ```
   **解决方案**: 
   - 确保已经运行 `ocpack download` 下载了安装工具
   - 支持带版本号的文件名（如 `openshift-install-4.17.0-registry.dr.example.com`）
   - 可以在集群目录内直接执行 `ocpack mon cluster-name`
   - 工具会自动查找当前目录中以 `openshift-install` 开头的文件

3. **监控超时**
   ```
   ⏰ 监控超时 (2h0m0s)，停止监控
   ```
   **解决方案**: 增加超时时间或检查集群安装是否正常进行

### 手动监控

如果自动监控有问题，可以手动执行监控命令：

```bash
cd my-cluster/installation/ignition
../../openshift-install-4.17.0-registry.my-cluster.example.com agent wait-for install-complete --dir .
```

## 集群访问

安装完成后，可以通过以下方式访问集群：

### 1. 使用 kubeconfig

```bash
export KUBECONFIG=my-cluster/installation/ignition/auth/kubeconfig
oc get nodes
```

### 2. 获取管理员密码

```bash
cat my-cluster/installation/ignition/auth/kubeadmin-password
```

### 3. 访问 Web 控制台

```
URL: https://console-openshift-console.apps.my-cluster.example.com
用户名: kubeadmin
密码: (从上面的文件获取)
```

## 最佳实践

1. **监控建议**
   - 使用 `--follow` 参数持续监控
   - 设置合理的超时时间（通常 2-3 小时）
   - 保存监控日志以便故障排除

2. **安全建议**
   - 妥善保管 kubeconfig 和密码文件
   - 安装完成后及时更改默认密码
   - 配置适当的 RBAC 权限

3. **备份建议**
   - 备份整个 installation 目录
   - 特别注意 auth 目录中的凭据文件
   - 保存安装日志用于后续参考 