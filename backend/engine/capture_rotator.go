package engine

import (
	"bufio"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"ankerye-flow/config"
)

// 每个规则 capture log 上限，超过则裁剪到最后 captureMaxBytes 字节（保留最新数据）。
const captureMaxBytes int64 = 5 * 1024 * 1024 // 5 MB

// StartCaptureRotator 每 1 分钟检测一次所有 rule_X_capture.log，
// 文件超过 5MB 则尾部保留 5MB（裁掉前面老数据）。
// 配合「全规则开启 capture」使用，相当于固定 5MB 的滚动调试缓冲。
func StartCaptureRotator() {
	log.Printf("[capture-rotator] started, check=1m, max=%d MB per file", captureMaxBytes/1024/1024)
	for {
		time.Sleep(1 * time.Minute)
		trimAllCaptureLogs()
	}
}

func trimAllCaptureLogs() {
	pattern := filepath.Join(config.Global.Nginx.LogDir, "rule_*_capture.log")
	files, _ := filepath.Glob(pattern)
	if len(files) == 0 {
		return
	}
	trimmed := 0
	for _, f := range files {
		if trimCaptureLog(f, captureMaxBytes) {
			trimmed++
		}
	}
	if trimmed > 0 {
		// 通知 nginx 重新打开 capture 文件（fd offset 从新文件 0 开始）
		_ = exec.Command("nginx", "-s", "reopen").Run()
		log.Printf("[capture-rotator] trimmed %d capture logs to last %d MB", trimmed, captureMaxBytes/1024/1024)
	}
}

// trimCaptureLog 若文件超过 maxBytes，截掉前面只保留尾部 maxBytes 字节。
// 用 rename 策略（写新文件 + 原子替换），nginx 端通过 -s reopen 切换到新 inode。
func trimCaptureLog(path string, maxBytes int64) bool {
	st, err := os.Stat(path)
	if err != nil || st.Size() <= maxBytes {
		return false
	}

	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	// 从尾部往前定位到 maxBytes 处
	if _, err := f.Seek(-maxBytes, io.SeekEnd); err != nil {
		return false
	}

	br := bufio.NewReaderSize(f, 64*1024)
	// 跳过开头那条不完整的行
	if _, err := br.ReadBytes('\n'); err != nil {
		return false
	}

	// 写到同目录临时文件
	tmp := path + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return false
	}
	if _, err := io.Copy(out, br); err != nil {
		_ = out.Close()
		_ = os.Remove(tmp)
		return false
	}
	_ = out.Close()

	// 原子替换；nginx 旧 fd 仍指向被取消链接的旧 inode，下一次 reopen 后切到新文件
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return false
	}
	return true
}
