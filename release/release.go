package release

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

type DistroRelease struct {
	Origin      string
	Label       string
	Suite       string
	Codename    string
	Date        string
	Description string
	MD5Sum      map[string]*PackageInfo
	SHA1        map[string]*PackageInfo
	SHA256      map[string]*PackageInfo
	SHA512      map[string]*PackageInfo
}

func (r *DistroRelease) ToString() string {
	var content strings.Builder
	fmt.Fprintf(&content, "Origin: %s\n", r.Origin)
	fmt.Fprintf(&content, "Label: %s\n", r.Label)
	fmt.Fprintf(&content, "Suite: %s\n", r.Suite)
	fmt.Fprintf(&content, "Codename: %s\n", r.Codename)
	fmt.Fprintf(&content, "Date: %s\n", r.Date)
	fmt.Fprintf(&content, "Description: %s\n", r.Description)

	fmt.Fprintf(&content, "MD5Sum:\n")
	for _, info := range r.MD5Sum {
		fmt.Fprintf(&content, " %s\n", info.ToString())
	}

	fmt.Fprintf(&content, "SHA1:\n")
	for _, info := range r.SHA1 {
		fmt.Fprintf(&content, " %s\n", info.ToString())
	}

	fmt.Fprintf(&content, "SHA256:\n")
	for _, info := range r.SHA256 {
		fmt.Fprintf(&content, " %s\n", info.ToString())
	}

	fmt.Fprintf(&content, "SHA512:\n")
	for _, info := range r.SHA512 {
		fmt.Fprintf(&content, " %s\n", info.ToString())
	}

	return content.String()
}

func ParseReleaseFile(reader io.Reader) *DistroRelease {
	release := &DistroRelease{
		Origin:      "coScene APT source",
		Label:       "coScene",
		Suite:       "",
		Codename:    "",
		Date:        "",
		Description: "CoScene APT Repository",
		MD5Sum:      make(map[string]*PackageInfo),
		SHA1:        make(map[string]*PackageInfo),
		SHA256:      make(map[string]*PackageInfo),
		SHA512:      make(map[string]*PackageInfo),
	}

	scanner := bufio.NewScanner(reader)
	var currentSection string

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		// 判断是否是新的校验和部分
		if strings.Contains(line, "MD5Sum:") {
			currentSection = "MD5Sum"
			continue
		} else if strings.Contains(line, "SHA1:") {
			currentSection = "SHA1"
			continue
		} else if strings.Contains(line, "SHA256:") {
			currentSection = "SHA256"
			continue
		} else if strings.Contains(line, "SHA512:") {
			currentSection = "SHA512"
			continue
		}

		// 处理校验和部分
		if currentSection != "" && strings.HasPrefix(line, " ") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				sum := parts[0]
				size, _ := strconv.ParseInt(parts[1], 10, 64)
				path := parts[2]

				info := PackageInfo{
					Sum:  sum,
					Size: int(size),
					Path: path,
				}

				switch currentSection {
				case "MD5Sum":
					release.MD5Sum[path] = &info
				case "SHA1":
					release.SHA1[path] = &info
				case "SHA256":
					release.SHA256[path] = &info
				case "SHA512":
					release.SHA512[path] = &info
				}
			}
			continue
		}

		// 处理元数据部分
		parts := strings.SplitN(line, ": ", 2)
		if len(parts) != 2 {
			continue
		}

		switch parts[0] {
		//case "Origin":
		//	release.Origin = parts[1]
		//case "Label":
		//	release.Label = parts[1]
		case "Suite":
			release.Suite = parts[1]
		case "Codename":
			release.Codename = parts[1]
		case "Date":
			release.Date = parts[1]
		case "Description":
			release.Description = parts[1]
		}
	}

	return release
}
