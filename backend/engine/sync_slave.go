package engine

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"nginxflow/config"
	"nginxflow/db"
)

type syncServer struct {
	Address string `json:"address"`
	Port    int64  `json:"port"`
	Weight  int64  `json:"weight"`
	State   string `json:"state"`
}

type syncRule struct {
	ID           int64        `json:"id"`
	Name         string       `json:"name"`
	Protocol     string       `json:"protocol"`
	ListenPort   int64        `json:"listen_port"`
	ListenStack  string       `json:"listen_stack"`
	HttpsEnabled int64        `json:"https_enabled"`
	HttpsPort    int64        `json:"https_port"`
	ServerName   string       `json:"server_name"`
	LbMethod     string       `json:"lb_method"`
	SslCertID    int64        `json:"ssl_cert_id"`
	SslRedirect  int64        `json:"ssl_redirect"`
	HcEnabled    int64        `json:"hc_enabled"`
	HcInterval   int64        `json:"hc_interval"`
	HcTimeout    int64        `json:"hc_timeout"`
	HcPath       string       `json:"hc_path"`
	HcFall       int64        `json:"hc_fall"`
	HcRise       int64        `json:"hc_rise"`
	LogMaxSize   string       `json:"log_max_size"`
	CustomConfig string       `json:"custom_config"`
	Status       int64        `json:"status"`
	Servers      []syncServer `json:"servers"`
}

type syncCert struct {
	Domain    string `json:"domain"`
	CertPEM   string `json:"cert_pem"`
	KeyPEM    string `json:"key_pem"`
	ExpireAt  string `json:"expire_at"`
	AutoRenew int64  `json:"auto_renew"`
}

type syncExportResp struct {
	Code int `json:"code"`
	Data struct {
		Version      string            `json:"version"`
		GeneratedAt  string            `json:"generated_at"`
		NginxConfigs map[string]string `json:"nginx_configs"`
		Rules        []syncRule        `json:"rules"`
		Certs        json.RawMessage   `json:"certs"` // 兼容旧版 map 和新版 array
		Settings     map[string]string `json:"settings"`
	} `json:"data"`
}

// parseCerts 兼容旧主节点（map 格式）和新主节点（array 格式）
func parseCerts(raw json.RawMessage) []syncCert {
	if len(raw) == 0 {
		return nil
	}
	// 尝试 array 格式（新版主节点）
	var arr []syncCert
	if json.Unmarshal(raw, &arr) == nil {
		return arr
	}
	// 兼容旧版 map 格式：{"domain": {"cert_pem": "...", "key_pem": "..."}}
	var m map[string]struct {
		CertPEM string `json:"cert_pem"`
		KeyPEM  string `json:"key_pem"`
	}
	if json.Unmarshal(raw, &m) == nil {
		certs := make([]syncCert, 0, len(m))
		for domain, v := range m {
			certs = append(certs, syncCert{
				Domain:  domain,
				CertPEM: v.CertPEM,
				KeyPEM:  v.KeyPEM,
			})
		}
		return certs
	}
	return nil
}

var lastSyncVersion string

// StartSlaveSyncAgent 从节点定时拉取主节点配置并应用
func StartSlaveSyncAgent() {
	for {
		masterURL, token, intervalStr := getSlaveConfig()
		if masterURL != "" && token != "" {
			interval, _ := strconv.Atoi(intervalStr)
			if interval < 10 {
				interval = 60
			}
			if err := pullAndApply(masterURL, token); err != nil {
				log.Printf("[slave-sync] 同步失败: %v", err)
				setSyncStatus("error", err.Error())
			}
			time.Sleep(time.Duration(interval) * time.Second)
		} else {
			time.Sleep(30 * time.Second)
		}
	}
}

func getSlaveConfig() (masterURL, token, interval string) {
	rows, _ := db.DB.Query(`SELECT k,v FROM system_settings WHERE k IN ('slave_master_url','slave_sync_token','slave_interval')`)
	if rows == nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		rows.Scan(&k, &v)
		switch k {
		case "slave_master_url":
			masterURL = v
		case "slave_sync_token":
			token = v
		case "slave_interval":
			interval = v
		}
	}
	return
}

