package deb

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
)

type DebFileInfo struct {
	Name          string
	Version       string
	Architecture  string
	Maintainer    string
	InstalledSize string
	Depends       string
	Filename      string
	Size          int64
	MD5sum        string
	SHA1          string
	SHA256        string
	Section       string
	Priority      string
	Description   string
}

func (p *DebFileInfo) Format() string {
	var content strings.Builder

	fmt.Fprintf(&content, "Package: %s\n", p.Name)
	fmt.Fprintf(&content, "Version: %s\n", p.Version)
	fmt.Fprintf(&content, "Architecture: %s\n", p.Architecture)
	fmt.Fprintf(&content, "Maintainer: %s\n", p.Maintainer)
	if p.InstalledSize != "" {
		fmt.Fprintf(&content, "Installed-Size: %s\n", p.InstalledSize)
	}
	if p.Depends != "" {
		fmt.Fprintf(&content, "Depends: %s\n", p.Depends)
	}
	fmt.Fprintf(&content, "Filename: %s\n", p.Filename)
	fmt.Fprintf(&content, "Size: %d\n", p.Size)
	fmt.Fprintf(&content, "MD5sum: %s\n", p.MD5sum)
	fmt.Fprintf(&content, "SHA1: %s\n", p.SHA1)
	fmt.Fprintf(&content, "SHA256: %s\n", p.SHA256)
	if p.Section != "" {
		fmt.Fprintf(&content, "Section: %s\n", p.Section)
	}
	if p.Priority != "" {
		fmt.Fprintf(&content, "Priority: %s\n", p.Priority)
	}
	if p.Description != "" {
		fmt.Fprintf(&content, "Description: %s\n", p.Description)
	}
	fmt.Fprintf(&content, "\n")

	return content.String()
}

func GetInfoFromDebFile(file *os.File) (*DebFileInfo, error) {
	arHeader := make([]byte, 8)
	if _, err := io.ReadFull(file, arHeader); err != nil {
		return nil, fmt.Errorf("failed to read ar header: %v", err)
	}
	if string(arHeader) != "!<arch>\n" {
		return nil, fmt.Errorf("invalid ar file format")
	}

	var controlData []byte
	var compressionType string
	for {
		header := make([]byte, 60)
		if _, err := io.ReadFull(file, header); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("failed to read ar file header: %v", err)
		}

		filename := strings.TrimRight(string(header[0:16]), " ")
		sizeStr := strings.TrimSpace(string(header[48:58]))
		fileSize, err := strconv.ParseInt(sizeStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse file size: %v", err)
		}

		if filename == "control.tar.gz" || filename == "control.tar.xz" || filename == "control.tar.zst" {
			controlData = make([]byte, fileSize)
			if _, err := io.ReadFull(file, controlData); err != nil {
				return nil, fmt.Errorf("failed to read %s: %v", filename, err)
			}

			if filename == "control.tar.xz" {
				compressionType = "xz"
			} else if filename == "control.tar.zst" {
				compressionType = "zst"
			} else {
				compressionType = "gz"
			}
			break
		}

		if _, err := file.Seek(fileSize+(fileSize%2), 1); err != nil {
			return nil, fmt.Errorf("failed to skip file content: %v", err)
		}
	}

	if controlData == nil {
		return nil, fmt.Errorf("control.tar.gz/xz/zst not found in deb package")
	}

	var reader io.Reader
	var closeReader func() error = nil

	switch compressionType {
	case "xz":
		xzReader, err := xz.NewReader(bytes.NewReader(controlData))
		if err != nil {
			return nil, fmt.Errorf("failed to create xz reader: %v", err)
		}
		reader = xzReader
	case "zst":
		zstReader, err := zstd.NewReader(bytes.NewReader(controlData))
		if err != nil {
			return nil, fmt.Errorf("failed to create zstd reader: %v", err)
		}
		reader = zstReader
		closeReader = func() error {
			zstReader.Close()
			return nil
		}
	case "gz":
		gzReader, err := gzip.NewReader(bytes.NewReader(controlData))
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %v", err)
		}
		reader = gzReader
		closeReader = gzReader.Close
	}

	if closeReader != nil {
		defer closeReader()
	}

	tarReader := tar.NewReader(reader)

	var controlContent []byte
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read tar header: %v", err)
		}

		if header.Name == "./control" || header.Name == "control" {
			controlContent, err = io.ReadAll(tarReader)
			if err != nil {
				return nil, fmt.Errorf("failed to read control file: %v", err)
			}
			break
		}
	}

	if controlContent == nil {
		return nil, fmt.Errorf("control file not found in deb package")
	}

	debInfo := &DebFileInfo{}
	scanner := bufio.NewScanner(bytes.NewReader(controlContent))
	var lastField string
	var multiLineDescription strings.Builder

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, " ") && lastField == "Description" {
			if multiLineDescription.Len() > 0 {
				multiLineDescription.WriteString("\n")
			}
			multiLineDescription.WriteString(strings.TrimPrefix(line, " "))
			continue
		}

		if multiLineDescription.Len() > 0 && lastField == "Description" {
			if debInfo.Description != "" {
				debInfo.Description = debInfo.Description + "\n" + multiLineDescription.String()
			} else {
				debInfo.Description = multiLineDescription.String()
			}
			multiLineDescription.Reset()
		}

		if line == "" {
			lastField = ""
			continue
		}

		parts := strings.SplitN(line, ": ", 2)
		if len(parts) != 2 {
			continue
		}

		lastField = parts[0]

		switch parts[0] {
		case "Package":
			debInfo.Name = parts[1]
		case "Version":
			debInfo.Version = parts[1]
		case "Architecture":
			debInfo.Architecture = parts[1]
		case "Maintainer":
			debInfo.Maintainer = parts[1]
		case "Installed-Size":
			debInfo.InstalledSize = parts[1]
		case "Depends":
			debInfo.Depends = parts[1]
		case "Section":
			debInfo.Section = parts[1]
		case "Priority":
			debInfo.Priority = parts[1]
		case "Description":
			debInfo.Description = parts[1]
			multiLineDescription.Reset()
		}
	}

	if multiLineDescription.Len() > 0 && lastField == "Description" {
		if debInfo.Description != "" {
			debInfo.Description = debInfo.Description + "\n" + multiLineDescription.String()
		} else {
			debInfo.Description = multiLineDescription.String()
		}
	}

	return debInfo, nil
}

