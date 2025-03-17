package main

import (
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"github.com/coscene-io/update-apt-source/config"
	"github.com/coscene-io/update-apt-source/deb"
	"github.com/coscene-io/update-apt-source/release"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"golang.org/x/crypto/openpgp"
	"golang.org/x/crypto/openpgp/armor"
	"golang.org/x/crypto/openpgp/clearsign"
)

var debug bool = false

var supportedUbuntuDistros = []string{
	"bionic",
	"focal",
	"jammy",
	"noble",
}

func main() {

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Printf("获取当前工作目录失败: %v\n", err)
	} else {
		fmt.Printf("当前工作目录: %s\n", cwd)
	}

	// 列出某个目录内容（例如dist目录）
	dirToList := "/__w/coListener/coListener"
	entries, err := os.ReadDir(dirToList)
	if err != nil {
		fmt.Printf("列出目录 %s 失败: %v\n", dirToList, err)
	} else {
		fmt.Printf("\n%s 目录内容:\n", dirToList)
		for _, entry := range entries {
			info, _ := entry.Info()
			size := ""
			if info != nil {
				size = fmt.Sprintf("%d bytes", info.Size())
			}
			fmt.Printf("  - %s %s\n", entry.Name(), size)
		}
		fmt.Println()
	}

	cfg := parseConfig()
	if !cfg.IsValid() {
		panic("config invalid!")
	}

	fmt.Printf("Parse configuration:\n")
	fmt.Printf("  Ubuntu Distribution: %s\n", cfg.UbuntuDistro)
	fmt.Printf("  Number of packages to process: %d\n", len(cfg.DebPaths))
	for i, path := range cfg.DebPaths {
		fmt.Printf("    Package %d: %s (Architecture: %s)\n", i+1, path, cfg.Architectures[i])
	}

	fmt.Printf("\nInitialize OSS clinet:\n")
	client, err := oss.New(
		"oss-cn-hangzhou.aliyuncs.com",
		cfg.AccessKeyId,
		cfg.AccessKeySecret,
	)
	if err != nil {
		panic(fmt.Sprintf("Failed to initialize OSS client: %v", err))
	}
	fmt.Printf("  Initialize OSS client... ✓\n")

	bucket, err := client.Bucket("coscene-download")
	if err != nil {
		panic(fmt.Sprintf("Failed to get bucket: %v", err))
	}
	fmt.Printf("  Bucket accessing... ✓\n")

	configMap := make(map[string][]*config.SingleConfig)
	if cfg.UbuntuDistro == "all" {
		for i := range cfg.DebPaths {
			for _, distro := range supportedUbuntuDistros {
				configMap[distro] = append(configMap[distro], &config.SingleConfig{
					DebPath:      cfg.DebPaths[i],
					Architecture: cfg.Architectures[i],
					Container:    "stable",
				})
			}
		}
	} else {
		for i := range cfg.DebPaths {
			configMap[cfg.UbuntuDistro] = append(configMap[cfg.UbuntuDistro], &config.SingleConfig{
				DebPath:      cfg.DebPaths[i],
				Architecture: cfg.Architectures[i],
				Container:    "main",
			})
		}
	}

	for distro, cfgList := range configMap {
		fmt.Printf("\nUbuntu distrobution: %s\n", distro)

		for i, curConfig := range cfgList {
			fmt.Printf("  [%d/%d] Processing package (%s, %s, %s):\n",
				i+1, len(cfgList), distro, curConfig.Architecture, curConfig.DebPath)

			// Upload deb file and get checksums
			fmt.Printf("    Uploading deb package...  ")
			debInfo, err := uploadDebFile(bucket, curConfig, distro)
			if err != nil {
				panic(fmt.Sprintf("**Failed to upload deb package: %v**", err))
			}
			fmt.Printf("✓\n")

			// Update Packages file
			fmt.Printf("    Updating Packages file...  ")
			packagesContent, err := updatePackages(bucket, curConfig, debInfo, distro)
			if err != nil {
				panic(fmt.Sprintf("**Failed to update Packages: %v**", err))
			}
			fmt.Printf("✓\n")

			// Generate and upload Packages.gz
			fmt.Printf("    Generating and uploading Packages.gz... ")
			err = generatePackagesGz(bucket, packagesContent, curConfig, distro)
			if err != nil {
				panic(fmt.Sprintf("**Failed to generate Packages.gz: %v**", err))
			}
			fmt.Printf("✓\n")
		}

		// Update Release file
		fmt.Printf("\n  Updating Release file... ")
		releaseContent, err := updateRelease(bucket, cfgList, distro)
		if err != nil {
			panic(fmt.Sprintf("**Failed to update Release: %v**", err))
		}
		fmt.Printf("✓\n")

		// Sign and generate Release.gpg and InRelease
		fmt.Printf("\n  Generating signature files... ")
		err = signReleaseFiles(bucket, releaseContent, cfg, distro)
		if err != nil {
			panic(fmt.Sprintf("**Failed to sign files: %v**", err))
		}
		fmt.Printf("✓\n")
	}
	fmt.Println("\nAll operations completed successfully! ✨")
}

