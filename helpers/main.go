package helpers

import (
	"execution-engine/types"
	"github.com/gin-gonic/gin"
)

// For parsing {attemptId}
func ReqBody(c *gin.Context) (string, error) {
	var DeleteReq types.DeleteReqStruct
	if err := c.ShouldBindJSON(&DeleteReq); err != nil {
		return "", err
	}
	return DeleteReq.ATTEMPT_ID, nil
}
