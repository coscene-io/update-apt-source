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
	"github.com/coscene-io/update-apt-source/locker"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/coscene-io/update-apt-source/config"
	"github.com/coscene-io/update-apt-source/deb"
	"github.com/coscene-io/update-apt-source/release"
	"github.com/coscene-io/update-apt-source/storage"
	"golang.org/x/crypto/openpgp"
	"golang.org/x/crypto/openpgp/armor"
	"golang.org/x/crypto/openpgp/clearsign"
)

var supportedUbuntuDistros = []string{
	"bionic",
	"focal",
	"jammy",
	"noble",
}

func main() {
	cfg := parseConfig()
	if !cfg.IsValid() {
		panic("Invalid config!")
	}

	fmt.Printf("Initialize storage client... ")
	storageProvider, err := storage.NewStorageProvider(
		cfg.StorageType,
		cfg.Endpoint,
		cfg.Region,
		cfg.AccessKeyId,
		cfg.AccessKeySecret,
	)
	if err != nil {
		panic(fmt.Sprintf("Initialize storage client failed: %v", err))
	}
	fmt.Printf(" ✓\n")
	fmt.Printf("  Accessing bucket... ✓\n")

	// ClearBucket(storageProvider, cfg.BucketName, "", "")

	l := locker.NewLocker(storageProvider, cfg.BucketName)
	err = l.Lock()
	if err != nil {
		panic(fmt.Sprintf("Lock bucket failed: %v", err))
	}
	defer func() {
		if err := l.Unlock(); err != nil {
			fmt.Printf("Warning: Unlock failed: %v\n", err)
		}
	}()

	configList := make([]*config.SingleConfig, len(cfg.DebPaths))
	for i := range cfg.DebPaths {
		configList[i] = &config.SingleConfig{
			UbuntuDistro: cfg.UbuntuDistro,
			DebPath:      cfg.DebPaths[i],
			Architecture: cfg.Architectures[i],
		}
	}
	for i, c := range configList {
		if c.UbuntuDistro != "all" {
			c.Container = "main"
			fmt.Printf("\nUbuntu Distro: %s\n", c.UbuntuDistro)
			fmt.Printf("  [%d/%d] Processing package (%s, %s):\n",
				i+1, len(configList), c.Architecture, c.DebPath)

			fmt.Printf("    Upload deb package...  ")
			debInfo, err := uploadDebFile(storageProvider, cfg.BucketName, c)
			if err != nil {
				panic(fmt.Sprintf("**Upload deb package failed: %v**", err))
			}

			fmt.Printf("    Update Packages file...  ")
			packagesContent, err := updatePackages(storageProvider, cfg.BucketName, c, debInfo)
			if err != nil {
				panic(fmt.Sprintf("**Update Packages failed: %v**", err))
			}
			fmt.Printf("✓\n")

			fmt.Printf("    Generate and upload Packages.gz... ")
			err = generatePackagesGz(storageProvider, cfg.BucketName, packagesContent, c)
			if err != nil {
				panic(fmt.Sprintf("**Generate Packages.gz failed: %v**", err))
			}
			fmt.Printf("✓\n")

			fmt.Printf("\n    Update Release file... ")
			releaseContent, err := updateRelease(storageProvider, cfg.BucketName, c, c.UbuntuDistro)
			if err != nil {
				panic(fmt.Sprintf("**Update Release failed: %v**", err))
			}
			fmt.Printf("✓\n")

			fmt.Printf("\n    Generate signed files... ")
			err = signReleaseFiles(storageProvider, cfg.BucketName, releaseContent, &cfg.GpgPrivateKey, c.UbuntuDistro)
			if err != nil {
				panic(fmt.Sprintf("Sign files failed: %v", err))
			}
			fmt.Printf("✓\n")

		} else {
			c.Container = "stable"
			fmt.Printf("\nUbuntu Distro: %s\n", c.UbuntuDistro)
			fmt.Printf("  [%d/%d] Processing package (%s, %s):\n",
				i+1, len(configList), c.Architecture, c.DebPath)

			fmt.Printf("    Upload deb package...  ")
			debInfo, err := uploadDebFile(storageProvider, cfg.BucketName, c)
			if err != nil {
				panic(fmt.Sprintf("**Upload deb package failed: %v**", err))
			}

			sourceFile := debInfo.Filename
			for _, d := range supportedUbuntuDistros {
				c.UbuntuDistro = d
				linkName := fmt.Sprintf("dists/%s/%s/binary-%s/%s", d, c.Container, c.Architecture, filepath.Base(c.DebPath))
				fmt.Printf("    Create deb file redirect: %s -> %s\n", linkName, sourceFile)

				err = storageProvider.CreateSymlink(cfg.BucketName, sourceFile, linkName)
				if err != nil {
					fmt.Printf("    Warning: Create redirect failed: %v\n", err)
				}

				debInfo.Filename = linkName

				fmt.Printf("    Update Packages file...  ")
				packagesContent, err := updatePackages(storageProvider, cfg.BucketName, c, debInfo)
				if err != nil {
					panic(fmt.Sprintf("**Update Packages failed: %v**", err))
				}
				fmt.Printf("✓\n")

				fmt.Printf("    Generate and upload Packages.gz... ")
				err = generatePackagesGz(storageProvider, cfg.BucketName, packagesContent, c)
				if err != nil {
					panic(fmt.Sprintf("**Generate Packages.gz failed: %v**", err))
				}
				fmt.Printf("✓\n")

				fmt.Printf("    Update Release file... ")
				releaseContent, err := updateRelease(storageProvider, cfg.BucketName, c, d)
				if err != nil {
					panic(fmt.Sprintf("**Update Release failed: %v**", err))
				}
				fmt.Printf("✓\n")

				fmt.Printf("    Generate signed files... ")
				err = signReleaseFiles(storageProvider, cfg.BucketName, releaseContent, &cfg.GpgPrivateKey, d)
				if err != nil {
					panic(fmt.Sprintf("Sign files failed: %v", err))
				}
				fmt.Printf("✓\n\n")
			}
		}
	}

	fmt.Println("\nAll operations completed successfully! 🎉")
}

