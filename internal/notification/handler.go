package notification

import (
	"encoding/json"
	"net/http"

	"github.com/peera/movizius-go-service/internal/shared/middleware"
	"github.com/peera/movizius-go-service/internal/shared/response"
)

var validPlatforms = map[string]bool{
	"ios":     true,
	"android": true,
	"web":     true,
}

// Handler exposes notification endpoints over HTTP.
type Handler struct {
	service *NotificationService
}

// NewHandler constructs a notification Handler.
func NewHandler(service *NotificationService) *Handler {
	return &Handler{service: service}
}

// RegisterRoutes binds notification routes onto the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux, auth func(http.Handler) http.Handler) {
	mux.Handle("POST /notification/devices", auth(http.HandlerFunc(h.RegisterDevice)))
	mux.Handle("POST /notification/test", auth(http.HandlerFunc(h.SendTest)))
}

// RegisterDevice registers or refreshes a device FCM token for push notifications.
//
//	@Summary		Register device token
//	@Description	Registers or updates a device FCM token for the authenticated user. Platform must be one of: ios, android, web.
//	@Tags			notification
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			body	body		RegisterDeviceRequest	true	"FCM token and platform"
//	@Success		201		{object}	map[string]string
//	@Failure		400		{object}	map[string]string
//	@Failure		401		{object}	map[string]string
//	@Failure		500		{object}	map[string]string
//	@Router			/notification/devices [post]
func (h *Handler) RegisterDevice(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req RegisterDeviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.FCMToken == "" {
		response.Error(w, http.StatusBadRequest, "fcm_token is required")
		return
	}
	if !validPlatforms[req.Platform] {
		response.Error(w, http.StatusBadRequest, "platform must be one of: ios, android, web")
		return
	}

	if err := h.service.RegisterDevice(r.Context(), userID, req); err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to register device")
		return
	}

	response.Success(w, http.StatusCreated, map[string]string{"message": "device registered"})
}

// SendTest sends a test FCM notification to all registered devices.
//
//	@Summary		Send test notification
//	@Description	Sends a test push notification to every device token in the collection.
//	@Tags			notification
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	notification.TestNotificationResult
//	@Failure		401	{object}	map[string]string
//	@Failure		500	{object}	map[string]string
//	@Router			/notification/test [post]
func (h *Handler) SendTest(w http.ResponseWriter, r *http.Request) {
	_, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	result, err := h.service.SendTestToAll(r.Context())
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to send test notification")
		return
	}

	response.Success(w, http.StatusOK, result)
}
