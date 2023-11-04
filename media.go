package mex

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"errors"
	"fmt"
	"html/template"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

//go:embed regexp.txt
var volumeExpStrs string

func parseVolumeIndex(path string) *int {
	for _, expStr := range strings.Split(volumeExpStrs, "\n") {
		exp := regexp.MustCompile(expStr)
		if matches := exp.FindStringSubmatch(filepath.Base(path)); len(matches) >= 2 {
			if index, err := strconv.ParseInt(matches[1], 10, 32); err == nil {
				indexInt := int(index)
				return &indexInt
			}
		}
	}

	return nil
}

func isImagePath(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg", ".png", ".webp", ".bmp", ".gif":
		return true
	default:
		return false
	}
}

func buildTemplatedName(pattern, path string, index, count int) (string, error) {
	var (
		paddingCount = math.Log10(float64(count))
		paddingFmt   = fmt.Sprintf("%%0.%dd", int(paddingCount+1))
	)

	context := struct {
		Index string
		Name  string
		Ext   string
	}{
		Index: fmt.Sprintf(paddingFmt, index),
		Name:  filepath.Base(path),
		Ext:   strings.ToLower(filepath.Ext(path)),
	}

	tmpl, err := template.New("name").Parse(pattern)
	if err != nil {
		return "", err
	}

	var buff bytes.Buffer
	if err := tmpl.Execute(&buff, context); err != nil {
		return "", err
	}

	return buff.String(), nil
}

type ExportFlags int

const (
	ExportFlag_CompressBook = 1 << iota
	ExportFlag_CompressVolumes
)

type ExportConfig struct {
	Flags          ExportFlags
	PageTemplate   string
	VolumeTemplate string
	BookTemplate   string
	Workers        int
}

type Page struct {
	Node   *Node
	Volume *Volume
	Index  int
}

func (self *Page) export(dir string, config ExportConfig) error {
	name, err := buildTemplatedName(config.PageTemplate, self.Node.Name, self.Index+1, len(self.Volume.Pages))
	if err != nil {
		return err
	}

	if err := copyFile(filepath.Join(dir, name), self.Node.Path); err != nil {
		return err
	}

	return nil
}

type Volume struct {
	Node  *Node
	Book  *Book
	Pages []*Page
	Index int

	avgSize int
	hash    []byte
}

func (self *Volume) export(path string, config ExportConfig, allocator *TempDirAllocator) error {
	name, err := buildTemplatedName(config.VolumeTemplate, stripExt(self.Node.Name), self.Index, self.Book.VolumeCount)
	if err != nil {
		return err
	}

	var (
		compress  = config.Flags&ExportFlag_CompressVolumes != 0
		outputDir = path
	)

	if compress {
		if outputDir, err = allocator.TempDir(); err != nil {
			return err
		}
	} else {
		outputDir = filepath.Join(outputDir, name)
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return err
		}
	}

	for _, page := range self.Pages {
		if err := page.export(outputDir, config); err != nil {
			return err
		}
	}

	if compress {
		archivePath := filepath.Join(path, name)
		if err := Compress(archivePath, outputDir); err != nil {
			return err
		}
	}

	return nil
}

func (self *Volume) compare(other *Volume) int {
	if len(self.Pages) > len(other.Pages) {
		return 1
	} else if len(self.Pages) < len(other.Pages) {
		return -1
	}

	if self.avgSize > other.avgSize {
		return 1
	} else if self.avgSize < other.avgSize {
		return -1
	}

	return bytes.Compare(self.hash, other.hash)
}

type Book struct {
	Node        *Node
	Volumes     map[int]*Volume
	VolumeCount int

	orphans []*Volume
}