func parseConfig() config.Config {
	debPaths := strings.Split(os.Getenv("INPUT_DEB-PATHS"), ",")
	architectures := strings.Split(os.Getenv("INPUT_ARCHITECTURES"), ",")

	if len(debPaths) != len(architectures) {
		panic("deb-paths and architectures must have the same number of elements")
	}

	privateKey, err := base64.StdEncoding.DecodeString(os.Getenv("INPUT_GPG-PRIVATE-KEY"))
	if err != nil {
		panic("deb-paths and architectures must have the same number of elements")
	}

	return config.Config{
		UbuntuDistro:    os.Getenv("INPUT_UBUNTU-DISTRO"),
		DebPaths:        debPaths,
		Architectures:   architectures,
		AccessKeyId:     os.Getenv("INPUT_ACCESS-KEY-ID"),
		AccessKeySecret: os.Getenv("INPUT_ACCESS-KEY-SECRET"),
		GpgPrivateKey:   privateKey,
	}
}

func uploadDebFile(bucket *oss.Bucket, cfg *config.SingleConfig, distro string) (*deb.DebFileInfo, error) {
	file, err := os.Open(cfg.DebPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %v", err)
	}
	defer file.Close()

	// Calculate checksums
	md5hash := md5.New()
	sha1hash := sha1.New()
	sha256hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(md5hash, sha1hash, sha256hash), file)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate checksums: %v", err)
	}

	// Reset file pointer
	if _, err := file.Seek(0, 0); err != nil {
		return nil, fmt.Errorf("failed to reset file pointer: %v", err)
	}

	debInfo, err := deb.GetInfoFromDebFile(file)
	if err != nil {
		return nil, fmt.Errorf("failed to get deb info: %v", err)
	}

	debInfo.Filename = fmt.Sprintf("coscene-apt-source/dists/%s/%s/binary-%s/%s",
		distro,
		cfg.Container,
		cfg.Architecture,
		filepath.Base(cfg.DebPath))

	debInfo.Size = size
	debInfo.MD5sum = hex.EncodeToString(md5hash.Sum(nil))
	debInfo.SHA1 = hex.EncodeToString(sha1hash.Sum(nil))
	debInfo.SHA256 = hex.EncodeToString(sha256hash.Sum(nil))

	// Upload file
	if !debug {
		err = bucket.PutObjectFromFile(debInfo.Filename, cfg.DebPath)
		if err != nil {
			return nil, fmt.Errorf("failed to upload to OSS: %v", err)
		}
	}

	return debInfo, nil
}

