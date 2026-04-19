package handler

import (
	"ToDoList/server/service"
	"ToDoList/server/utils"
	"strings"

	"github.com/gin-gonic/gin"
)

func authenticateWebSocket(c *gin.Context, authSvc *service.AuthService) (*utils.Claims, error) {
	token := strings.TrimSpace(c.Query("token"))
	if token == "" {
		authz := c.GetHeader("Authorization")
		if strings.HasPrefix(authz, "Bearer ") {
			token = strings.TrimSpace(strings.TrimPrefix(authz, "Bearer "))
		}
	}

	claims, err := utils.Parse(token)
	if err != nil {
		return nil, err
	}
	if err := authSvc.ValidateClaims(c.Request.Context(), utils.CtxLogger(c), claims); err != nil {
		return nil, err
	}
	return claims, nil
}
