package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/codeshelldev/gotl/pkg/logger"
	"github.com/codeshelldev/secured-signal-api/internals/config"
)

var fileLog struct {
	sync.Mutex
	file *os.File
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

	fileLog.Lock()
	defer fileLog.Unlock()

	if fileLog.file != nil {
		if _, err := fmt.Fprintln(fileLog.file, content); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, "Could not write log file: ", err)
		}
	}

	return content
}

func Init(level string) {
	options := logger.DefaultOptions()

	logger.InitWith(level, options)
	logger.InitStdLoggerWith(level, options)
}

func ConfigureFile(path string) error {
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

	fileLog.Lock()
	defer fileLog.Unlock()

	if fileLog.file != nil {
		_ = fileLog.file.Close()
	}

	fileLog.file = file
	return nil
}

func CloseFile() {
	fileLog.Lock()
	defer fileLog.Unlock()

	if fileLog.file != nil {
		_ = fileLog.file.Close()
		fileLog.file = nil
	}
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
