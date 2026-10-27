package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"mime/multipart"
	"net/smtp"
	"net/textproto"
	"time"
)

func (s *Server) SendStrangerAlert(recipientEmail string, imageBytes []byte) error {
	s.mu.Lock()
	fromEmail := s.senderEmail
	fromPassword := s.senderPassword
	s.mu.Unlock()

	if fromEmail == "" || fromPassword == "" || recipientEmail == "" {
		return fmt.Errorf("credentials missing: sender=%s, recipient=%s", fromEmail, recipientEmail)
	}

	auth := smtp.PlainAuth("", fromEmail, fromPassword, "smtp.gmail.com")

	buf := new(bytes.Buffer)
	writer := multipart.NewWriter(buf)

	headers := make(textproto.MIMEHeader)
	headers.Set("From", fromEmail)
	headers.Set("To", recipientEmail)
	headers.Set("Subject", "⚠️ WATCHMANX ALERT: Unknown Stranger Detected!")
	headers.Set("MIME-Version", "1.0")
	headers.Set("Content-Type", fmt.Sprintf("multipart/mixed; boundary=%s", writer.Boundary()))

	headerBuf := new(bytes.Buffer)
	for k, v := range headers {
		headerBuf.WriteString(fmt.Sprintf("%s: %s\r\n", k, v[0]))
	}
	headerBuf.WriteString("\r\n")

	bodyPart, err := writer.CreatePart(textproto.MIMEHeader{
		"Content-Type": []string{"text/html; charset=UTF-8"},
	})
	if err != nil {
		return err
	}

	currentTime := time.Now().Format("02-Jan-2006 03:04:05 PM")
	htmlBody := fmt.Sprintf(`
		<div style="background-color: #030712; color: #f1f5f9; padding: 25px; border-radius: 12px; font-family: Arial, sans-serif; border: 1px solid #ef4444;">
			<h2 style="color: #ef4444; margin-top: 0;">🚨 UNKNOWN STRANGER ALERT</h2>
			<p style="font-size: 15px;">WatchmanX Core Surveillance Engine has detected an unauthorized face in the live video feed.</p>
			<p><strong>Timestamp:</strong> %s</p>
			<hr style="border: 0; border-top: 1px solid #334155; margin: 20px 0;" />
			<p style="color: #94a3b8; font-size: 13px;">The captured face image is attached to this email.</p>
		</div>
	`, currentTime)
	bodyPart.Write([]byte(htmlBody))

	attachmentPart, err := writer.CreatePart(textproto.MIMEHeader{
		"Content-Type":              []string{"image/jpeg"},
		"Content-Transfer-Encoding": []string{"base64"},
		"Content-Disposition":       []string{`attachment; filename="stranger_capture.jpg"`},
	})
	if err != nil {
		return err
	}

	b64Data := base64.StdEncoding.EncodeToString(imageBytes)
	for i := 0; i < len(b64Data); i += 76 {
		end := i + 76
		if end > len(b64Data) {
			end = len(b64Data)
		}
		attachmentPart.Write([]byte(b64Data[i:end] + "\r\n"))
	}

	writer.Close()

	finalMsg := append(headerBuf.Bytes(), buf.Bytes()...)
	return smtp.SendMail("smtp.gmail.com:587", auth, fromEmail, []string{recipientEmail}, finalMsg)
}
