package handlers

import "execution-engine/internal/execution"

type AttemptHandlers struct {
	service *execution.Service
}

func NewAttemptHandlers(service *execution.Service) *AttemptHandlers {
	return &AttemptHandlers{service: service}
}

func (h *AttemptHandlers) CreateAttempt() (string, error) {
	return h.service.CreateAttempt()
}

func (h *AttemptHandlers) DeleteAttempt(attemptID string) error {
	return h.service.DeleteAttempt(attemptID)
}
