package notification

import (
	"context"
	"fmt"

	firebase "firebase.google.com/go/v4"
)

// NotificationService holds the business logic for the notification feature.
type NotificationService struct {
	repo        NotificationRepository
	firebaseApp *firebase.App
}

// NewService constructs a NotificationService.
// firebaseApp is stored for future push-sending methods.
func NewService(repo NotificationRepository, firebaseApp *firebase.App) *NotificationService {
	return &NotificationService{repo: repo, firebaseApp: firebaseApp}
}

// RegisterDevice persists a device FCM token for the given user.
func (s *NotificationService) RegisterDevice(ctx context.Context, userID string, req RegisterDeviceRequest) error {
	device := NotificationDevice{
		UserID:   userID,
		FCMToken: req.FCMToken,
		Platform: req.Platform,
	}
	if err := s.repo.UpsertDevice(ctx, device); err != nil {
		return fmt.Errorf("notification service: register device: %w", err)
	}
	return nil
}
