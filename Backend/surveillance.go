package main

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"log"
	"math"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pgvector/pgvector-go"
	"gocv.io/x/gocv"
)

func (s *Server) ExtractEmbedding(faceMat gocv.Mat) ([]float32, error) {
	blob := gocv.BlobFromImage(
		faceMat,
		1.0/127.5,
		image.Pt(112, 112),
		gocv.NewScalar(127.5, 127.5, 127.5, 0),
		true,
		false,
	)
	defer blob.Close()

	s.net.SetInput(blob, "")
	prob := s.net.Forward("")
	defer prob.Close()

	vec := make([]float32, 512)
	for i := 0; i < 512; i++ {
		vec[i] = prob.GetFloatAt(0, i)
	}

	var norm float64
	for _, val := range vec {
		norm += float64(val * val)
	}
	norm = math.Sqrt(norm)
	if norm > 0 {
		for i := range vec {
			vec[i] = float32(float64(vec[i]) / norm)
		}
	}

	return vec, nil
}

func (s *Server) MatchFace(ctx context.Context, embedding []float32) (string, string, bool) {
	var (
		personName string
		details    string
		distance   float64
	)

	query := `
		SELECT person_name, details, (embedding <=> $1) AS distance
		FROM face_profiles
		ORDER BY distance ASC
		LIMIT 1;
	`

	vectorParam := pgvector.NewVector(embedding)
	err := s.db.QueryRowContext(ctx, query, vectorParam).Scan(&personName, &details, &distance)

	log.Printf("[FACE MATCH TEST] Closest: %s | Distance: %.4f | Threshold: %.4f\n", personName, distance, matchThreshold)

	if err == nil && distance < matchThreshold {
		return personName, details, true
	}

	return "UNKNOWN STRANGER", "UNAUTHORIZED", false
}

func (s *Server) RunSurveillance(webcam *gocv.VideoCapture) {
	s.mu.Lock()
	s.isStreaming = true
	s.mu.Unlock()

	defer func() {
		webcam.Close()
		s.mu.Lock()
		s.isStreaming = false
		s.mu.Unlock()
	}()

	img := gocv.NewMat()
	defer img.Close()

	log.Println("[GoCV] Video pipeline processing frames for web stream.")

	for {
		s.mu.Lock()
		active := s.isStreaming
		s.mu.Unlock()
		if !active {
			break
		}

		if ok := webcam.Read(&img); !ok || img.Empty() {
			continue
		}

		rects := s.detector.DetectMultiScale(img)
		for _, r := range rects {
			faceROI := img.Region(r)

			embedding, err := s.ExtractEmbedding(faceROI)
			if err == nil {
				name, details, isKnown := s.MatchFace(context.Background(), embedding)

				if isKnown {
					boxColor := color.RGBA{R: 34, G: 197, B: 94, A: 0}
					cyanColor := color.RGBA{R: 56, G: 189, B: 248, A: 0}

					gocv.Rectangle(&img, r, boxColor, 2)
					gocv.PutText(&img, name, image.Pt(r.Min.X, r.Min.Y-26), gocv.FontHersheyPlain, 1.3, boxColor, 2)
					gocv.PutText(&img, details, image.Pt(r.Min.X, r.Min.Y-8), gocv.FontHersheyPlain, 1.0, cyanColor, 2)
				} else {
					redColor := color.RGBA{R: 239, G: 68, B: 68, A: 0}
					gocv.Rectangle(&img, r, redColor, 2)
					gocv.PutText(&img, name, image.Pt(r.Min.X, r.Min.Y-10), gocv.FontHersheyPlain, 1.2, redColor, 2)

					s.mu.Lock()
					canAlert := time.Since(s.lastAlertTime) > 5*time.Second
					targetEmail := s.alertEmail
					senderSet := s.senderEmail != ""

					if canAlert && targetEmail != "" && senderSet {
						s.lastAlertTime = time.Now()
					}
					s.mu.Unlock()

					if canAlert && targetEmail != "" && senderSet {
						capturedBuf, encErr := gocv.IMEncode(".jpg", faceROI)
						if encErr == nil && capturedBuf != nil {
							faceBytes := make([]byte, len(capturedBuf.GetBytes()))
							copy(faceBytes, capturedBuf.GetBytes())
							capturedBuf.Close()

							go func(recipient string, data []byte) {
								log.Printf("[SECURITY ALERT] Unknown face! Sending email to %s...\n", recipient)
								if err := s.SendStrangerAlert(recipient, data); err != nil {
									log.Printf("[EMAIL ERROR] Failed to send: %v\n", err)
								} else {
									log.Println("[EMAIL SUCCESS] Alert sent successfully to inbox!")
								}
							}(targetEmail, faceBytes)
						}
					}
				}
			}
			faceROI.Close()
		}

		frameBuf, encErr := gocv.IMEncode(".jpg", img)
		if encErr == nil && frameBuf != nil {
			bytes := frameBuf.GetBytes()
			s.mu.Lock()
			s.latestFrame = make([]byte, len(bytes))
			copy(s.latestFrame, bytes)
			s.mu.Unlock()
			frameBuf.Close()
		}

		time.Sleep(30 * time.Millisecond)
	}
}

// HandleVideoFeed streams live MJPEG multipart frames to the browser
func (s *Server) HandleVideoFeed(c *gin.Context) {
	c.Writer.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary=frame")
	c.Writer.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Writer.Header().Set("Connection", "keep-alive")

	ctx := c.Request.Context()

	for {
		select {
		case <-ctx.Done():
			log.Println("[SURVEILLANCE] Client disconnected. Shutting down webcam feed.")
			s.mu.Lock()
			s.isStreaming = false
			s.latestFrame = nil
			s.mu.Unlock()
			return
		default:
			s.mu.Lock()
			frame := s.latestFrame
			s.mu.Unlock()

			if len(frame) > 0 {
				header := fmt.Sprintf("--frame\r\nContent-Type: image/jpeg\r\nContent-Length: %d\r\n\r\n", len(frame))
				if _, err := c.Writer.Write([]byte(header)); err != nil {
					return
				}
				if _, err := c.Writer.Write(frame); err != nil {
					return
				}
				if _, err := c.Writer.Write([]byte("\r\n")); err != nil {
					return
				}
				c.Writer.Flush()
			}
			time.Sleep(33 * time.Millisecond)
		}
	}
}

func (s *Server) HandleStopSurveillance(c *gin.Context) {
	s.mu.Lock()
	s.isStreaming = false
	s.latestFrame = nil
	s.mu.Unlock()

	log.Println("[SURVEILLANCE] Camera stopped by user command.")
	c.Redirect(http.StatusSeeOther, "/Frontend/home.html")
}