func parseConfig() config.Config {
	debPathsStr := os.Getenv("INPUT_DEB_PATHS")
	architecturesStr := os.Getenv("INPUT_ARCHITECTURES")
	distroStr := os.Getenv("INPUT_UBUNTU_DISTRO")
	endpointStr := os.Getenv("INPUT_ENDPOINT")
	bucketStr := os.Getenv("INPUT_BUCKET_NAME")
	regionStr := os.Getenv("INPUT_REGION")
	storageTypeStr := os.Getenv("INPUT_STORAGE_TYPE")
	githubOutput := os.Getenv("GITHUB_OUTPUT")

	fmt.Println("🌍Environment variables:")
	fmt.Println("    INPUT_DEB_PATHS:", debPathsStr)
	fmt.Println("    INPUT_ARCHITECTURES:", architecturesStr)
	fmt.Println("    INPUT_UBUNTU_DISTRO:", distroStr)
	fmt.Println("    INPUT_ENDPOINT:", endpointStr)
	fmt.Println("    INPUT_BUCKET_NAME:", bucketStr)
	fmt.Println("    INPUT_REGION:", regionStr)
	fmt.Println("    INPUT_STORAGE_TYPE:", storageTypeStr)
	fmt.Println("    GITHUB_OUTPUT:", githubOutput)
	fmt.Println("")

	var debPaths, architectures []string

	debPaths = parseMultilineOrCommaInput(debPathsStr)

	architectures = parseMultilineOrCommaInput(architecturesStr)

	if len(debPaths) != len(architectures) {
		panic("deb_paths and architectures must have the same number of elements")
	}

	privateKey, err := base64.StdEncoding.DecodeString(os.Getenv("INPUT_GPG_PRIVATE_KEY"))
	if err != nil {
		panic("Failed to decode GPG private key: " + err.Error())
	}

	return config.Config{
		UbuntuDistro:    distroStr,
		DebPaths:        debPaths,
		Architectures:   architectures,
		StorageType:     storageTypeStr,
		Endpoint:        endpointStr,
		Region:          regionStr,
		BucketName:      bucketStr,
		AccessKeyId:     os.Getenv("INPUT_ACCESS_KEY_ID"),
		AccessKeySecret: os.Getenv("INPUT_ACCESS_KEY_SECRET"),
		GpgPrivateKey:   privateKey,
	}
}

func parseMultilineOrCommaInput(input string) []string {
	lines := strings.Split(input, "\n")

	if len(lines) == 1 {
		parts := strings.Split(input, ",")
		if len(parts) > 1 {
			lines = parts
		}
	}

	var result []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			result = append(result, line)
		}
	}

	return result
}

