package types

type DeleteReqStruct struct {
	ATTEMPT_ID string `json:"attemptId" binding:"required"`
}
