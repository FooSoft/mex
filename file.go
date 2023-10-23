package mex

import (
	"io"
	"os"
	"path/filepath"
	"sync"
)

func copyFile(targetPath, sourcePath string) error {
	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	targetFile, err := os.Create(targetPath)
	if err != nil {
		return err
	}
	defer targetFile.Close()

	if _, err := io.Copy(targetFile, sourceFile); err != nil {
		return err
	}

	return nil
}

func stripExt(path string) string {
	name := filepath.Base(path)
	return name[:len(name)-len(filepath.Ext(name))]
}

type TempDirAllocator struct {
	mutex sync.Mutex
	dirs  []string
}

func (self *TempDirAllocator) TempDir() (string, error) {
	dir, err := os.MkdirTemp("", "mex_")
	if err != nil {
		return "", err
	}

	self.mutex.Lock()
	defer self.mutex.Unlock()
	self.dirs = append(self.dirs, dir)

	return dir, nil
}

func (self *TempDirAllocator) Cleanup() {
	for _, dir := range self.dirs {
		os.RemoveAll(dir)
	}

	self.mutex.Lock()
	defer self.mutex.Unlock()
	self.dirs = nil
}

type Node struct {
	Name     string
	Path     string
	Info     os.FileInfo
	Children []*Node
}

func Walk(path string, allocator *TempDirAllocator) (*Node, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	currNode := &Node{
		Name: filepath.Base(path),
		Path: path,
		Info: info,
	}

	if info.IsDir() {
		entries, err := os.ReadDir(path)
		if err != nil {
			return nil, err
		}

		for _, entry := range entries {
			child, err := Walk(filepath.Join(path, entry.Name()), allocator)
			if err != nil {
				return nil, err
			}

			currNode.Children = append(currNode.Children, child)
		}
	} else {
		if contentDir, err := Decompress(path, allocator); err == nil {
			archName := currNode.Name
			if currNode, err = Walk(contentDir, allocator); err != nil {
				return nil, err
			}

			currNode.Name = archName
		} else if err != UnsupportedArchiveFormatError {
			return nil, err
		}
	}

	return currNode, nil
}
