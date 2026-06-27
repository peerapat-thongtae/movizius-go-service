package notification

import (
	"context"
	"fmt"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
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

// TestNotificationResult summarises the outcome of a test broadcast.
type TestNotificationResult struct {
	Total     int `json:"total"`
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
}

// SendTestToAll sends a test FCM notification to every device in the collection.
func (s *NotificationService) SendTestToAll(ctx context.Context) (*TestNotificationResult, error) {
	devices, err := s.repo.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("notification service: send test: %w", err)
	}

	result := &TestNotificationResult{Total: len(devices)}
	if len(devices) == 0 {
		return result, nil
	}

	client, err := s.firebaseApp.Messaging(ctx)
	if err != nil {
		return nil, fmt.Errorf("notification service: messaging client: %w", err)
	}

	tokens := make([]string, len(devices))
	for i, d := range devices {
		tokens[i] = d.FCMToken
	}

	msg := &messaging.MulticastMessage{
		Tokens: tokens,
		Notification: &messaging.Notification{
			Title: "Test Notification",
			Body:  "This is a test notification from Movizius.",
		},
	}

	resp, err := client.SendEachForMulticast(ctx, msg)
	if err != nil {
		return nil, fmt.Errorf("notification service: send multicast: %w", err)
	}

	result.Succeeded = resp.SuccessCount
	result.Failed = resp.FailureCount
	return result, nil
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