func uploadDebFile(storageProvider storage.StorageProvider, bucketName string, cfg *config.SingleConfig) (*deb.DebFileInfo, error) {
	file, err := os.Open(cfg.DebPath)
	if err != nil {
		return nil, fmt.Errorf("open file failed: %v", err)
	}
	defer file.Close()

	md5hash := md5.New()
	sha1hash := sha1.New()
	sha256hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(md5hash, sha1hash, sha256hash), file)
	if err != nil {
		return nil, fmt.Errorf("calculate checksum failed: %v", err)
	}

	if _, err := file.Seek(0, 0); err != nil {
		return nil, fmt.Errorf("reset file pointer failed: %v", err)
	}

	debInfo, err := deb.GetInfoFromDebFile(file)
	if err != nil {
		return nil, fmt.Errorf("get deb info failed: %v", err)
	}

	baseFilename := filepath.Base(cfg.DebPath)
	debInfo.Filename = fmt.Sprintf("dists/%s/%s/binary-%s/%s",
		cfg.UbuntuDistro,
		cfg.Container,
		cfg.Architecture,
		baseFilename)

	debInfo.Size = size
	debInfo.MD5sum = hex.EncodeToString(md5hash.Sum(nil))
	debInfo.SHA1 = hex.EncodeToString(sha1hash.Sum(nil))
	debInfo.SHA256 = hex.EncodeToString(sha256hash.Sum(nil))

	fileContent, err := ioutil.ReadFile(cfg.DebPath)
	if err != nil {
		return nil, fmt.Errorf("read file failed: %v", err)
	}

	err = storageProvider.PutObject(bucketName, debInfo.Filename, fileContent)
	if err != nil {
		return nil, fmt.Errorf("upload to cloud storage failed: %v", err)
	}

	fmt.Printf("✓\n")
	parts := strings.Split(baseFilename, "_")
	if len(parts) >= 3 {
		packageName := parts[0]
		architecture := parts[len(parts)-1]

		latestFilename := fmt.Sprintf("%s_latest_%s", packageName, architecture)
		latestS3Path := fmt.Sprintf("dists/%s/%s/binary-%s/%s",
			cfg.UbuntuDistro,
			cfg.Container,
			cfg.Architecture,
			latestFilename)

		fmt.Printf("    Create redirect %s -> %s ...  ", latestFilename, baseFilename)
		err = storageProvider.CreateSymlink(bucketName, debInfo.Filename, latestS3Path)
		if err != nil {
			fmt.Printf("    Warning: Create redirect failed: %v\n", err)
		}
		fmt.Printf("✓\n")
	} else {
		fmt.Printf("    Warning: File name format does not match link creation: %s\n", baseFilename)
	}

	return debInfo, nil
}

func updatePackages(storageProvider storage.StorageProvider, bucketName string, cfg *config.SingleConfig, newDeb *deb.DebFileInfo) (string, error) {
	prefix := fmt.Sprintf("dists/%s/%s/binary-%s", cfg.UbuntuDistro, cfg.Container, cfg.Architecture)
	packagesPath := fmt.Sprintf("%s/Packages", prefix)

	packages := make(map[string]*deb.DebFileInfo)

	exists, err := storageProvider.HeadObject(bucketName, packagesPath)
	if err != nil {
		return "", fmt.Errorf("get Packages file failed: %v", err)
	}
	if exists {
		packagesContent, err := storageProvider.GetObject(bucketName, packagesPath)
		if err != nil {
			return "", fmt.Errorf("get Packages file failed: %v", err)
		}
		packages = deb.ParsePackagesFile(bytes.NewReader(packagesContent))
	}

	packages[newDeb.Name] = newDeb

	if err := os.MkdirAll(prefix, 0755); err != nil {
		return "", fmt.Errorf("create directory failed: %v", err)
	}

	var content strings.Builder
	for _, pkg := range packages {
		content.WriteString(pkg.Format())
	}

	contentStr := content.String()

	if err := os.WriteFile(packagesPath, []byte(contentStr), 0644); err != nil {
		return "", fmt.Errorf("write local Packages file failed: %v", err)
	}

	err = storageProvider.PutObject(bucketName, packagesPath, []byte(contentStr))
	if err != nil {
		return "", fmt.Errorf("upload Packages file failed: %v", err)
	}

	return contentStr, nil
}

func generatePackagesGz(storageProvider storage.StorageProvider, bucketName string, content string, cfg *config.SingleConfig) error {
	localDir := fmt.Sprintf("dists/%s/%s/binary-%s", cfg.UbuntuDistro, cfg.Container, cfg.Architecture)
	localPackagesGzPath := filepath.Join(localDir, "Packages.gz")

	gzFile, err := os.Create(localPackagesGzPath)
	if err != nil {
		return fmt.Errorf("create Packages.gz file failed: %v", err)
	}
	defer gzFile.Close()

	gz := gzip.NewWriter(gzFile)
	if _, err := gz.Write([]byte(content)); err != nil {
		return fmt.Errorf("write gzip content failed: %v", err)
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("close gzip writer failed: %v", err)
	}

	gzContent, err := ioutil.ReadFile(localPackagesGzPath)
	if err != nil {
		return fmt.Errorf("read Packages.gz file failed: %v", err)
	}

	packagesGzPath := fmt.Sprintf("dists/%s/%s/binary-%s/Packages.gz",
		cfg.UbuntuDistro,
		cfg.Container,
		cfg.Architecture)

	err = storageProvider.PutObject(bucketName, packagesGzPath, gzContent)
	if err != nil {
		return fmt.Errorf("upload Packages.gz file failed: %v", err)
	}

	return nil
}

