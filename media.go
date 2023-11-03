package mex

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"log"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

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
	name, err := buildTemplatedName(config.PageTemplate, self.Node.Name, self.Index+1, len(self.Volume.Pages)-1)
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
}

func (self *Volume) AveragePageSize() int {
	if len(self.Pages) == 0 {
		return 0
	}

	var totalSize int
	for _, page := range self.Pages {
		totalSize += int(page.Node.Info.Size())
	}

	return totalSize / len(self.Pages)
}

func (self *Volume) export(path string, config ExportConfig, allocator *TempDirAllocator) error {
	name, err := buildTemplatedName(config.VolumeTemplate, stripExt(self.Node.Name), self.Index, self.Book.VolumeCount-1)
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

func (self *Volume) supercedes(other *Volume) bool {
	if len(self.Pages) > len(other.Pages) {
		log.Printf("picking %s over %s because it has more pages", self.Node.Name, other.Node.Name)
		return true
	}

	if self.AveragePageSize() > other.AveragePageSize() {
		log.Printf("picking %s over %s because it has larger pages", self.Node.Name, other.Node.Name)
		return true
	}

	return false
}

type Book struct {
	Node        *Node
	Volumes     map[int]*Volume
	VolumeCount int
	orphans     []*Volume
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

func (self *Book) addVolume(volume *Volume) {
	insert := func(v *Volume) {
		self.Volumes[v.Index] = v
		if v.Index >= self.VolumeCount {
			self.VolumeCount = v.Index + 1
		}
	}

	currVolume, _ := self.Volumes[volume.Index]
	if currVolume == nil {
		insert(volume)
	} else if volume.supercedes(currVolume) {
		self.addOrphan(currVolume)
		insert(volume)
	} else {
		self.addOrphan(volume)
	}
}

func (self *Book) addOrphan(volume *Volume) {
	self.orphans = append(self.orphans, volume)
}

func (self *Book) parseVolumes(node *Node) {
	if !node.Info.IsDir() {
		return
	}

	volume := &Volume{
		Node: node,
		Book: self,
	}

	var pageIndex int
	for _, child := range node.Children {
		if child.Info.IsDir() {
			self.parseVolumes(child)
		} else if isImagePath(child.Name) {
			volume.Pages = append(volume.Pages, &Page{child, volume, pageIndex})
			pageIndex++
		}
	}

	if len(volume.Pages) > 0 {
		exp := regexp.MustCompile(`(\d+)\D*$`)
		if matches := exp.FindStringSubmatch(node.Name); len(matches) >= 2 {
			index, err := strconv.ParseInt(matches[1], 10, 32)
			if err != nil {
				panic(err)
			}

			volume.Index = int(index)
			self.addVolume(volume)
		} else {
			self.addOrphan(volume)
		}
	}
}

func ParseBook(node *Node) (*Book, error) {
	book := Book{
		Node:    node,
		Volumes: make(map[int]*Volume),
	}

	book.parseVolumes(node)

	if len(book.orphans) > 0 {
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
