package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"ness-to-odoo-golang-validation-api-tool/utils"
)

// Logger is a middleware that logs incoming requests and their responses
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get the logger
		//logger := utils.GetLogger()

		// Start timer
		start := time.Now()
		path := c.Request.URL.Path
		method := c.Request.Method

		// Log request
		params := make(map[string]string)
		for _, param := range c.Params {
			params[param.Key] = param.Value
		}
		utils.LogRequest(method, path, params)

		// Process request
		c.Next()

		// Log response
		statusCode := c.Writer.Status()
		duration := time.Since(start)
		utils.LogResponse(path, statusCode, duration)
	}
}