func updateRelease(storageProvider storage.StorageProvider, bucketName string, cfg *config.SingleConfig, distro string) (string, error) {
	prefix := fmt.Sprintf("dists/%s/", distro)
	releasePath := fmt.Sprintf("%sRelease", prefix)

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

	exists, err := storageProvider.HeadObject(bucketName, releasePath)
	if err != nil {
		return "", fmt.Errorf("get Release file failed: %v", err)
	}
	if exists {
		releaseContent, err := storageProvider.GetObject(bucketName, releasePath)
		if err != nil {
			return "", fmt.Errorf("get Release file failed: %v", err)
		}
		releaseFile = release.ParseReleaseFile(bytes.NewReader(releaseContent))
	}

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

	currentTime := time.Now().UTC().Format("Mon, 02 Jan 2006 15:04:05 -0700")
	releaseFile.Date = currentTime

	releaseString := releaseFile.ToString()

	localReleasePath := fmt.Sprintf("dists/%s/Release", distro)
	if err := os.MkdirAll(fmt.Sprintf("dists/%s", distro), 0755); err != nil {
		return "", fmt.Errorf("create directory failed: %v", err)
	}

	if err := os.WriteFile(localReleasePath, []byte(releaseString), 0644); err != nil {
		return "", fmt.Errorf("write local Release file failed: %v", err)
	}

	err = storageProvider.PutObject(bucketName, releasePath, []byte(releaseString))
	if err != nil {
		return "", fmt.Errorf("upload Release file failed: %v", err)
	}

	return releaseString, nil
}

func calculateFileHashes(filepath string) (md5sum, sha1sum, sha256sum, sha512sum string, size int, err error) {
	content, err := ioutil.ReadFile(filepath)
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

func signReleaseFiles(storageProvider storage.StorageProvider, bucketName string, releaseContent string, privateKey *[]byte, distro string) error {
	keyring, err := openpgp.ReadArmoredKeyRing(bytes.NewReader(*privateKey))
	if err != nil {
		return fmt.Errorf("read GPG key failed: %v", err)
	}

	var gpgBuf bytes.Buffer
	w, err := armor.Encode(&gpgBuf, openpgp.SignatureType, nil)
	if err != nil {
		return fmt.Errorf("create signature encoder failed: %v", err)
	}

	err = openpgp.DetachSign(w, keyring[0], strings.NewReader(releaseContent), nil)
	if err != nil {
		return fmt.Errorf("generate detached signature failed: %v", err)
	}
	w.Close()

	releasePath := fmt.Sprintf("dists/%s/Release.gpg", distro)

	err = storageProvider.PutObject(bucketName, releasePath, gpgBuf.Bytes())
	if err != nil {
		return fmt.Errorf("upload Release.gpg file failed: %v", err)
	}

	var inReleaseBuf bytes.Buffer
	w2, err := clearsign.Encode(&inReleaseBuf, keyring[0].PrivateKey, nil)
	if err != nil {
		return fmt.Errorf("create plaintext signature encoder failed: %v", err)
	}

	_, err = w2.Write([]byte(releaseContent))
	if err != nil {
		return fmt.Errorf("write plaintext signature content failed: %v", err)
	}

	err = w2.Close()
	if err != nil {
		return fmt.Errorf("close plaintext signature encoder failed: %v", err)
	}

	inReleasePath := fmt.Sprintf("dists/%s/InRelease", distro)

	err = storageProvider.PutObject(bucketName, inReleasePath, inReleaseBuf.Bytes())
	if err != nil {
		return fmt.Errorf("upload InRelease file failed: %v", err)
	}
	return nil
}

func PrintDirectoryTree(root string, indent string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return fmt.Errorf("read directory %s failed: %v", root, err)
	}

	for i, entry := range entries {
		isLast := i == len(entries)-1

		connector := "├── "
		if isLast {
			connector = "└── "
		}

		info, err := entry.Info()
		size := ""
		if err == nil {
			size = fmt.Sprintf("(%d bytes)", info.Size())
		}

		fmt.Printf("%s%s%s %s\n", indent, connector, entry.Name(), size)

		if entry.IsDir() {
			newIndent := indent + "│   "
			if isLast {
				newIndent = indent + "    "
			}

			subdir := filepath.Join(root, entry.Name())
			err := PrintDirectoryTree(subdir, newIndent)
			if err != nil {
				fmt.Printf("%s    [Error: %v]\n", newIndent, err)
			}
		}
	}

	return nil
}
