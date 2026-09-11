package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/codeshelldev/gotl/pkg/logger"
	"github.com/codeshelldev/secured-signal-api/internals/config"
)

var fileLog struct {
	sync.Mutex
	file        *os.File
	path        string
	maxSize     int64
	maxFiles    int
	currentSize int64
}

func DefaultTransforms() []func(string) string {
	transforms := []func(string) string{}

	transforms = append(transforms, BeginWithCapital)

	if config.ENV.REDACT_TOKENS {
		transforms = append(transforms, RedactTokens())
	}

	fileLog.Lock()
	fileEnabled := fileLog.file != nil
	fileLog.Unlock()
	if fileEnabled {
		transforms = append(transforms, writeFileLog)
	}

	return transforms
}

func writeFileLog(content string) string {
	content = strings.NewReplacer("\r", "\\r", "\n", "\\n").Replace(content)
	line := content + "\n"

	fileLog.Lock()
	defer fileLog.Unlock()

	if fileLog.file != nil {
		if fileLog.maxSize > 0 && fileLog.currentSize > 0 && fileLog.currentSize+int64(len(line)) > fileLog.maxSize {
			if err := rotateLocked(); err != nil {
				_, _ = fmt.Fprintln(os.Stderr, "Could not rotate log file: ", err)
				if reopenErr := reopenLocked(); reopenErr != nil {
					_, _ = fmt.Fprintln(os.Stderr, "Could not reopen log file: ", reopenErr)
				}
			}
		}

		if _, err := fileLog.file.WriteString(line); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, "Could not write log file: ", err)
		} else {
			fileLog.currentSize += int64(len(line))
		}
	}

	return content
}

func Init(level string) {
	options := logger.DefaultOptions()

	logger.InitWith(level, options)
	logger.InitStdLoggerWith(level, options)
}

func ConfigureFile(path string, maxSize int64, maxFiles int) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return err
	}

	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0640)
	if err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return err
	}

	fileLog.Lock()
	defer fileLog.Unlock()

	if fileLog.file != nil {
		_ = fileLog.file.Close()
	}

	fileLog.file = file
	fileLog.path = path
	fileLog.maxSize = maxSize
	fileLog.maxFiles = maxFiles
	if maxSize > 0 && fileLog.maxFiles < 1 {
		fileLog.maxFiles = 1
	}
	fileLog.currentSize = info.Size()
	return nil
}

func reopenLocked() error {
	if fileLog.path == "" {
		return nil
	}

	if fileLog.file != nil {
		_ = fileLog.file.Close()
	}

	file, err := os.OpenFile(fileLog.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0640)
	if err != nil {
		fileLog.file = nil
		return err
	}

	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		fileLog.file = nil
		return err
	}

	fileLog.file = file
	fileLog.currentSize = info.Size()
	return nil
}

func rotateLocked() error {
	if fileLog.file == nil || fileLog.path == "" {
		return nil
	}

	if err := fileLog.file.Close(); err != nil {
		return err
	}

	for index := fileLog.maxFiles - 1; index >= 1; index-- {
		oldPath := rotatedPath(index)
		newPath := rotatedPath(index + 1)

		if _, err := os.Stat(oldPath); os.IsNotExist(err) {
			continue
		}
		if err := os.Remove(newPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := os.Rename(oldPath, newPath); err != nil {
			return err
		}
	}

	if fileLog.maxFiles > 0 {
		if err := os.Remove(rotatedPath(1)); err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := os.Rename(fileLog.path, rotatedPath(1)); err != nil {
			return err
		}
	}

	if err := reopenLocked(); err != nil {
		return err
	}
	fileLog.currentSize = 0
	return pruneRotatedFilesLocked()
}

func rotatedPath(index int) string {
	return fmt.Sprintf("%s.%d", fileLog.path, index)
}

func pruneRotatedFilesLocked() error {
	if fileLog.maxFiles < 1 {
		return nil
	}

	entries, err := os.ReadDir(filepath.Dir(fileLog.path))
	if err != nil {
		return err
	}

	prefix := filepath.Base(fileLog.path) + "."
	var rotated []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), prefix) {
			rotated = append(rotated, entry.Name())
		}
	}
	sort.Slice(rotated, func(left, right int) bool {
		leftIndex, _ := strconv.Atoi(strings.TrimPrefix(rotated[left], prefix))
		rightIndex, _ := strconv.Atoi(strings.TrimPrefix(rotated[right], prefix))
		return leftIndex < rightIndex
	})

	for len(rotated) > fileLog.maxFiles {
		if err := os.Remove(filepath.Join(filepath.Dir(fileLog.path), rotated[0])); err != nil {
			return err
		}
		rotated = rotated[1:]
	}
	return nil
}

func CloseFile() {
	fileLog.Lock()
	defer fileLog.Unlock()

	if fileLog.file != nil {
		_ = fileLog.file.Close()
		fileLog.file = nil
	}
	fileLog.path = ""
	fileLog.currentSize = 0
}

func RotateFile() error {
	fileLog.Lock()
	defer fileLog.Unlock()

	if fileLog.file == nil {
		return nil
	}

	return rotateLocked()
}

func ReopenFile() error {
	fileLog.Lock()
	defer fileLog.Unlock()

	return reopenLocked()
}

func Setup() {
	transform := Apply(DefaultTransforms()...)

	logger.Get().SetTransform(transform)
	logger.GetStdLogger().SetTransform(transform)
}

func RedactTokens() func(string) string {
	return RedactWords('*', config.ENV.TOKENS...)
}

func Apply(transforms ...func(content string) string) func(string) string {
	return func(content string) string {
		for _, fn := range transforms {
			content = fn(content)
		}

		return content
	}
}

func BeginWithCapital(content string) string {
	return strings.ToUpper(content[:1]) + content[1:]
}

func Redact(redact string) string {
	if len(redact) <= 1 {
		return strings.Repeat("*", len(redact))
	}

	left := 2
	right := 2

	repeatTimes := 10

	revealLeft := string(redact[:left])
	revealRight := string(redact[len(redact)-right:])

	redactedStr := strings.Repeat("*", repeatTimes)

	if len(redact)-left-right-repeatTimes > 0 {
		redactedStr = strings.Repeat("*", repeatTimes+4) + "(" + strconv.Itoa(len(redact)) + ")" + strings.Repeat("*", repeatTimes-4)
	}

	return revealLeft + redactedStr + revealRight
}

func RedactWords(replaceBy rune, words ...string) func(string) string {
	return func(content string) string {
		for _, word := range words {
			content = strings.ReplaceAll(content, word, "["+Redact(word)+"]")
		}

		return content
	}
}
