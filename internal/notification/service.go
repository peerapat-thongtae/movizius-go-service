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

// SendTodayAiringTV finds all users who have shows airing today in their watchlist
// and sends each user a personalised FCM notification listing those shows.
func (s *NotificationService) SendTodayAiringTV(ctx context.Context) (*TodayAiringResult, error) {
	userShows, err := s.repo.FindUsersWithAiringToday(ctx)
	if err != nil {
		return nil, fmt.Errorf("notification service: airing today: %w", err)
	}
	result := &TodayAiringResult{UsersNotified: len(userShows)}
	if len(userShows) == 0 {
		return result, nil
	}

	userIDs := make([]string, len(userShows))
	for i, u := range userShows {
		userIDs[i] = u.UserID
	}

	deviceMap, err := s.repo.FindDevicesByUserIDs(ctx, userIDs)
	if err != nil {
		return nil, fmt.Errorf("notification service: fetch devices: %w", err)
	}

	client, err := s.firebaseApp.Messaging(ctx)
	if err != nil {
		return nil, fmt.Errorf("notification service: messaging client: %w", err)
	}

	for _, u := range userShows {
		tokens := deviceMap[u.UserID]
		if len(tokens) == 0 {
			continue
		}

		total := len(u.Shows)
		title := fmt.Sprintf("%d series from your watchlist have new episodes today.", total)
		var body string
		switch total {
		case 1:
			body = fmt.Sprintf("%s is airing today.", u.Shows[0].Name)
		case 2:
			body = fmt.Sprintf("%s and %s are airing today.", u.Shows[0].Name, u.Shows[1].Name)
		default:
			body = fmt.Sprintf("%s, %s, and %d more are airing today.", u.Shows[0].Name, u.Shows[1].Name, total-2)
		}

		msg := &messaging.MulticastMessage{
			Tokens: tokens,
			Notification: &messaging.Notification{
				Title: title,
				Body:  body,
			},
			Data: map[string]string{"type": "tv_airing_today"},
		}
		resp, err := client.SendEachForMulticast(ctx, msg)
		if err != nil {
			result.Failed += len(tokens)
			continue
		}
		result.Succeeded += resp.SuccessCount
		result.Failed += resp.FailureCount
	}
	return result, nil
}

// SendTodayReleasingMovies finds all users who have movies releasing today in their watchlist
// and sends each user a personalised FCM notification listing those movies.
func (s *NotificationService) SendTodayReleasingMovies(ctx context.Context) (*TodayAiringResult, error) {
	userMovies, err := s.repo.FindUsersWithMovieReleasingToday(ctx)
	if err != nil {
		return nil, fmt.Errorf("notification service: movie releasing today: %w", err)
	}
	result := &TodayAiringResult{UsersNotified: len(userMovies)}
	if len(userMovies) == 0 {
		return result, nil
	}

	userIDs := make([]string, len(userMovies))
	for i, u := range userMovies {
		userIDs[i] = u.UserID
	}

	deviceMap, err := s.repo.FindDevicesByUserIDs(ctx, userIDs)
	if err != nil {
		return nil, fmt.Errorf("notification service: fetch devices: %w", err)
	}

	client, err := s.firebaseApp.Messaging(ctx)
	if err != nil {
		return nil, fmt.Errorf("notification service: messaging client: %w", err)
	}

	for _, u := range userMovies {
		tokens := deviceMap[u.UserID]
		if len(tokens) == 0 {
			continue
		}

		total := len(u.Shows)
		title := fmt.Sprintf("%d movies in your watchlist are releasing today.", total)
		var body string
		switch total {
		case 1:
			body = fmt.Sprintf("%s is releasing today.", u.Shows[0].Name)
		case 2:
			body = fmt.Sprintf("%s and %s are releasing today.", u.Shows[0].Name, u.Shows[1].Name)
		default:
			body = fmt.Sprintf("%s, %s, and %d more are releasing today.", u.Shows[0].Name, u.Shows[1].Name, total-2)
		}

		msg := &messaging.MulticastMessage{
			Tokens: tokens,
			Notification: &messaging.Notification{
				Title: title,
				Body:  body,
			},
			Data: map[string]string{"type": "movie_releasing_today"},
		}
		resp, err := client.SendEachForMulticast(ctx, msg)
		if err != nil {
			result.Failed += len(tokens)
			continue
		}
		result.Succeeded += resp.SuccessCount
		result.Failed += resp.FailureCount
	}
	return result, nil
}

// RegisterDevice persists a device FCM token for the given user.
func (s *NotificationService) RegisterDevice(ctx context.Context, userID string, req RegisterDeviceRequest) error {
	device := NotificationDevice{
		UserID:   userID,
		DeviceID: req.DeviceID,
		FCMToken: req.FCMToken,
		Platform: req.Platform,
	}
	if err := s.repo.UpsertDevice(ctx, device); err != nil {
		return fmt.Errorf("notification service: register device: %w", err)
	}
	return nil
}
