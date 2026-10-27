package main

import (
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
	"gocv.io/x/gocv"
)

const (
	dbConnStr      = "postgres://postgres:240907@localhost:5434/watchmanx?sslmode=disable"
	matchThreshold = 0.38
	cascadePath    = "D:/Bugatti/Watchman AI/Cascades/haarcascade_frontalface_default.xml"
	faceModelProto = "D:/Bugatti/Watchman AI/Cascades/arcfaceresnet100-8.onnx"
	serverPort     = ":8080"
)

type Server struct {
	db             *sql.DB
	net            gocv.Net
	detector       gocv.CascadeClassifier
	isStreaming    bool
	latestFrame    []byte
	senderEmail    string
	senderPassword string
	alertEmail     string
	lastAlertTime  time.Time
	mu             sync.Mutex
}

func NewServer(db *sql.DB) (*Server, error) {
	detector := gocv.NewCascadeClassifier()
	if !detector.Load(cascadePath) {
		return nil, fmt.Errorf("failed to load cascade classifier from %s", cascadePath)
	}

	net := gocv.ReadNetFromONNX(faceModelProto)
	if net.Empty() {
		return nil, fmt.Errorf("failed to load face recognition ONNX model from %s", faceModelProto)
	}

	s := &Server{
		db:       db,
		net:      net,
		detector: detector,
	}

	var sender, password, alert string
	query := `SELECT sender_email, app_password, alert_email FROM system_settings WHERE id = 1;`
	err := db.QueryRow(query).Scan(&sender, &password, &alert)
	if err == nil {
		s.senderEmail = sender
		s.senderPassword = password
		s.alertEmail = alert
		log.Printf("[SETTINGS LOADED] Auto-loaded email configuration for: %s -> %s\n", sender, alert)
	} else {
		log.Println("[SETTINGS NOTICE] No email settings found in database. Configure once in /email_config.html")
	}

	return s, nil
}

func main() {
	db, err := sql.Open("postgres", dbConnStr)
	if err != nil {
		log.Fatalf("PostgreSQL connection error: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("Database ping failed: %v", err)
	}
	log.Println("[PostgreSQL] Connected successfully.")

	server, err := NewServer(db)
	if err != nil {
		log.Fatalf("Server initialization failed: %v", err)
	}
	defer server.detector.Close()
	defer server.net.Close()

	router := gin.Default()

	// Static Assets
	router.Static("/Frontend", "D:/Bugatti/Watchman AI/Frontend")
	router.StaticFile("/", "D:/Bugatti/Watchman AI/Frontend/login.html")
	router.StaticFile("/login.html", "D:/Bugatti/Watchman AI/Frontend/login.html")
	router.StaticFile("/signup.html", "D:/Bugatti/Watchman AI/Frontend/signup.html")
	router.StaticFile("/instructions.html", "D:/Bugatti/Watchman AI/Frontend/instructions.html")

	router.POST("/signup", server.HandleSignup)
	router.POST("/login", server.HandleLogin)
	router.GET("/logout", server.HandleLogout)

	protected := router.Group("/")
	protected.Use(AuthRequired())
	{
		protected.StaticFile("/home.html", "D:/Bugatti/Watchman AI/Frontend/home.html")
		protected.StaticFile("/add_face.html", "D:/Bugatti/Watchman AI/Frontend/add_face.html")
		protected.StaticFile("/source_selector.html", "D:/Bugatti/Watchman AI/Frontend/source_selector.html")
		protected.StaticFile("/email_config.html", "D:/Bugatti/Watchman AI/Frontend/email_config.html")
		protected.StaticFile("/surveillance.html", "D:/Bugatti/Watchman AI/Frontend/surveillance.html")

		protected.POST("/api/scan-and-register", server.HandleScanAndRegister)
		protected.POST("/api/configure-email", server.HandleConfigureEmail)
		protected.POST("/api/select-source", server.HandleStartWebcam)
		protected.GET("/api/stop-surveillance", server.HandleStopSurveillance)

		protected.GET("/api/video-feed", server.HandleVideoFeed)
	}

	log.Printf("[WatchmanX] Server running on http://localhost%s\n", serverPort)
	if err := router.Run(serverPort); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
