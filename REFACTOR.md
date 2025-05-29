# ocpack 镜像管理重构说明

## 重构概述

本次重构将原本混合在 `pkg/loadimage` 包中的镜像保存和加载功能进行了分离，遵循单一职责原则，提高了代码的可维护性和可扩展性。

## 重构前的问题

1. **功能混合**: `pkg/loadimage` 包同时包含了镜像保存和加载功能
2. **命令重叠**: `load-image` 命令实际上执行了完整的 save + load 流程
3. **职责不清**: 一个包承担了两个不同的职责
4. **不符合最佳实践**: 违反了单一职责原则

## 重构后的结构

### 新的包结构

```
pkg/
├── saveimage/              # 新增：镜像保存功能
│   ├── saver.go           # 镜像保存器
│   └── templates/
│       └── imageset-config.yaml
├── loadimage/              # 重构：纯镜像加载功能
│   └── loader.go          # 镜像加载器（移除了保存功能）
└── ...
```

### 新的命令结构

```
cmd/ocpack/cmd/
├── save_image.go          # 新增：save-image 命令
├── load_image.go          # 重构：load-image 命令（只负责加载）
└── ...
```

## 功能分离

### pkg/saveimage 包

**职责**: 专门负责镜像保存功能

**主要功能**:
- 处理 pull-secret 文件
- 生成 ImageSet 配置文件
- 使用 oc-mirror 下载镜像到本地磁盘
- 验证和格式化 pull-secret

**核心类型**:
- `ImageSaver`: 镜像保存器
- `ImageSetConfig`: ImageSet 配置结构

### pkg/loadimage 包

**职责**: 专门负责镜像加载功能

**主要功能**:
- 从本地磁盘加载镜像到 registry
- 配置 CA 证书信任
- 验证 registry 连接
- 配置认证信息
- 执行镜像推送

**核心类型**:
- `ImageLoader`: 镜像加载器（简化后）

## 命令变化

### save-image 命令（新增）

```bash
# 基本用法
ocpack save-image my-cluster

# 包含 Operator 镜像
ocpack save-image my-cluster --include-operators

# 包含 Helm Charts
ocpack save-image my-cluster --include-helm

# 添加额外镜像
ocpack save-image my-cluster --additional-images image1,image2
```

### load-image 命令（重构）

```bash
# 基本用法
ocpack load-image my-cluster

# 指定 registry 参数
ocpack load-image my-cluster --registry-url registry.example.com:8443
```

## 工作流变化

### 重构前

```bash
# 一个命令完成所有操作（不够灵活）
ocpack load-image my-cluster  # 实际执行 save + load
```

### 重构后

```bash
# 分离的工作流（更灵活）
ocpack save-image my-cluster   # 只保存镜像
ocpack load-image my-cluster   # 只加载镜像
```

## 优势

1. **单一职责**: 每个包和命令都有明确的单一职责
2. **更好的可测试性**: 功能分离后更容易进行单元测试
3. **更高的灵活性**: 支持跨环境部署场景
4. **更清晰的接口**: 用户可以根据需要选择执行哪个步骤
5. **更好的错误处理**: 每个步骤的错误可以独立处理
6. **更容易维护**: 代码结构更清晰，维护成本更低

## 向后兼容性

- 移除了 `LoadImages()` 方法（包含 save + load 的混合功能）
- 新增了独立的 `save-image` 命令
- `load-image` 命令现在只执行加载功能
- 用户需要分别执行两个命令，但获得了更大的灵活性

## 使用场景

### 场景 1: 同一环境部署
```bash
ocpack save-image my-cluster
ocpack load-image my-cluster
```

### 场景 2: 跨环境部署
```bash
# 在有网络的环境中
ocpack save-image my-cluster --include-operators

# 传输 my-cluster/images/ 目录到离线环境

# 在离线环境中
ocpack load-image my-cluster
```

## 文件变化总结

### 新增文件
- `pkg/saveimage/saver.go`
- `pkg/saveimage/templates/imageset-config.yaml`
- `cmd/ocpack/cmd/save_image.go`

### 修改文件
- `pkg/loadimage/loader.go` - 移除保存功能，简化为纯加载功能
- `cmd/ocpack/cmd/load_image.go` - 更新为只调用 LoadToRegistry
- `README.md` - 更新文档反映新的命令结构

### 删除文件
- `pkg/loadimage/templates/imageset-config.yaml` - 移动到 saveimage 包

## 测试验证

重构完成后，所有命令都能正常编译和运行：

```bash
# 编译成功
go build -o build/ocpack cmd/ocpack/main.go

# 命令帮助正常
./build/ocpack --help
./build/ocpack save-image --help
./build/ocpack load-image --help
```

这次重构显著提高了代码质量，使得 ocpack 工具更加模块化和易于维护。 