func (self *Book) Export(path string, config ExportConfig, allocator *TempDirAllocator) error {
	name, err := buildTemplatedName(config.BookTemplate, stripExt(self.Node.Name), 0, 0)
	if err != nil {
		return err
	}

	var (
		compress  = config.Flags&ExportFlag_CompressBook != 0
		outputDir = path
	)

	if compress {
		if outputDir, err = allocator.TempDir(); err != nil {
			return err
		}
	} else {
		outputDir = filepath.Join(outputDir, name)
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return err
		}
	}

	var (
		volumeChan    = make(chan *Volume, 4)
		volumeErr     error
		volumeErrLock sync.Mutex
		volumeWg      sync.WaitGroup
	)

	for i := 0; i < cap(volumeChan); i++ {
		volumeWg.Add(1)
		go func() {
			defer volumeWg.Done()
			for volume := range volumeChan {
				if err := volume.export(outputDir, config, allocator); err != nil {
					volumeErrLock.Lock()
					volumeErr = err
					volumeErrLock.Unlock()
					break
				}
			}
		}()
	}

	for _, volume := range self.Volumes {
		volumeChan <- volume
	}

	close(volumeChan)
	volumeWg.Wait()

	if volumeErr != nil {
		return volumeErr
	}

	if compress {
		archivePath := filepath.Join(path, name)
		if err := Compress(archivePath, outputDir); err != nil {
			return err
		}
	}

	return nil
}

func (self *Book) addVolume(newVolume *Volume) {
	insert := func(v *Volume) {
		self.Volumes[v.Index] = v
		if v.Index >= self.VolumeCount {
			self.VolumeCount = v.Index + 1
		}
	}

	currVolume, _ := self.Volumes[newVolume.Index]
	if currVolume == nil {
		insert(newVolume)
	} else {
		switch currVolume.compare(newVolume) {
		case 1:
			self.addOrphan(newVolume)
		case -1:
			self.addOrphan(currVolume)
			insert(newVolume)
		}
	}
}

func (self *Book) addOrphan(newVolume *Volume) {
	for _, volume := range self.orphans {
		if volume.compare(newVolume) == 0 {
			return
		}
	}

	self.orphans = append(self.orphans, newVolume)
}

func (self *Book) parseVolumes(node *Node) error {
	if !node.Info.IsDir() {
		return nil
	}

	volume := &Volume{
		Node: node,
		Book: self,
	}

	var pageIndex int
	for _, child := range node.Children {
		if child.Info.IsDir() {
			if err := self.parseVolumes(child); err != nil {
				return err
			}
		} else if isImagePath(child.Name) {
			volume.Pages = append(volume.Pages, &Page{child, volume, pageIndex})
			pageIndex++
		}
	}

	if len(volume.Pages) == 0 {
		return nil
	}

	sort.Slice(volume.Pages, func(i, j int) bool {
		return strings.Compare(volume.Pages[i].Node.Name, volume.Pages[j].Node.Name) < 0
	})

	var (
		hasher    = sha256.New()
		totalSize = 0
	)

	for _, page := range volume.Pages {
		fp, err := os.Open(page.Node.Path)
		if err != nil {
			return err
		}

		size, err := io.Copy(hasher, fp)
		fp.Close()

		if err != nil {
			return err
		}

		totalSize += int(size)
	}

	volume.avgSize = totalSize / len(volume.Pages)
	volume.hash = hasher.Sum(nil)

	if index := parseVolumeIndex(node.Name); index != nil {
		volume.Index = *index
		self.addVolume(volume)
	} else {
		self.addOrphan(volume)
	}

	return nil
}

func ParseBook(node *Node) (*Book, error) {
	book := Book{
		Node:    node,
		Volumes: make(map[int]*Volume),
	}

	book.parseVolumes(node)

	if len(book.orphans) > 0 {
		sort.Slice(book.orphans, func(i, j int) bool {
			return strings.Compare(book.orphans[i].Node.Name, book.orphans[j].Node.Name) < 0
		})

		for _, volume := range book.orphans {
			volume.Index = book.VolumeCount
			book.addVolume(volume)
		}

		book.orphans = nil
	}

	if len(book.Volumes) == 0 {
		return nil, errors.New("no volumes found")
	}

	return &book, nil
}
