package middlewares

// 文件说明：这个文件承载 HTTP 横切逻辑。
// 实现方式：把鉴权、日志、跨域和限流等通用逻辑集中到中间件层。
// 这样做的好处是接口层更干净，公共策略也更容易统一维护。
import (
	apperrors "ToDoList/server/errors"
	"ToDoList/server/response"
	"ToDoList/server/service"
	"ToDoList/server/utils"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func AuthMiddleware(authService *service.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		lg := utils.CtxLogger(c)
		authz := c.GetHeader("Authorization")
		if !strings.HasPrefix(authz, "Bearer ") {
			response.Unauthorized(c, "没有授权token")
			c.Abort()
			return
		}

		tokenStr := strings.TrimSpace(strings.TrimPrefix(authz, "Bearer "))
		claims, err := utils.Parse(tokenStr)
		if err != nil {
			response.Unauthorized(c, "token已不可用")
			c.Abort()
			return
		}

		err = authService.ValidateClaims(c.Request.Context(), lg, claims)
		if err != nil {
			var ae *apperrors.Error
			if apperrors.As(err, &ae) {
				switch ae.Code {
				case apperrors.CodeUnauthorized:
					response.Unauthorized(c, ae.Message)
				case apperrors.CodeNotFound:
					response.NotFound(c, ae.Message)
				case apperrors.CodeInternal:
					response.Error(c, err)
				default:
					response.Error(c, err)
				}
			} else {
				lg.Error("auth_validate_claims", zap.Error(err))
				response.Error(c, err)
			}
			c.Abort()
			return
		}

		c.Set("uid", claims.UID)
		c.Set("username", claims.Username)
		c.Set("claims", claims)
		c.Next()
	}
}
