package sandbox

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidatePath(t *testing.T) {
	tempDir := t.TempDir()
	realWorkspace := filepath.Join(tempDir, "workspace")
	if err := os.Mkdir(realWorkspace, 0755); err != nil {
		t.Fatalf("failed to create workspace: %v", err)
	}

	subDir := filepath.Join(realWorkspace, "subdir")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatalf("failed to create subdir: %v", err)
	}

	testFile := filepath.Join(subDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("hello"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// 1. Valid relative path
	validPath, err := ValidatePath(realWorkspace, "subdir/test.txt")
	if err != nil {
		t.Fatalf("expected valid path, got error: %v", err)
	}
	evalExpected, err := filepath.EvalSymlinks(testFile)
	if err != nil {
		evalExpected = testFile
	}
	if validPath != evalExpected {
		t.Errorf("expected %s, got %s", evalExpected, validPath)
	}

	// 2. Directory traversal attempt
	_, err = ValidatePath(realWorkspace, "../outside.txt")
	if err == nil {
		t.Errorf("expected error for directory traversal, got nil")
	}

	// 3. Nested traversal
	_, err = ValidatePath(realWorkspace, "subdir/../../outside.txt")
	if err == nil {
		t.Errorf("expected error for nested traversal, got nil")
	}

	// 4. Prefix confusion attack (workspace_suffix)
	outsidePrefixDir := filepath.Join(tempDir, "workspace_sibling")
	if err := os.Mkdir(outsidePrefixDir, 0755); err != nil {
		t.Fatalf("failed to create outside dir: %v", err)
	}
	outsideFile := filepath.Join(outsidePrefixDir, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("secret"), 0644); err != nil {
		t.Fatalf("failed to write secret file: %v", err)
	}

	_, err = ValidatePath(realWorkspace, outsideFile)
	if err == nil {
		t.Errorf("expected error for sibling path outside workspace, got nil")
	}

	// 5. Symlink pointing outside workspace
	outsideSecret := filepath.Join(tempDir, "external_secret.txt")
	if err := os.WriteFile(outsideSecret, []byte("hidden"), 0644); err != nil {
		t.Fatalf("failed to create external secret: %v", err)
	}
	symlinkPath := filepath.Join(realWorkspace, "symlink_outside")
	if err := os.Symlink(outsideSecret, symlinkPath); err != nil {
		t.Skip("skipping symlink test if symlinks are not supported on filesystem")
	} else {
		_, err = ValidatePath(realWorkspace, "symlink_outside")
		if err == nil {
			t.Errorf("expected error for symlink pointing outside workspace, got nil")
		}
	}
}
