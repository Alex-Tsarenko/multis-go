package sandbox

import (
	"context"
	"os"
	"path/filepath"
)

// FileSystem интерфейс для безопасного доступа к файловой системе
type FileSystem interface {
	Read(ctx context.Context, path string) ([]byte, error)
	Write(ctx context.Context, path string, data []byte) error
	Exists(ctx context.Context, path string) (bool, error)
	List(ctx context.Context, path string) ([]string, error)
	Metadata(ctx context.Context, path string) (os.FileInfo, error)
}

// SafeFileSystem безопасная реализация FileSystem с валидацией путей
type SafeFileSystem struct {
	workspace string
}

// NewSafeFileSystem создаёт новый SafeFileSystem
func NewSafeFileSystem(workspace string) (*SafeFileSystem, error) {
	absWorkspace, err := filepath.Abs(workspace)
	if err != nil {
		return nil, err
	}

	cleanWorkspace := filepath.Clean(absWorkspace)
	return &SafeFileSystem{workspace: cleanWorkspace}, nil
}

// Read читает файл по пути
func (fs *SafeFileSystem) Read(ctx context.Context, path string) ([]byte, error) {
	validPath, err := fs.validatePath(path)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(validPath)
}

// Write записывает данные в файл
func (fs *SafeFileSystem) Write(ctx context.Context, path string, data []byte) error {
	validPath, err := fs.validatePath(path)
	if err != nil {
		return err
	}
	return os.WriteFile(validPath, data, 0644)
}

// Exists проверяет существование пути
func (fs *SafeFileSystem) Exists(ctx context.Context, path string) (bool, error) {
	validPath, err := fs.validatePath(path)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(validPath)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// List возвращает список файлов в директории
func (fs *SafeFileSystem) List(ctx context.Context, path string) ([]string, error) {
	validPath, err := fs.validatePath(path)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(validPath)
	if err != nil {
		return nil, err
	}
	result := make([]string, 0, len(entries))
	for _, entry := range entries {
		result = append(result, entry.Name())
	}
	return result, nil
}

// Metadata возвращает метаданные файла
func (fs *SafeFileSystem) Metadata(ctx context.Context, path string) (os.FileInfo, error) {
	validPath, err := fs.validatePath(path)
	if err != nil {
		return nil, err
	}
	return os.Stat(validPath)
}

// validatePath валидирует и нормализует путь
func (fs *SafeFileSystem) validatePath(path string) (string, error) {
	return ValidatePath(fs.workspace, path)
}

// PathValidationError ошибка валидации пути
type PathValidationError struct {
	Path   string
	Reason string
}

func (e *PathValidationError) Error() string {
	return "path validation failed: " + e.Reason + " (path: " + e.Path + ")"
}
