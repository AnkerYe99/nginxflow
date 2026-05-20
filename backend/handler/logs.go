package handler

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"ankerye-flow/config"
	"ankerye-flow/db"
	"ankerye-flow/util"
)

var logFilePattern = regexp.MustCompile(`^rule_(\d+)_(access|error|capture|stream)\.log$`)

type logFileInfo struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	RuleID    int64  `json:"rule_id"`
	RuleName  string `json:"rule_name"`
	Type      string `json:"type"`
	SizeBytes int64  `json:"size_bytes"`
	SizeHuman string `json:"size_human"`
	ModTime   string `json:"mod_time"`
}

// ListLogs 列出 LogDir 下所有 rule_X_*.log 文件
func ListLogs(c *gin.Context) {
	logDir := config.Global.Nginx.LogDir
	entries, err := os.ReadDir(logDir)
	if err != nil {
		util.Fail(c, 500, "读取日志目录失败: "+err.Error())
		return
	}

	// 预加载所有规则名称
	ruleNames := map[int64]string{}
	rows, _ := db.DB.Query(`SELECT id, name FROM rules`)
	if rows != nil {
		for rows.Next() {
			var id int64
			var name string
			rows.Scan(&id, &name)
			ruleNames[id] = name
		}
		rows.Close()
	}

	var list []logFileInfo
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		m := logFilePattern.FindStringSubmatch(name)
		if m == nil {
			continue
		}
		ruleID, _ := strconv.ParseInt(m[1], 10, 64)
		logType := m[2]

		info, err := e.Info()
		if err != nil {
			continue
		}

		list = append(list, logFileInfo{
			Name:      name,
			Path:      name, // 只暴露文件名，后端根据此校验
			RuleID:    ruleID,
			RuleName:  ruleNames[ruleID],
			Type:      logType,
			SizeBytes: info.Size(),
			SizeHuman: humanSize(info.Size()),
			ModTime:   info.ModTime().Format("2006-01-02 15:04:05"),
		})
	}

	// 按修改时间降序排列
	sort.Slice(list, func(i, j int) bool {
		return list[i].ModTime > list[j].ModTime
	})

	util.OK(c, list)
}

// ViewLog 返回日志文件最后 N 行内容（默认 500 行）
func ViewLog(c *gin.Context) {
	name := c.Query("file")
	if !isValidLogFile(name) {
		util.Fail(c, 400, "无效的日志文件名")
		return
	}
	lines := 500
	if n, err := strconv.Atoi(c.Query("lines")); err == nil && n > 0 && n <= 5000 {
		lines = n
	}

	path := filepath.Join(config.Global.Nginx.LogDir, name)
	content, err := tailLines(path, lines)
	if err != nil {
		util.Fail(c, 404, "读取日志失败: "+err.Error())
		return
	}
	util.OK(c, gin.H{"file": name, "lines": len(content), "content": strings.Join(content, "\n")})
}

// DownloadLog 强制下载日志文件
func DownloadLog(c *gin.Context) {
	name := c.Query("file")
	if !isValidLogFile(name) {
		util.Fail(c, 400, "无效的日志文件名")
		return
	}
	path := filepath.Join(config.Global.Nginx.LogDir, name)
	if _, err := os.Stat(path); err != nil {
		util.Fail(c, 404, "文件不存在")
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
	c.Header("Content-Type", "application/octet-stream")
	c.File(path)
}

// DeleteLog 清空日志文件（截断为 0 字节，文件保留）
func DeleteLog(c *gin.Context) {
	name := c.Query("file")
	if !isValidLogFile(name) {
		util.Fail(c, 400, "无效的日志文件名")
		return
	}
	path := filepath.Join(config.Global.Nginx.LogDir, name)

	// 如果文件不存在则直接新建空文件
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		util.Fail(c, 500, "清空日志失败: "+err.Error())
		return
	}
	f.Close()

	util.OK(c, gin.H{"file": name, "msg": "已清空"})
}

// isValidLogFile 校验文件名只能是 rule_X_(access|error|capture|stream).log
func isValidLogFile(name string) bool {
	if name == "" || strings.Contains(name, "/") || strings.Contains(name, "..") {
		return false
	}
	return logFilePattern.MatchString(name)
}

// tailLines 读取文件最后 n 行
func tailLines(path string, n int) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		// 文件不存在视为空文件
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) > n {
			lines = lines[1:]
		}
	}
	return lines, scanner.Err()
}

func humanSize(b int64) string {
	switch {
	case b >= 1024*1024*1024:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(1024*1024*1024))
	case b >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(1024*1024))
	case b >= 1024:
		return fmt.Sprintf("%.1f KB", float64(b)/1024)
	default:
		return fmt.Sprintf("%d B", b)
	}
}
