package mex

import (
	"errors"
	"fmt"
	"log"
	"os/exec"
	"path/filepath"
	"strings"
)

var (
	UnsupportedArchiveFormatError = errors.New("unsupported archive format")
	RequiredToolNotInstalled      = errors.New("required tool not installed")
)

var (
	archExt = ".cbz"

	rarToolNames = []string{"unrar"}
	zipToolNames = []string{"7za", "7z"}
)

func findToolByName(names ...string) (string, error) {
	for _, name := range names {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}

	return "", RequiredToolNotInstalled
}

func Compress(archPath, contentDir string) error {
	if filepath.Ext(archPath) != archExt {
		archPath += archExt
	}

	toolPath, err := findToolByName(zipToolNames...)
	if err != nil {
		return RequiredToolNotInstalled
	}

	toolCmd := exec.Command(
		toolPath,
		"a", archPath,
		contentDir+string(filepath.Separator)+"*",
	)

	log.Printf("compressing %s...", archPath)
	if output, err := toolCmd.CombinedOutput(); err != nil {
		fmt.Println(string(output))
		return errors.Join(fmt.Errorf("compression of %s failed", archPath), err)
	}

	return nil
}

func Decompress(archPath string, allocator *TempDirAllocator) (string, error) {
	archPathAbs, err := filepath.Abs(archPath)
	if err != nil {
		return "", err
	}

	var (
		toolNames []string
		toolArgs  []string
	)

	switch strings.ToLower(filepath.Ext(archPathAbs)) {
	case ".rar", ".cbr":
		toolNames = rarToolNames
		toolArgs = []string{"x", archPathAbs}
	case ".zip", ".cbz", ".7z":
		toolNames = zipToolNames
		toolArgs = []string{"x", archPathAbs}
	default:
		return "", UnsupportedArchiveFormatError
	}

	toolPath, err := findToolByName(toolNames...)
	if err != nil {
		return "", err
	}

	contentDir, err := allocator.TempDir()
	if err != nil {
		return "", err
	}

	toolCmd := exec.Command(toolPath, toolArgs...)
	toolCmd.Dir = contentDir

	log.Printf("decompressing %s...", archPath)
	if output, err := toolCmd.CombinedOutput(); err != nil {
		fmt.Println(string(output))
		return "", errors.Join(fmt.Errorf("decompression of %s failed", archPath), err)
	}

	return contentDir, nil
}
