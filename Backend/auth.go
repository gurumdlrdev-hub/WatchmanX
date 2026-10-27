package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

var jwtSecretKey = []byte("WATCHMANX_SECURE_CYBER_SECRET_KEY_2026")

type Claims struct {
	Username string `json:"username"`
	jwt.RegisteredClaims
}

func GenerateJWT(username string) (string, error) {
	expirationTime := time.Now().Add(24 * time.Hour)
	claims := &Claims{
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "WatchmanX-Core",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecretKey)
}

func AuthRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString, err := c.Cookie("auth_token")
		if err != nil || tokenString == "" {
			c.Redirect(http.StatusSeeOther, "/Frontend/login.html")
			c.Abort()
			return
		}

		claims := &Claims{}
		token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return jwtSecretKey, nil
		})

		if err != nil || !token.Valid {
			c.SetCookie("auth_token", "", -1, "/", "", false, true)
			c.Redirect(http.StatusSeeOther, "/Frontend/login.html")
			c.Abort()
			return
		}

		c.Set("username", claims.Username)
		c.Next()
	}
}

func (s *Server) HandleSignup(c *gin.Context) {
	username := c.PostForm("username")
	password := c.PostForm("password")

	if username == "" || password == "" {
		c.String(http.StatusBadRequest, "Username and password are required")
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		c.String(http.StatusInternalServerError, "Error securing password")
		return
	}

	query := `INSERT INTO operators (username, password_hash) VALUES ($1, $2);`
	_, err = s.db.Exec(query, username, string(hashedPassword))
	if err != nil {
		c.String(http.StatusConflict, "Username already exists or database error")
		return
	}

	c.Redirect(http.StatusSeeOther, "/Frontend/login.html")
}

func (s *Server) HandleLogin(c *gin.Context) {
	username := c.PostForm("username")
	password := c.PostForm("password")

	if username == "" || password == "" {
		c.String(http.StatusBadRequest, "Username and password required")
		return
	}

	var storedHash string
	query := `SELECT password_hash FROM operators WHERE username = $1;`
	err := s.db.QueryRow(query, username).Scan(&storedHash)
	if err != nil {
		log.Printf("[LOGIN ERROR] User not found: %s", username)
		c.String(http.StatusUnauthorized, "Invalid credentials")
		return
	}

	err = bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(password))
	if err != nil {
		log.Printf("[LOGIN ERROR] Password mismatch for: %s", username)
		c.String(http.StatusUnauthorized, "Invalid credentials")
		return
	}

	token, err := GenerateJWT(username)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to generate security token")
		return
	}

	c.SetCookie("auth_token", token, 86400, "/", "", false, true)

	c.Redirect(http.StatusSeeOther, "/Frontend/home.html")
}

func (s *Server) HandleLogout(c *gin.Context) {
	c.SetCookie("auth_token", "", -1, "/", "", false, true)
	c.Redirect(http.StatusSeeOther, "/Frontend/login.html")
}
