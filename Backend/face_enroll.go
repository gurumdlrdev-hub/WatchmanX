package main

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pgvector/pgvector-go"
	"gocv.io/x/gocv"
)

func (s *Server) HandleScanAndRegister(c *gin.Context) {
	personName := strings.TrimSpace(c.PostForm("person_name"))
	details := strings.TrimSpace(c.PostForm("details"))

	if personName == "" || details == "" {
		c.String(http.StatusBadRequest, "Both Person Name and Details are required.")
		return
	}

	// Windows DirectShow Backend (API ID = 700)
	webcam, err := gocv.VideoCaptureDeviceWithAPI(0, 700)
	if err != nil || !webcam.IsOpened() {
		webcam, err = gocv.VideoCaptureDevice(0)
	}

	if err != nil || !webcam.IsOpened() {
		log.Printf("[ENROLL ERROR] Failed to access webcam: %v\n", err)
		c.String(http.StatusInternalServerError, "Failed to access webcam hardware.")
		return
	}
	defer webcam.Close()

	img := gocv.NewMat()
	defer img.Close()

	var (
		capturedFaceBytes []byte
		capturedEmbedding []float32
		faceFound         bool
	)

	for attempt := 0; attempt < 30; attempt++ {
		if ok := webcam.Read(&img); !ok || img.Empty() {
			time.Sleep(50 * time.Millisecond)
			continue
		}

		rects := s.detector.DetectMultiScale(img)
		if len(rects) > 0 {
			faceROI := img.Region(rects[0])
			embedding, err := s.ExtractEmbedding(faceROI)
			if err == nil {
				buf, encErr := gocv.IMEncode(".jpg", faceROI)
				if encErr == nil && buf != nil {
					capturedFaceBytes = make([]byte, len(buf.GetBytes()))
					copy(capturedFaceBytes, buf.GetBytes())
					buf.Close()

					capturedEmbedding = embedding
					faceFound = true
					faceROI.Close()
					break
				}
			}
			faceROI.Close()
		}
		time.Sleep(50 * time.Millisecond)
	}

	if !faceFound {
		c.String(http.StatusUnprocessableEntity, "No face detected. Please look straight at the camera.")
		return
	}

	vectorParam := pgvector.NewVector(capturedEmbedding)
	insertQuery := `
		INSERT INTO face_profiles (person_name, details, embedding, image_data)
		VALUES ($1, $2, $3, $4);
	`
	_, err = s.db.Exec(insertQuery, personName, details, vectorParam, capturedFaceBytes)
	if err != nil {
		c.String(http.StatusInternalServerError, "Database error: "+err.Error())
		return
	}

	log.Printf("[LIVE ENROLLED] %s [%s] successfully saved.\n", personName, details)
	c.Redirect(http.StatusSeeOther, "/Frontend/home.html")
}