func pullAndApply(masterURL, token string) error {
	url := fmt.Sprintf("%s/api/v1/sync/export?token=%s", masterURL, token)
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("请求主节点失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("主节点返回 %d: %s", resp.StatusCode, string(body))
	}

	var result syncExportResp
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("解析响应失败: %v", err)
	}
	if result.Code != 0 {
		return fmt.Errorf("主节点返回错误码 %d", result.Code)
	}

	newVersion := result.Data.Version
	if newVersion == lastSyncVersion {
		log.Printf("[slave-sync] 版本未变化 (%s)，跳过", newVersion[:min(16, len(newVersion))])
		return nil
	}

	log.Printf("[slave-sync] 检测到新版本 %s，开始同步...", newVersion[:min(16, len(newVersion))])

	// 写入 nginx 配置文件
	confDir := config.Global.Nginx.ConfDir
	if err := os.MkdirAll(confDir, 0755); err != nil {
		return fmt.Errorf("创建 conf 目录失败: %v", err)
	}
	for filename, content := range result.Data.NginxConfigs {
		path := filepath.Join(confDir, filename)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return fmt.Errorf("写入 %s 失败: %v", filename, err)
		}
	}

	// 同步规则到本地 DB（保持与主节点相同的 ID，UI 才能显示）
	if len(result.Data.Rules) > 0 {
		masterIDs := make([]interface{}, 0, len(result.Data.Rules))
		placeholders := ""
		for i, r := range result.Data.Rules {
			masterIDs = append(masterIDs, r.ID)
			if i > 0 {
				placeholders += ","
			}
			placeholders += "?"
		}
		// 删除主节点已不存在的规则
		db.DB.Exec("DELETE FROM rules WHERE id NOT IN ("+placeholders+")", masterIDs...)

		for _, r := range result.Data.Rules {
			var sslCertID interface{}
			if r.SslCertID > 0 {
				sslCertID = r.SslCertID
			}
			db.DB.Exec(`INSERT INTO rules(id,name,protocol,listen_port,listen_stack,
				https_enabled,https_port,server_name,lb_method,ssl_cert_id,ssl_redirect,
				hc_enabled,hc_interval,hc_timeout,hc_path,hc_fall,hc_rise,
				log_max_size,custom_config,status)
				VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
				ON CONFLICT(id) DO UPDATE SET
				name=excluded.name, protocol=excluded.protocol, listen_port=excluded.listen_port,
				listen_stack=excluded.listen_stack, https_enabled=excluded.https_enabled,
				https_port=excluded.https_port, server_name=excluded.server_name,
				lb_method=excluded.lb_method, ssl_cert_id=excluded.ssl_cert_id,
				ssl_redirect=excluded.ssl_redirect, hc_enabled=excluded.hc_enabled,
				hc_interval=excluded.hc_interval, hc_timeout=excluded.hc_timeout,
				hc_path=excluded.hc_path, hc_fall=excluded.hc_fall, hc_rise=excluded.hc_rise,
				log_max_size=excluded.log_max_size, custom_config=excluded.custom_config,
				status=excluded.status, updated_at=datetime('now','localtime')`,
				r.ID, r.Name, r.Protocol, r.ListenPort, r.ListenStack,
				r.HttpsEnabled, r.HttpsPort, r.ServerName, r.LbMethod, sslCertID, r.SslRedirect,
				r.HcEnabled, r.HcInterval, r.HcTimeout, r.HcPath, r.HcFall, r.HcRise,
				r.LogMaxSize, r.CustomConfig, r.Status)

			// 重建该规则的后端节点
			db.DB.Exec(`DELETE FROM upstream_servers WHERE rule_id=?`, r.ID)
			for _, s := range r.Servers {
				db.DB.Exec(`INSERT INTO upstream_servers(rule_id,address,port,weight,state) VALUES(?,?,?,?,?)`,
					r.ID, s.Address, s.Port, s.Weight, s.State)
			}
		}
		log.Printf("[slave-sync] 同步规则 %d 条", len(result.Data.Rules))
	}

	// 同步证书到本地 DB 和磁盘（auto_renew 强制为 0，从节点不自动续签）
	for _, cert := range parseCerts(result.Data.Certs) {
		expireAt := cert.ExpireAt
		if expireAt == "" {
			expireAt = "2099-01-01 00:00:00"
		}
		db.DB.Exec(`INSERT INTO ssl_certs(domain,cert_pem,key_pem,expire_at,auto_renew)
			VALUES(?,?,?,?,0)
			ON CONFLICT(domain) DO UPDATE SET
			cert_pem=excluded.cert_pem, key_pem=excluded.key_pem,
			expire_at=excluded.expire_at, auto_renew=0,
			updated_at=datetime('now','localtime')`,
			cert.Domain, cert.CertPEM, cert.KeyPEM, expireAt)

		if err := WriteCert(cert.Domain, cert.CertPEM, cert.KeyPEM); err != nil {
			log.Printf("[slave-sync] 写入证书文件 %s 失败: %v", cert.Domain, err)
		}
	}
	parsedCerts := parseCerts(result.Data.Certs)
	if len(parsedCerts) > 0 {
		log.Printf("[slave-sync] 同步证书 %d 个", len(parsedCerts))
	}

	// 同步系统设置（跳过从节点专属配置和主节点续签凭证）
	skipSync := map[string]bool{
		"slave_master_url": true, "slave_sync_token": true, "slave_interval": true,
		"slave_last_sync_at": true, "slave_last_status": true, "slave_last_msg": true,
		"acme_email": true, "acme_account_json": true, "acme_account_key": true,
		"dnspod_id": true, "dnspod_key": true,
		"tencent_secret_id": true, "tencent_secret_key": true,
	}
	for k, v := range result.Data.Settings {
		if skipSync[k] {
			continue
		}
		db.DB.Exec(`INSERT INTO system_settings(k,v) VALUES(?,?) ON CONFLICT(k) DO UPDATE SET v=excluded.v`, k, v)
	}

	// 重载 nginx
	if err := Reload(); err != nil {
		return fmt.Errorf("nginx 重载失败: %v", err)
	}

	lastSyncVersion = newVersion
	setSyncStatus("ok", fmt.Sprintf("同步成功，版本 %s", newVersion[:min(16, len(newVersion))]))
	log.Printf("[slave-sync] 同步完成，版本 %s，规则 %d 个，证书 %d 个",
		newVersion[:min(16, len(newVersion))], len(result.Data.Rules), len(parsedCerts))
	return nil
}

func setSyncStatus(status, msg string) {
	now := time.Now().Format("2006-01-02 15:04:05")
	db.DB.Exec(`INSERT INTO system_settings(k,v) VALUES('slave_last_sync_at',?) ON CONFLICT(k) DO UPDATE SET v=excluded.v`, now)
	db.DB.Exec(`INSERT INTO system_settings(k,v) VALUES('slave_last_status',?) ON CONFLICT(k) DO UPDATE SET v=excluded.v`, status)
	db.DB.Exec(`INSERT INTO system_settings(k,v) VALUES('slave_last_msg',?) ON CONFLICT(k) DO UPDATE SET v=excluded.v`, msg)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
