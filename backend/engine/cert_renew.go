package engine

import (
	"archive/zip"
	"bytes"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	ssl "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ssl/v20191205"

	"nginxflow/db"
)

func getTencentSSLClient() (*ssl.Client, error) {
	var sid, skey string
	db.DB.QueryRow(`SELECT v FROM system_settings WHERE k='tencent_secret_id'`).Scan(&sid)
	db.DB.QueryRow(`SELECT v FROM system_settings WHERE k='tencent_secret_key'`).Scan(&skey)
	if sid == "" || skey == "" {
		return nil, fmt.Errorf("未配置腾讯云 API 密钥，请在系统设置中填写 SecretId 和 SecretKey")
	}
	credential := common.NewCredential(sid, skey)
	cpf := profile.NewClientProfile()
	cpf.HttpProfile.Endpoint = "ssl.tencentcloudapi.com"
	client, err := ssl.NewClient(credential, "ap-guangzhou", cpf)
	if err != nil {
		return nil, fmt.Errorf("初始化腾讯云客户端失败: %v", err)
	}
	return client, nil
}

// RenewCert 向腾讯云申请新证书，异步轮询等待签发后自动安装
func RenewCert(certID int64, domain string) error {
	client, err := getTencentSSLClient()
	if err != nil {
		return err
	}

	req := ssl.NewApplyCertificateRequest()
	dvAuthMethod := "DNS_AUTO"
	req.DvAuthMethod = &dvAuthMethod
	req.DomainName = &domain

	resp, err := client.ApplyCertificate(req)
	if err != nil {
		return fmt.Errorf("申请证书失败: %v", err)
	}

	tcCertID := *resp.Response.CertificateId
	setRenewStatus(certID, "pending", tcCertID, "已提交腾讯云申请，等待 DNS 自动验证和签发（约 5-30 分钟）")
	log.Printf("[renew] cert %s applied, tencent id: %s", domain, tcCertID)

	go pollAndInstall(certID, domain, tcCertID)
	return nil
}

func pollAndInstall(certID int64, domain, tcCertID string) {
	client, err := getTencentSSLClient()
	if err != nil {
		setRenewStatus(certID, "failed", tcCertID, err.Error())
		return
	}

	for i := 0; i < 72; i++ { // 最多等 36 分钟
		time.Sleep(30 * time.Second)

		req := ssl.NewDescribeCertificateDetailRequest()
		req.CertificateId = &tcCertID
		resp, err := client.DescribeCertificateDetail(req)
		if err != nil {
			log.Printf("[renew] poll %s error: %v", tcCertID, err)
			continue
		}

		status := int(*resp.Response.Status)
		log.Printf("[renew] cert %s status: %d", tcCertID, status)

		switch status {
		case 1: // 已签发
			if err := downloadAndInstall(client, certID, domain, tcCertID); err != nil {
				setRenewStatus(certID, "failed", tcCertID, "安装证书失败: "+err.Error())
			}
			return
		case 4, 5, 6, 7: // 各类失败状态
			setRenewStatus(certID, "failed", tcCertID, fmt.Sprintf("证书签发失败（状态 %d），请检查域名 DNS 是否托管在腾讯云", status))
			return
		}
	}
	setRenewStatus(certID, "failed", tcCertID, "等待签发超时（36分钟），请手动检查腾讯云控制台")
}

func downloadAndInstall(client *ssl.Client, certID int64, domain, tcCertID string) error {
	req := ssl.NewDownloadCertificateRequest()
	req.CertificateId = &tcCertID

	resp, err := client.DownloadCertificate(req)
	if err != nil {
		return fmt.Errorf("下载证书失败: %v", err)
	}

	zipData, err := base64.StdEncoding.DecodeString(*resp.Response.Content)
	if err != nil {
		return fmt.Errorf("解码证书 zip 失败: %v", err)
	}

	certPEM, keyPEM, err := extractNginxPEM(zipData)
	if err != nil {
		return err
	}

	// 解析到期时间
	block, _ := pem.Decode([]byte(certPEM))
	x509Cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("解析新证书失败: %v", err)
	}
	expireAt := x509Cert.NotAfter.Format("2006-01-02 15:04:05")

	db.DB.Exec(`UPDATE ssl_certs SET cert_pem=?, key_pem=?, expire_at=?, tencent_cert_id=?,
		renew_status='success', renew_log='续签成功',
		last_renew_at=datetime('now','localtime'), updated_at=datetime('now','localtime')
		WHERE id=?`, certPEM, keyPEM, expireAt, tcCertID, certID)

	if err := WriteCert(domain, certPEM, keyPEM); err != nil {
		return fmt.Errorf("写入证书文件失败: %v", err)
	}
	Reload()

	log.Printf("[renew] cert %s renewed, expires %s", domain, expireAt)
	return nil
}

// extractNginxPEM 从腾讯云下载的 zip 中提取 Nginx 用的证书和私钥
func extractNginxPEM(data []byte) (certPEM, keyPEM string, err error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", "", fmt.Errorf("解压 zip 失败: %v", err)
	}

	for _, f := range r.File {
		name := strings.ToLower(f.Name)
		// 跳过目录
		if f.FileInfo().IsDir() {
			continue
		}
		rc, _ := f.Open()
		content, _ := io.ReadAll(rc)
		rc.Close()
		s := string(content)

		if strings.HasSuffix(name, ".key") {
			keyPEM = s
		} else if strings.HasSuffix(name, ".crt") || strings.HasSuffix(name, ".pem") {
			if !strings.Contains(name, "key") {
				certPEM = s
			}
		}
	}

	if certPEM == "" || keyPEM == "" {
		return "", "", fmt.Errorf("zip 中未找到证书（.crt）或私钥（.key）文件，请确认域名 DNS 托管在腾讯云 DNSPod")
	}
	return certPEM, keyPEM, nil
}

func setRenewStatus(certID int64, status, tcCertID, msg string) {
	db.DB.Exec(`UPDATE ssl_certs SET renew_status=?, tencent_cert_id=?, renew_log=?,
		last_renew_at=datetime('now','localtime') WHERE id=?`, status, tcCertID, msg, certID)
}

// AutoRenewCheck 检查所有开启自动续签的证书，到期前 N 天自动续签
func AutoRenewCheck() {
	rows, _ := db.DB.Query(`SELECT id, domain, expire_at, renew_status FROM ssl_certs WHERE auto_renew=1`)
	defer rows.Close()
	for rows.Next() {
		var id int64
		var domain, expireAt, renewStatus string
		rows.Scan(&id, &domain, &expireAt, &renewStatus)
		if renewStatus == "pending" {
			continue // 已在续签中，跳过
		}
		expire, err := time.Parse("2006-01-02 15:04:05", expireAt)
		if err != nil {
			continue
		}
		daysLeft := int(time.Until(expire).Hours() / 24)
		if daysLeft <= 10 {
			log.Printf("[auto-renew] %s expires in %d days, renewing...", domain, daysLeft)
			if err := RenewCert(id, domain); err != nil {
				log.Printf("[auto-renew] %s failed: %v", domain, err)
			}
		}
	}
}