func updatePackages(bucket *oss.Bucket, cfg *config.SingleConfig, newDeb *deb.DebFileInfo, distro string) (string, error) {
	prefix := fmt.Sprintf("coscene-apt-source/dists/%s/%s/binary-%s/", distro, cfg.Container, cfg.Architecture)
	packagesPath := fmt.Sprintf("%sPackages", prefix)

	result, err := bucket.GetObject(packagesPath)
	if err != nil && !strings.Contains(err.Error(), "NoSuchKey") {
		return "", fmt.Errorf("failed to get packages: %v", err)
	}
	packages := make(map[string]*deb.DebFileInfo)
	if result != nil {
		packages = deb.ParsePackagesFile(result)
		defer result.Close()
	}

	packages[newDeb.Name] = newDeb

	localDir := fmt.Sprintf("dists/%s/%s/binary-%s", distro, cfg.Container, cfg.Architecture)
	if err := os.MkdirAll(localDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %v", err)
	}

	var content strings.Builder
	for _, pkg := range packages {
		content.WriteString(pkg.Format())
	}

	localPackagesPath := filepath.Join(localDir, "Packages")
	if err := os.WriteFile(localPackagesPath, []byte(content.String()), 0644); err != nil {
		return "", fmt.Errorf("failed to write local Packages file: %v", err)
	}

	if !debug {
		err = bucket.PutObject(packagesPath, strings.NewReader(content.String()))
		if err != nil {
			return "", err
		}
	}

	return content.String(), nil
}

func generatePackagesGz(bucket *oss.Bucket, content string, cfg *config.SingleConfig, distro string) error {
	localDir := fmt.Sprintf("dists/%s/%s/binary-%s", distro, cfg.Container, cfg.Architecture)
	localPackagesGzPath := filepath.Join(localDir, "Packages.gz")

	gzFile, err := os.Create(localPackagesGzPath)
	if err != nil {
		return fmt.Errorf("failed to create Packages.gz: %v", err)
	}
	defer gzFile.Close()

	gz := gzip.NewWriter(gzFile)
	if _, err := gz.Write([]byte(content)); err != nil {
		return fmt.Errorf("failed to write gzip content: %v", err)
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("failed to close gzip writer: %v", err)
	}

	if !debug {
		packagesGzPath := fmt.Sprintf("coscene-apt-source/dists/%s/%s/binary-%s/Packages.gz",
			distro,
			cfg.Container,
			cfg.Architecture)
		err = bucket.PutObjectFromFile(packagesGzPath, localPackagesGzPath)
	}
	return err
}

