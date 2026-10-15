package http

import "github.com/gin-gonic/gin"

const sessionCookie = "lanverse_session"

func cookieToken(c *gin.Context) string { token, _ := c.Cookie(sessionCookie); return token }
