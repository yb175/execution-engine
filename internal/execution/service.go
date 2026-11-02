package execution

import (
	"execution-engine/internal/workspace"

	"github.com/google/uuid"
)

type Service struct {
	workspaces    workspace.Manager
	challengePath string
}

func NewService(workspaces workspace.Manager, challengePath string) *Service {
	return &Service{workspaces: workspaces, challengePath: challengePath}
}

func (s *Service) CreateAttempt() (string, error) {
	attemptID := uuid.NewString()
	if err := s.workspaces.CreateAttempt(attemptID, s.challengePath); err != nil {
		return "", err
	}
	return attemptID, nil
}

func (s *Service) DeleteAttempt(attemptID string) error {
	return s.workspaces.DeleteAttempt(attemptID)
}
