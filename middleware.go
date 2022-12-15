package main

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt"
)

// func OptionsMiddleWare(c *gin.Context) {
// 	if c.Request.Method != "OPTIONS" {
// 		c.Next()
// 	} else {
// 		c.Header("Access-Control-Allow-Origin", "*")
// 		c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
// 		c.Header("Access-Control-Allow-Headers", "authorization, origin, content-type, accept, X-API-KEY, x-api-key")
// 		c.Header("Allow", "HEAD,GET,POST,PUT,PATCH,DELETE,OPTIONS")
// 		c.Header("Content-Type", "application/json")
// 		c.AbortWithStatus(http.StatusOK)
// 	}
// }

func requireAuth(c *gin.Context) {
	tokenString := c.Request.Header.Get("X-API-KEY")
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			c.AbortWithStatus(http.StatusUnauthorized)
		}
		return []byte(secretKEY), nil
	})

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		if float64(time.Now().Unix()) > claims["expiry"].(float64) {
			c.AbortWithStatus(http.StatusUnauthorized)
		}
		//db := openDB()
		c.Set("userID", uint(claims["sub"].(float64)))
	} else {
		fmt.Println(err)
	}

	c.Next()
}
func googleAuth(c *gin.Context) {
	tokenString := c.Query("state")
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			c.AbortWithStatus(http.StatusUnauthorized)
		}
		return []byte(secretKEY), nil
	})

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		if float64(time.Now().Unix()) > claims["expiry"].(float64) {
			c.AbortWithStatus(http.StatusUnauthorized)
		}
		//db := openDB()
		c.Set("userID", uint(claims["sub"].(float64)))
	} else {
		fmt.Println(err)
	}

	c.Next()
}
func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With, x-api-key")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
