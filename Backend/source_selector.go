package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gocv.io/x/gocv"
)

// HandleConfigureEmail saves alert credentials once
func (s *Server) HandleConfigureEmail(c *gin.Context) {
	senderEmail := strings.TrimSpace(c.PostForm("sender_email"))
	appPassword := strings.TrimSpace(c.PostForm("app_password"))
	alertEmail := strings.TrimSpace(c.PostForm("alert_email"))

	if senderEmail == "" || appPassword == "" || alertEmail == "" {
		c.String(http.StatusBadRequest, "Sender email, App Password, and recipient email are all required")
		return
	}

	upsertQuery := `
		INSERT INTO system_settings (id, sender_email, app_password, alert_email, updated_at)
		VALUES (1, $1, $2, $3, NOW())
		ON CONFLICT (id) 
		DO UPDATE SET sender_email = EXCLUDED.sender_email, 
		              app_password = EXCLUDED.app_password, 
		              alert_email = EXCLUDED.alert_email, 
		              updated_at = NOW();
	`
	_, err := s.db.Exec(upsertQuery, senderEmail, appPassword, alertEmail)
	if err != nil {
		log.Printf("[DB ERROR] Failed to save email settings: %v\n", err)
		c.String(http.StatusInternalServerError, "Failed to save settings to database")
		return
	}

	s.mu.Lock()
	s.senderEmail = senderEmail
	s.senderPassword = appPassword
	s.alertEmail = alertEmail
	s.mu.Unlock()

	log.Println("[SETTINGS] Email credentials permanently configured.")
	c.Redirect(http.StatusSeeOther, "/Frontend/home.html")
}

func (s *Server) HandleStartWebcam(c *gin.Context) {
	s.mu.Lock()
	if s.isStreaming {
		s.mu.Unlock()
		c.Redirect(http.StatusSeeOther, "/Frontend/surveillance.html")
		return
	}
	s.mu.Unlock()

	webcam, err := gocv.VideoCaptureDeviceWithAPI(0, 700)
	if err != nil || !webcam.IsOpened() {
		webcam, err = gocv.VideoCaptureDevice(0)
	}

	if err != nil || !webcam.IsOpened() {
		log.Printf("[CAMERA ERROR] Failed to open webcam: %v\n", err)
		c.String(http.StatusInternalServerError, fmt.Sprintf("Did not start Webcam: %v", err))
		return
	}

	go s.RunSurveillance(webcam)

	c.Redirect(http.StatusSeeOther, "/Frontend/surveillance.html")
}
