package health

import "context"

// Service holds the business logic for health checks.
type Service struct{}

// NewService constructs a health Service.
func NewService() *Service {
	return &Service{}
}

// Status reports the current health of the service.
func (s *Service) Status(ctx context.Context) Status {
	return Status{Status: "ok"}
}