func updateRelease(bucket *oss.Bucket, configs []*config.SingleConfig, distro string) (string, error) {
	prefix := fmt.Sprintf("coscene-apt-source/dists/%s/", distro)
	releasePath := fmt.Sprintf("%sRelease", prefix)

	result, err := bucket.GetObject(releasePath)
	if err != nil && !strings.Contains(err.Error(), "NoSuchKey") {
		return "", fmt.Errorf("failed to get packages: %v", err)
	}
	releaseFile := &release.DistroRelease{
		Origin:      "coScene APT source",
		Label:       "coScene",
		Suite:       distro,
		Codename:    distro,
		Date:        "",
		Description: "CoScene APT Repository",
		MD5Sum:      make(map[string]*release.PackageInfo),
		SHA1:        make(map[string]*release.PackageInfo),
		SHA256:      make(map[string]*release.PackageInfo),
		SHA512:      make(map[string]*release.PackageInfo),
	}
	if result != nil {
		releaseFile = release.ParseReleaseFile(result)
		defer result.Close()
	}

	for _, cfg := range configs {
		pkgKey := fmt.Sprintf("%s/binary-%s/Packages", cfg.Container, cfg.Architecture)
		pkgPath := fmt.Sprintf("dists/%s/%s", distro, pkgKey)
		md5Str, sha1Str, sha256Str, sha512Str, length, err := calculateFileHashes(pkgPath)
		if err == nil {
			releaseFile.MD5Sum[pkgKey] = &release.PackageInfo{
				Sum:  md5Str,
				Size: length,
				Path: pkgKey,
			}
			releaseFile.SHA1[pkgKey] = &release.PackageInfo{
				Sum:  sha1Str,
				Size: length,
				Path: pkgKey,
			}
			releaseFile.SHA256[pkgKey] = &release.PackageInfo{
				Sum:  sha256Str,
				Size: length,
				Path: pkgKey,
			}
			releaseFile.SHA512[pkgKey] = &release.PackageInfo{
				Sum:  sha512Str,
				Size: length,
				Path: pkgKey,
			}
		}
		gzKey := fmt.Sprintf("%s/binary-%s/Packages.gz", cfg.Container, cfg.Architecture)
		gzPath := fmt.Sprintf("dists/%s/%s", distro, gzKey)
		md5Str, sha1Str, sha256Str, sha512Str, length, err = calculateFileHashes(gzPath)
		if err == nil {
			releaseFile.MD5Sum[gzKey] = &release.PackageInfo{
				Sum:  md5Str,
				Size: length,
				Path: gzKey,
			}
			releaseFile.SHA1[gzKey] = &release.PackageInfo{
				Sum:  sha1Str,
				Size: length,
				Path: gzKey,
			}
			releaseFile.SHA256[gzKey] = &release.PackageInfo{
				Sum:  sha256Str,
				Size: length,
				Path: gzKey,
			}
			releaseFile.SHA512[gzKey] = &release.PackageInfo{
				Sum:  sha512Str,
				Size: length,
				Path: gzKey,
			}
		}
	}

	currentTime := time.Now().UTC().Format("Mon, 02 Jan 2006 15:04:05 -0700")
	releaseFile.Date = currentTime
	releaseString := releaseFile.ToString()

	localReleasePath := fmt.Sprintf("dists/%s/Release", distro)
	if err := os.WriteFile(localReleasePath, []byte(releaseString), 0644); err != nil {
		return "", fmt.Errorf("failed to write Release file: %v", err)
	}

	if !debug {
		releasePath := fmt.Sprintf("coscene-apt-source/dists/%s/Release", distro)
		if err := bucket.PutObject(releasePath, strings.NewReader(releaseString)); err != nil {
			return "", err
		}
	}

	return releaseString, nil
}

func calculateFileHashes(filepath string) (md5sum, sha1sum, sha256sum, sha512sum string, size int, err error) {
	content, err := os.ReadFile(filepath)
	if err != nil {
		return "", "", "", "", 0, err
	}

	md5hash := md5.Sum(content)
	sha1hash := sha1.Sum(content)
	sha256hash := sha256.Sum256(content)
	sha512hash := sha512.Sum512(content)

	return hex.EncodeToString(md5hash[:]),
		hex.EncodeToString(sha1hash[:]),
		hex.EncodeToString(sha256hash[:]),
		hex.EncodeToString(sha512hash[:]),
		len(content),
		nil
}

func signReleaseFiles(bucket *oss.Bucket, releaseContent string, cfg config.Config, distro string) error {
	keyring, err := openpgp.ReadArmoredKeyRing(bytes.NewReader(cfg.GpgPrivateKey))
	if err != nil {
		return err
	}

	var gpgBuf bytes.Buffer
	w, err := armor.Encode(&gpgBuf, openpgp.SignatureType, nil)
	if err != nil {
		return err
	}

	err = openpgp.DetachSign(w, keyring[0], strings.NewReader(releaseContent), nil)
	if err != nil {
		return err
	}
	w.Close()

	releasePath := fmt.Sprintf("coscene-apt-source/dists/%s/Release.gpg", distro)
	if !debug {
		err = bucket.PutObject(releasePath, bytes.NewReader(gpgBuf.Bytes()))
		if err != nil {
			return err
		}
	}

	var inReleaseBuf bytes.Buffer
	w2, err := clearsign.Encode(&inReleaseBuf, keyring[0].PrivateKey, nil)
	if err != nil {
		return err
	}

	_, err = w2.Write([]byte(releaseContent))
	if err != nil {
		return err
	}

	err = w2.Close()
	if err != nil {
		return err
	}

	inReleasePath := fmt.Sprintf("coscene-apt-source/dists/%s/InRelease", distro)
	return bucket.PutObject(inReleasePath, bytes.NewReader(inReleaseBuf.Bytes()))
}
