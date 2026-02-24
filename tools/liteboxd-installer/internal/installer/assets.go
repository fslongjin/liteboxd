package installer

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed assets/deploy/system/* assets/deploy/sandbox/* assets/deploy/sandbox/egress-gateway/*
var embeddedDeployFS embed.FS

func extractEmbeddedDeploy(dst string) error {
	if err := extractDir("assets/deploy/system", filepath.Join(dst, "base", "system")); err != nil {
		return err
	}
	if err := extractDir("assets/deploy/sandbox", filepath.Join(dst, "base", "sandbox")); err != nil {
		return err
	}
	return nil
}

func extractDir(srcDir, dstDir string) error {
	entries, err := fs.ReadDir(embeddedDeployFS, srcDir)
	if err != nil {
		return fmt.Errorf("read embedded dir %s: %w", srcDir, err)
	}
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return fmt.Errorf("create dir %s: %w", dstDir, err)
	}
	for _, e := range entries {
		srcPath := filepath.ToSlash(filepath.Join(srcDir, e.Name()))
		dstPath := filepath.Join(dstDir, e.Name())
		if e.IsDir() {
			if err := extractDir(srcPath, dstPath); err != nil {
				return err
			}
			continue
		}

		content, readErr := embeddedDeployFS.ReadFile(srcPath)
		if readErr != nil {
			return fmt.Errorf("read embedded file %s: %w", srcPath, readErr)
		}
		if err := os.WriteFile(dstPath, content, 0o644); err != nil {
			return fmt.Errorf("write file %s: %w", dstPath, err)
		}
	}
	return nil
}