func ParsePackagesFile(reader io.Reader) map[string]*DebFileInfo {
	packagesMap := map[string]*DebFileInfo{}
	scanner := bufio.NewScanner(reader)
	var currentPackage DebFileInfo
	var lastField string
	var multiLineDescription strings.Builder

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, " ") && lastField == "Description" {
			if multiLineDescription.Len() > 0 {
				multiLineDescription.WriteString("\n")
			}
			multiLineDescription.WriteString(strings.TrimPrefix(line, " "))
			continue
		}

		if multiLineDescription.Len() > 0 && lastField == "Description" {
			if currentPackage.Description != "" {
				currentPackage.Description = currentPackage.Description + "\n" + multiLineDescription.String()
			} else {
				currentPackage.Description = multiLineDescription.String()
			}
			multiLineDescription.Reset()
		}

		if line == "" {
			lastField = ""
			if currentPackage.Name != "" {
				pkgCopy := currentPackage
				packagesMap[currentPackage.Name] = &pkgCopy
			}
			currentPackage = DebFileInfo{}
			continue
		}

		parts := strings.SplitN(line, ": ", 2)
		if len(parts) != 2 {
			continue
		}

		lastField = parts[0]

		switch parts[0] {
		case "Package":
			currentPackage.Name = parts[1]
		case "Version":
			currentPackage.Version = parts[1]
		case "Architecture":
			currentPackage.Architecture = parts[1]
		case "Maintainer":
			currentPackage.Maintainer = parts[1]
		case "Installed-Size":
			currentPackage.InstalledSize = parts[1]
		case "Depends":
			currentPackage.Depends = parts[1]
		case "Filename":
			currentPackage.Filename = parts[1]
		case "Size":
			size, _ := strconv.ParseInt(parts[1], 10, 64)
			currentPackage.Size = size
		case "MD5sum":
			currentPackage.MD5sum = parts[1]
		case "SHA1":
			currentPackage.SHA1 = parts[1]
		case "SHA256":
			currentPackage.SHA256 = parts[1]
		case "Section":
			currentPackage.Section = parts[1]
		case "Priority":
			currentPackage.Priority = parts[1]
		case "Description":
			currentPackage.Description = parts[1]
			multiLineDescription.Reset()
		}
	}

	if multiLineDescription.Len() > 0 && lastField == "Description" {
		if currentPackage.Description != "" {
			currentPackage.Description = currentPackage.Description + "\n" + multiLineDescription.String()
		} else {
			currentPackage.Description = multiLineDescription.String()
		}
	}

	if currentPackage.Name != "" {
		pkgCopy := currentPackage
		packagesMap[currentPackage.Name] = &pkgCopy
	}

	return packagesMap
}
