# Update-APT-Source GitHub Action

A GitHub Action for managing Debian/Ubuntu APT repositories, parsing .deb packages and publishing them to cloud storage services.

## Features

- Parse and process Debian package (.deb) files
- Support various compression formats (gz, xz, zst) for control files
- Generate complete APT repository structure (Packages, Release files)
- Calculate and verify checksums (MD5, SHA1, SHA256, SHA512)
- Implement GPG signing to ensure repository security
- Support multiple architectures (amd64, arm64, etc.)
- Support multiple Ubuntu distributions (bionic, focal, jammy, noble, etc.)
- Integration with Aliyun OSS storage service

## Usage in GitHub Workflow

Create a workflow file in your GitHub repository (e.g.: `.github/workflows/update-apt.yml`):

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
    - name: Checkout code
      uses: actions/checkout@v3
    - name: Update APT Source
      uses: coscene-io/update-apt-source@v1
      with:
        ubuntu-distro: noble
        deb-paths: |
            ./dist/myapp_1.0.0_amd64.deb
            ./dist/myapp_1.0.0_arm64.deb
        architectures: |
            amd64
            arm64
        oss-key-id: ${{ secrets.ALIYUN_ACCESS_KEY_ID }}
        oss-key-secret: ${{ secrets.ALIYUN_ACCESS_KEY_SECRET }}
        gpg-private-key: ${{ secrets.GPG_PRIVATE_KEY }}
```

## Inputs

| Input Name | Description | Required |
|------------|-------------|----------|
| `ubuntu-distro` | Ubuntu distribution codename (e.g., `focal`, `jammy`, or `all`) | Yes |
| `deb-paths` | Paths to .deb packages, separated by newlines | Yes |
| `architectures` | Architectures for each .deb package, separated by newlines, in the same order as deb-paths, with the same number of entries as deb-paths | Yes |
| `oss-key-id` | Aliyun OSS Access Key ID | Yes |
| `oss-key-secret` | Aliyun OSS Access Key Secret | Yes |
| `gpg-private-key` | GPG private key for signing | Yes |

## How It Works

1. Parse specified .deb packages and extract metadata
2. Generate APT repository structure based on specified Ubuntu distribution and architecture
3. Create Packages file containing detailed information of all packages
4. Generate and sign Release file to ensure repository integrity
5. Upload packages and metadata files to Aliyun OSS storage

## Security Note

Always use GitHub repository Secrets to store sensitive information like keys and tokens. Never expose these values directly in your workflow files.
