# Cache Optimization for ocpack

## Overview

This document describes the cache optimization improvements made to ocpack to solve the disk space issues with oc-mirror cache accumulation.

## Problem

Previously, oc-mirror was using the default cache location `$HOME/.oc-mirror/`, which caused:
- Accumulation of large cache files (111GB+ reported)
- Cache files scattered in user home directory
- Difficulty in managing and cleaning cache per cluster
- Potential disk space exhaustion

## Solution

### 1. Workspace & Cache Directory Control

**Modified files:** `pkg/mirror/wrapper/wrapper.go`

- Added `setupWorkspaceAndCache()` function to explicitly control oc-mirror workspace and cache locations
- Workspace: `{cluster-dir}/images/working-dir`
- Cache: `{cluster-dir}/images/cache`
- Different operations use different parameter strategies:
  - **MirrorToDisk**: Only uses `--cache-dir` (no `--workspace` needed)
  - **DiskToMirror**: Uses both `--workspace` and `--cache-dir`
  - **MirrorDirect**: Uses both `--workspace` and `--cache-dir`

### 2. oc-mirror Parameter Fix

**Issue Found**: When using `file://` destination (mirror-to-disk), oc-mirror doesn't accept `--workspace` parameter.

**Solution**: 
- Removed `--workspace` parameter from MirrorToDisk operations
- Only use `--cache-dir` for mirror-to-disk operations
- oc-mirror automatically creates workspace in the destination directory

### 3. Cache Management Commands

**New file:** `cmd/ocpack/cmd/clean_cache.go`

Added `clean-cache` command with the following features:

```bash
# Clean cache for specific cluster
ocpack clean-cache my-cluster

# Show cache information without cleaning
ocpack clean-cache my-cluster --info

# Clean cache for cluster specified in config.toml
ocpack clean-cache --config config.toml
```

### 4. Cache Information & Statistics

**New functions in wrapper.go:**

- `CleanCache()` - Remove cache directory and show size before cleaning
- `GetCacheInfo()` - Get comprehensive cache and workspace information
- `calculateDirectorySize()` - Calculate directory size recursively
- `formatBytes()` - Human-readable byte formatting (B, KB, MB, GB, etc.)

## Benefits

1. **Controlled Storage**: Cache files are now stored within each cluster directory
2. **Easy Management**: Per-cluster cache management and cleaning
3. **Disk Space Monitoring**: View cache size and location information
4. **Automated Cleanup**: Built-in cache cleaning capabilities
5. **Better Isolation**: Each cluster has its own cache directory
6. **Correct oc-mirror Usage**: Fixed parameter conflicts for different operation types

## oc-mirror Operations Comparison

| Operation | Source | Destination | --workspace | --cache-dir | Notes |
|-----------|--------|-------------|-------------|-------------|-------|
| MirrorToDisk | Registry | `file://` | ❌ | ✅ | oc-mirror creates workspace automatically |
| DiskToMirror | `file://` | Registry | ✅ | ✅ | Requires explicit workspace |
| MirrorDirect | Registry | Registry | ✅ | ✅ | Mirror-to-mirror operation |

## Error Fix

**Previous Error**:
```
Error: when destination is file://, mirrorToDisk workflow is assumed, and the --workspace argument is not needed
```

**Root Cause**: Using `--workspace` parameter with `file://` destination in mirror-to-disk operations.

**Fix**: Remove `--workspace` parameter for MirrorToDisk operations, keep only `--cache-dir`.

## Usage Examples

### View Cache Information
```bash
ocpack clean-cache test-cluster --info
```

Output:
```
📊 Cache Information for cluster: test-cluster
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
📁 Cache Directory: /path/to/test-cluster/images/cache
💾 Cache Size: 15.2 GB
📅 Last Modified: 2025-01-15 10:30:45
🗂️  Workspace Directory: /path/to/test-cluster/images/working-dir
💾 Workspace Size: 2.1 GB
📅 Last Modified: 2025-01-15 10:30:45
```

### Clean Cache
```bash
ocpack clean-cache test-cluster
```

Output:
```
🧹 Cleaning cache for cluster: test-cluster
📊 Current cache size: 15.2 GB
🧹 Cleaning cache directory: /path/to/test-cluster/images/cache
✅ Cache cleaned successfully

💡 Tips:
   - Cache will be recreated automatically on next mirror operation
   - Regular cache cleaning helps maintain disk space
   - Use 'ocpack clean-cache --info' to check cache size
```

### Fixed save-image Command
```bash
ocpack save-image test
```

Now works correctly without workspace parameter conflicts:
```
�� Starting mirror-to-disk operation...
📁 Workspace directory: /root/test/images/working-dir
💾 Cache directory: /root/test/images/cache
📋 Using configuration generator (based on config.toml)
💾 Using cache: /root/test/images/cache
📁 Mirror destination: file:///root/test/images/mirror
✅ Mirror operation completed
```

## Migration Notes

### Existing Deployments

If you have existing deployments with cache in `$HOME/.oc-mirror/`:

1. Check current cache size: `du -sh $HOME/.oc-mirror/`
2. Clean old cache: `rm -rf $HOME/.oc-mirror/`
3. New cache will be created in cluster directories on next mirror operation

### Automatic Migration

The optimization is backward compatible - no manual migration required. New cache directories will be automatically created in the correct locations.

## Log Message Standardization

As part of this optimization, all user-facing log messages in the mirror wrapper have been standardized to English for consistency:

- Error messages: Translated from Chinese to English
- Debug messages: Standardized format
- User guidance: Clear English instructions

## Directory Structure

After optimization, each cluster will have:

```
cluster-name/
├── config.toml
├── pull-secret.txt
├── images/
│   ├── cache/           # oc-mirror cache directory
│   ├── working-dir/     # oc-mirror workspace (for disk-to-mirror operations)
│   └── mirror/          # Saved images (mirror-to-disk destination)
└── registry/
    └── merged-auth.json
```

## Performance Impact

- **Positive**: Faster access to cluster-specific cache
- **Positive**: Easier parallel operations for multiple clusters
- **Neutral**: No impact on mirror operation performance
- **Positive**: Reduced risk of cache conflicts between clusters
- **Fixed**: Eliminated oc-mirror parameter conflicts 