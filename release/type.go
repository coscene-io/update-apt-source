package release

import "fmt"

type PackageInfo struct {
	Sum  string
	Size int
	Path string
}

func (p *PackageInfo) ToString() string {
	return fmt.Sprintf(" %s %16d %s", p.Sum, p.Size, p.Path)
}
