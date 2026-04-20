package utils

// 文件说明：这个文件封装 SMTP 邮件发送能力。
// 实现方式：根据端口自动选择 SMTPS 或 STARTTLS，再统一处理消息头、认证和正文写入。
// 这样做的好处是业务侧只需要给出收件人和正文，不必关心 SMTP 握手细节。

import (
	"ToDoList/server/config"
	"crypto/tls"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"strconv"
	"strings"
)

// SendEmail 发送一封纯文本邮件。
// 这里按端口区分 465 和 STARTTLS 流程，是为了兼容常见 SMTP 服务商配置。
func SendEmail(cfg config.EmailConfig, to string, subject string, body string) error {
	host := strings.TrimSpace(cfg.Host)
	to = strings.TrimSpace(to)
	from := strings.TrimSpace(cfg.From)
	username := strings.TrimSpace(cfg.Username)

	if host == "" || cfg.Port == 0 {
		return fmt.Errorf("email config is missing or invalid")
	}
	if to == "" {
		return fmt.Errorf("recipient email is empty")
	}
	if from == "" {
		from = username
	}
	if from == "" {
		return fmt.Errorf("email sender is empty")
	}

	msg := buildEmailMessage(from, to, subject, body)
	addr := net.JoinHostPort(host, strconv.Itoa(cfg.Port))
	auth := smtp.PlainAuth("", username, cfg.Password, host)

	var err error
	if cfg.Port == 465 {
		err = sendViaTLS(addr, host, auth, username, from, to, msg)
	} else {
		err = sendViaStartTLS(addr, host, auth, username, from, to, msg)
	}
	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	return nil
}

// buildEmailMessage 构造带 UTF-8 头部的邮件报文。
func buildEmailMessage(from, to, subject, body string) []byte {
	fromHeader := fmt.Sprintf("%s <%s>", mime.QEncoding.Encode("utf-8", "待办系统"), from)

	from = fromHeader
	to = sanitizeHeaderValue(to)
	subject = mime.QEncoding.Encode("utf-8", sanitizeHeaderValue(subject))

	return []byte(fmt.Sprintf("To: %s\r\n"+
		"From: %s\r\n"+
		"Subject: %s\r\n"+
		"MIME-Version: 1.0\r\n"+
		"Content-Type: text/plain; charset=UTF-8\r\n"+
		"Content-Transfer-Encoding: 8bit\r\n\r\n"+
		"%s", to, from, subject, body))
}

// sendViaStartTLS 通过 STARTTLS 发送邮件。
func sendViaStartTLS(addr, host string, auth smtp.Auth, username, from, to string, msg []byte) error {
	client, err := smtp.Dial(addr)
	if err != nil {
		return fmt.Errorf("dial smtp server: %w", err)
	}
	defer client.Close()

	if ok, _ := client.Extension("STARTTLS"); ok {
		tlsConfig := &tls.Config{
			ServerName: host,
			MinVersion: tls.VersionTLS12,
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			return fmt.Errorf("starttls failed: %w", err)
		}
	}

	if strings.TrimSpace(username) != "" {
		if ok, _ := client.Extension("AUTH"); !ok {
			return fmt.Errorf("smtp auth is not supported by server")
		}
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth failed: %w", err)
		}
	}

	return sendMailData(client, from, to, msg)
}

// sendViaTLS 通过 SMTPS 直接发送邮件。
func sendViaTLS(addr, host string, auth smtp.Auth, username, from, to string, msg []byte) error {
	tlsConfig := &tls.Config{
		ServerName: host,
		MinVersion: tls.VersionTLS12,
	}
	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("dial tls smtp server: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("create smtp client: %w", err)
	}
	defer client.Close()

	if strings.TrimSpace(username) != "" {
		if ok, _ := client.Extension("AUTH"); !ok {
			return fmt.Errorf("smtp auth is not supported by server")
		}
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth failed: %w", err)
		}
	}

	return sendMailData(client, from, to, msg)
}

// sendMailData 向 SMTP 会话写入发件人、收件人和邮件正文。
func sendMailData(client *smtp.Client, from, to string, msg []byte) error {
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("set mail from failed: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("set recipient failed: %w", err)
	}

	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("open data writer failed: %w", err)
	}
	if _, err := writer.Write(msg); err != nil {
		_ = writer.Close()
		return fmt.Errorf("write mail body failed: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("close data writer failed: %w", err)
	}

	if err := client.Quit(); err != nil {
		return fmt.Errorf("quit smtp session failed: %w", err)
	}
	return nil
}

// sanitizeHeaderValue 清理邮件头字段，避免换行注入。
func sanitizeHeaderValue(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return value
}
