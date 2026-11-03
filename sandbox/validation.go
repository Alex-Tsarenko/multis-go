package sandbox

import (
	"os"
	"path/filepath"
)

// ValidatePath валидирует путь на предмет выхода за границы workspace с учётом символических ссылок
func ValidatePath(workspace, path string) (string, error) {
	cleanWorkspace := filepath.Clean(workspace)
	evalWorkspace, err := filepath.EvalSymlinks(cleanWorkspace)
	if err == nil {
		cleanWorkspace = evalWorkspace
	}

	absWorkspace, err := filepath.Abs(cleanWorkspace)
	if err != nil {
		return "", err
	}
	absWorkspace = filepath.Clean(absWorkspace)

	cleanPath := filepath.Clean(path)
	if !filepath.IsAbs(cleanPath) {
		cleanPath = filepath.Join(absWorkspace, cleanPath)
	}

	absPath, err := filepath.Abs(cleanPath)
	if err != nil {
		return "", err
	}
	absPath = filepath.Clean(absPath)

	evalPath, err := filepath.EvalSymlinks(absPath)
	if err == nil {
		absPath = filepath.Clean(evalPath)
	} else if !os.IsNotExist(err) {
		return "", err
	}

	relPath, err := filepath.Rel(absWorkspace, absPath)
	if err != nil {
		return "", err
	}

	if relPath == ".." || (len(relPath) >= 3 && relPath[:3] == "../") {
		return "", &PathValidationError{Path: path, Reason: "path outside workspace"}
	}

	return absPath, nil
}

// IsPathInWorkspace проверяет, находится ли путь в workspace
func IsPathInWorkspace(workspace, path string) bool {
	validPath, err := ValidatePath(workspace, path)
	return err == nil && validPath != ""
}
