# Update-APT-Source GitHub Action

一个用于管理Debian/Ubuntu APT软件源的GitHub Action，支持解析.deb包并将其发布到云存储服务。

## 功能特点

- 解析和处理Debian软件包(.deb)文件
- 支持多种压缩格式(gz、xz、zst)的控制文件
- 生成完整的APT仓库结构(Packages、Release文件等)
- 计算并验证各种校验和(MD5, SHA1, SHA256, SHA512)
- 使用GPG进行签名，确保软件源安全性
- 支持多架构(amd64, arm64等)
- 支持多个Ubuntu发行版(bionic, focal, jammy, noble等)
- 集成云存储服务(目前支持阿里云OSS和AWS S3)

## 在GitHub Workflow中使用

在你的GitHub仓库中创建一个工作流程文件（如：`.github/workflows/update-apt.yml`）：

```yaml
name: Update APT Repository
on:
  push:
    tags:
      - 'v*'

jobs:
  update-apt-repo:
  runs-on: ubuntu-latest
  steps:
    - name: Update APT Source (OSS)
      uses: coscene-io/update-apt-source@test
      with:
        ubuntu_distro: foxy
        deb_paths: /workspace/myapp_1.0.0_amd64.deb,/workspace/myapp_1.0.0_arm64.deb
        architectures: amd64,arm64
        storage_type: oss
        endpoint: https://oss-***.aliyuncs.com
        region: cn-***
        bucket_name: ***
        access_key_id: key
        access_key_secret: secret
        gpg_private_key: private_key
```

## 输入参数

| 参数名                 | 描述                                                       | 是否必需 |
|---------------------|----------------------------------------------------------|------|
| `ubuntu_distro`     | Ubuntu发行版代号(如`focal`, `jammy` 等，或者 `all`)                | 是    |
| `deb_paths`         | .deb包的路径，多个路径用换行符或逗号分隔                                   | 是    |
| `architectures`     | 对应每个.deb包的架构，多个架构用换行符或逗号分隔，顺序与deb-paths一致，数量与deb-paths一致 | 是    |
| `storage_type`      | 云存储类型，目前支持aws或oss                                        | 是    |
| `endpoint`          | 云存储服务端点                                                  | 是    |
| `region`            | 云存储区域                                                    | 是    |
| `bucket_name`       | 云存储桶名称                                                   | 是    |
| `access_key_id`     | 云存储访问密钥ID                                                | 是    |
| `access_key_secret` | 云存储访问密钥Secret                                            | 是    |
| `gpg_private_key`   | 用于签名的GPG私钥                                               | 是    |

## 工作原理

1. 解析指定的.deb包，提取元数据信息
2. 根据指定的Ubuntu发行版和架构，生成APT仓库结构
3. 生成Packages文件，包含所有软件包的详细信息
4. 创建并签名Release文件，确保软件源完整性
5. 将软件包和元数据文件上传到云存储服务(目前支持阿里云OSS和AWS S3)

## 安全提示

存储敏感信息（如密钥和令牌）请使用GitHub仓库的Secrets功能。请勿直接在工作流文件中暴露这些值。