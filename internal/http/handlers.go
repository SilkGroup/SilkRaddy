package http

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/cheatsnake/airstation/internal/pkg/sse"
	"github.com/cheatsnake/airstation/internal/station"
	"github.com/cheatsnake/airstation/internal/track"
	"github.com/golang-jwt/jwt/v5"
	"github.com/oklog/ulid/v2"
)

// ulidMake is a tiny adapter used by shortRand so callers do not need to
// import the ulid package.
func ulidMake() ulid.ULID { return ulid.Make() }

// handleHealth is a liveness probe — returns 200 once the process has started.
// It does not check downstream dependencies; that is what /readyz is for.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, map[string]string{"status": "ok"})
}

// handleReady is a readiness probe — returns 200 only when the storage backend
// is reachable. Used by Cloud Run / Kubernetes to gate traffic. Object-store
// and other downstream checks should be added here as those interfaces land.
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.storage.Ping(ctx); err != nil {
		jsonMessage(w, http.StatusServiceUnavailable, "storage unreachable: "+err.Error())
		return
	}
	jsonResponse(w, map[string]string{"status": "ready"})
}

const multipartChunkLimit = 64 * 1024 * 1024 // 64 MB
const copyBufferSize = 256 * 1024            // 256 KB

func (s *Server) handleHLSPlaylist(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "audio/mpegurl")
	// HLS playlists change every segment; never let a proxy or browser
	// cache them or listeners will loop the same window forever.
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")

	if !s.playbackState.IsPlaying {
		// Return 503 so listeners retry rather than treating an empty
		// playlist as a successful "no tracks" response.
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	fmt.Fprint(w, s.playbackState.PlaylistStr)
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Buffered so a briefly-slow consumer doesn't force the emitter to drop
	// its event on the floor (see sse.Emitter — sends are non-blocking).
	eventChan := make(chan *sse.Event, 16)
	s.eventsEmitter.Subscribe(eventChan)

	closeNotify := r.Context().Done()
	go func() {
		<-closeNotify
		s.eventsEmitter.Unsubscribe(eventChan)
		close(eventChan)
	}()

	// Send current number of listeners immediately
	countEvent := s.countListeners()
	fmt.Fprint(w, countEvent.Stringify())
	w.(http.Flusher).Flush()

	for {
		event, isOpen := <-eventChan
		if !isOpen {
			break
		}

		fmt.Fprint(w, event.Stringify())
		w.(http.Flusher).Flush()
	}
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	body, err := parseJSONBody[struct {
		Secret string `json:"secret"`
	}](r)
	if err != nil {
		jsonBadRequest(w, "Parsing request body failed.")
		return
	}

	isValidSecret := subtle.ConstantTimeCompare([]byte(body.Secret), []byte(s.config.SecretKey)) == 1
	if !isValidSecret {
		jsonForbidden(w, "Wrong secret, access denied.")
		return
	}

	expirationTime := time.Now().Add(7 * 24 * time.Hour)
	claims := jwt.MapClaims{
		"iss": "airstation",
		"exp": expirationTime.Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(s.config.JWTSign))
	if err != nil {
		s.logger.Debug("Failed to generate token: " + err.Error())
		jsonInternalError(w, "Failed to generate token.")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "jwt",
		Value:    tokenString,
		Expires:  expirationTime,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.config.SecureCookie,
		SameSite: http.SameSiteStrictMode,
	})

	s.logger.Info(fmt.Sprintf("New login succeed from %s with secureCookie=%v", r.Host, s.config.SecureCookie))

	jsonOK(w, "Login succeed.")
}

func (s *Server) handleTracks(w http.ResponseWriter, r *http.Request) {
	queries := r.URL.Query()
	page := parseIntQuery(queries, "page", 1)
	limit := parseIntQuery(queries, "limit", 20)
	search := queries.Get("search")
	sortBy := queries.Get("sort_by")
	sortOrder := queries.Get("sort_order")

	result, err := s.trackService.Tracks(page, limit, search, sortBy, sortOrder)
	if err != nil {
		jsonBadRequest(w, "Tracks retrieving failed: "+err.Error())
		return
	}

	jsonResponse(w, result)
}

func (s *Server) handleTracksUpload(w http.ResponseWriter, r *http.Request) {
	// Cap total request body size to prevent disk-fill DoS. Zero disables.
	if s.config.MaxUploadBytes > 0 {
		r.Body = http.MaxBytesReader(w, r.Body, s.config.MaxUploadBytes)
	}

	err := r.ParseMultipartForm(multipartChunkLimit)
	if err != nil {
		// Distinguish "too big" (413) from parse errors (400).
		var mbErr *http.MaxBytesError
		if errors.As(err, &mbErr) {
			jsonMessage(w, http.StatusRequestEntityTooLarge,
				fmt.Sprintf("Upload exceeds the %d-byte limit.", s.config.MaxUploadBytes))
			return
		}
		jsonBadRequest(w, "Failed to parse multipart form: "+err.Error())
		return
	}
	// Multipart parsing spills large parts to temp files; clean them up
	// regardless of success or failure below.
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()

	files := r.MultipartForm.File["tracks"]
	if len(files) == 0 {
		jsonBadRequest(w, "No files uploaded")
		return
	}

	saved := make([]string, 0, len(files))
	for _, fileHeader := range files {
		path, err := s.saveFile(fileHeader)
		if err != nil {
			// Roll back the partial success so a failed upload doesn't
			// litter the tracks directory with half-processed files.
			for _, p := range saved {
				_ = os.Remove(p)
			}
			s.logger.Warn("Track upload failed",
				"error", err,
				"filename", fileHeader.Filename,
				"saved_before_fail", len(saved))
			jsonBadRequest(w, err.Error())
			return
		}
		saved = append(saved, path)
	}

	go s.trackService.LoadTracksFromDisk(s.config.TracksDir)

	msg := fmt.Sprintf("%d track(s) uploaded successfully. They will be available in your library once processed.", len(files))
	jsonOK(w, msg)
}

func (s *Server) handleDeleteTracks(w http.ResponseWriter, r *http.Request) {
	body, err := parseJSONBody[track.BodyWithIDs](r)
	if err != nil {
		jsonBadRequest(w, "Parsing request body failed: "+err.Error())
		return
	}

	err = s.trackService.DeleteTracks(body.IDs)
	if err != nil {
		s.logger.Debug(err.Error())
		jsonBadRequest(w, "Deleting tracks failed")
		return
	}

	jsonOK(w, "Tracks deleted")
}

func (s *Server) handleQueue(w http.ResponseWriter, _ *http.Request) {
	queue, err := s.queueService.Queue()
	if err != nil {
		s.logger.Debug(err.Error())
		jsonBadRequest(w, "Queue retrieving failed: "+err.Error())
		return
	}

	jsonResponse(w, queue)
}

func (s *Server) handleAddToQueue(w http.ResponseWriter, r *http.Request) {
	body, err := parseJSONBody[track.BodyWithIDs](r)
	if err != nil {
		jsonBadRequest(w, "Parsing request body failed: "+err.Error())
		return
	}

	tracks, err := s.trackService.FindTracks(body.IDs)
	if err != nil {
		jsonBadRequest(w, "Adding tracks to queue failed: "+err.Error())
		return
	}

	err = s.queueService.AddToQueue(tracks)
	if err != nil {
		jsonBadRequest(w, "Adding tracks to queue failed: "+err.Error())
		return
	}

	err = s.playbackState.Reload()
	if err != nil {
		s.logger.Debug("Playback reload failed: " + err.Error())
	}

	jsonOK(w, "Tracks added")
}

func (s *Server) handleReorderQueue(w http.ResponseWriter, r *http.Request) {
	body, err := parseJSONBody[track.BodyWithIDs](r)
	if err != nil {
		jsonBadRequest(w, "Parsing request body failed: "+err.Error())
		return
	}

	err = s.queueService.ReorderQueue(body.IDs)
	if err != nil {
		jsonBadRequest(w, "Queue reordering failed: "+err.Error())
		return
	}

	err = s.playbackState.Reload()
	if err != nil {
		s.logger.Debug("Playback reload failed: " + err.Error())
	}

	jsonOK(w, "Queue reordered")
}

func (s *Server) handleRemoveFromQueue(w http.ResponseWriter, r *http.Request) {
	body, err := parseJSONBody[track.BodyWithIDs](r)
	if err != nil {
		jsonBadRequest(w, "Parsing request body failed: "+err.Error())
		return
	}

	if s.playbackState.CurrentTrack != nil {
		hasCurrent := slices.Contains(body.IDs, s.playbackState.CurrentTrack.ID)
		if hasCurrent {
			s.playbackState.Pause()
		}
	}

	err = s.queueService.RemoveFromQueue(body.IDs)
	if err != nil {
		jsonBadRequest(w, "Removing from queue failed: "+err.Error())
		return
	}

	err = s.playbackState.Reload()
	if err != nil {
		s.logger.Debug("Playback reload failed: " + err.Error())
	}

	jsonOK(w, "Tracks removed")
}

func (s *Server) handlePlaybackState(w http.ResponseWriter, _ *http.Request) {
	jsonResponse(w, s.playbackState)
}

func (s *Server) handlePausePlayback(w http.ResponseWriter, _ *http.Request) {
	s.playbackState.Pause()
	jsonResponse(w, s.playbackState)
}

func (s *Server) handlePlayPlayback(w http.ResponseWriter, _ *http.Request) {
	err := s.playbackState.Play()
	if err != nil {
		jsonBadRequest(w, "Playback failed to start: "+err.Error())
		return
	}

	jsonResponse(w, s.playbackState)
}

func (s *Server) handlePlaybackHistory(w http.ResponseWriter, r *http.Request) {
	queries := r.URL.Query()
	limit := parseIntQuery(queries, "limit", 50)
	history, err := s.playbackService.RecentPlaybackHistory(limit)
	if err != nil {
		s.logger.Debug(err.Error())
		jsonBadRequest(w, "Playback history retrieving failed")
		return
	}

	jsonResponse(w, history)
}

func (s *Server) handleAddPlaylist(w http.ResponseWriter, r *http.Request) {
	body, err := parseJSONBody[struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		TrackIDs    []string `json:"trackIDs"`
	}](r)
	if err != nil {
		jsonBadRequest(w, "Parsing request body failed: "+err.Error())
		return
	}

	pl, err := s.playlistService.AddPlaylist(body.Name, body.Description, body.TrackIDs)
	if err != nil {
		jsonBadRequest(w, "Playlist creation failed: "+err.Error())
		return
	}

	jsonResponse(w, pl)
}

func (s *Server) handlePlaylists(w http.ResponseWriter, r *http.Request) {
	pls, err := s.playlistService.Playlists()
	if err != nil {
		jsonBadRequest(w, "Playlists retrieving failed: "+err.Error())
		return
	}

	jsonResponse(w, pls)
}

func (s *Server) handlePlaylist(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	pl, err := s.playlistService.Playlist(id)
	if err != nil {
		jsonBadRequest(w, "Playlist retrieving failed: "+err.Error())
		return
	}

	jsonResponse(w, pl)
}

func (s *Server) handleEditPlaylist(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	body, err := parseJSONBody[struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		TrackIDs    []string `json:"trackIDs"`
	}](r)
	if err != nil {
		jsonBadRequest(w, "Parsing request body failed: "+err.Error())
		return
	}

	err = s.playlistService.EditPlaylist(id, body.Name, body.Description, body.TrackIDs)
	if err != nil {
		jsonBadRequest(w, "Playlist creation failed: "+err.Error())
		return
	}

	jsonOK(w, "Playlist updated")
}

func (s *Server) handleDeletePlaylist(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	err := s.playlistService.DeletePlaylist(id)
	if err != nil {
		jsonBadRequest(w, "Playlist deletion failed: "+err.Error())
		return
	}

	jsonOK(w, "Playlist deleted")
}

func (s *Server) handleStaticDir(prefix string, path string) http.Handler {
	return http.StripPrefix(prefix, http.FileServer(http.Dir(path)))
}

func (s *Server) handleStaticDirWithoutCache(prefix string, path string) http.Handler {
	fileHandler := http.StripPrefix(prefix, http.FileServer(http.Dir(path)))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		fileHandler.ServeHTTP(w, r)
	})
}

func (s *Server) handleStationInfo(w http.ResponseWriter, _ *http.Request) {
	info, err := s.stationService.Info()
	if err != nil {
		jsonBadRequest(w, "Failed to get station info: "+err.Error())
		return
	}

	jsonResponse(w, info)
}

func (s *Server) handleEditStationInfo(w http.ResponseWriter, r *http.Request) {
	body, err := parseJSONBody[station.Info](r)
	if err != nil {
		jsonBadRequest(w, "Parsing request body failed: "+err.Error())
		return
	}

	info, err := s.stationService.EditInfo(body)
	if err != nil {
		jsonBadRequest(w, "Station info editing failed: "+err.Error())
		return
	}

	s.eventsEmitter.RegisterEvent(eventChangeTheme, " ")

	jsonResponse(w, info)
}

func (s *Server) saveFile(fileHeader *multipart.FileHeader) (string, error) {
	file, err := fileHeader.Open()
	if err != nil {
		return "", fmt.Errorf("open uploaded file: %w", err)
	}
	defer file.Close()

	// filepath.Base strips any directory component, which stops the classic
	// "../../etc/passwd" traversal attack. Rejecting names that resolve to
	// "." or "/" or that are otherwise empty guards against oddball edge
	// cases the base cleanup does not catch.
	fileName := safeUploadName(fileHeader.Filename)
	if fileName == "" {
		return "", fmt.Errorf("uploaded file has no usable name")
	}

	// Avoid overwriting an existing library file. If a name collides,
	// append a short random suffix before the extension.
	filePath := filepath.Join(s.config.TracksDir, fileName)
	filePath = uniquePath(filePath)

	dst, err := os.Create(filePath)
	if err != nil {
		return "", fmt.Errorf("create file on disk: %w", err)
	}

	if _, err = io.CopyBuffer(dst, file, make([]byte, copyBufferSize)); err != nil {
		// Best-effort cleanup so a half-written file isn't left behind.
		_ = dst.Close()
		_ = os.Remove(filePath)
		return "", fmt.Errorf("copy upload body: %w", err)
	}
	if err := dst.Close(); err != nil {
		_ = os.Remove(filePath)
		return "", fmt.Errorf("close file: %w", err)
	}

	return filePath, nil
}

// safeUploadName sanitises an untrusted multipart filename. It strips any
// path component and rejects names that would otherwise be no-ops on disk.
func safeUploadName(raw string) string {
	name := filepath.Base(raw)
	// filepath.Base returns "." for empty input and "/" for a bare slash.
	if name == "." || name == string(filepath.Separator) || name == "" {
		return ""
	}
	return name
}

// uniquePath appends a short random suffix before the extension when the
// target path already exists, so concurrent uploads with the same filename
// don't overwrite one another.
func uniquePath(path string) string {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return path
	}
	ext := filepath.Ext(path)
	stem := strings.TrimSuffix(path, ext)
	// Try a handful of suffixes; if all collide (extremely unlikely),
	// return the last candidate and let the caller fail loudly.
	for i := 0; i < 8; i++ {
		candidate := stem + "-" + shortRand() + ext
		if _, err := os.Stat(candidate); errors.Is(err, os.ErrNotExist) {
			return candidate
		}
	}
	return stem + "-" + shortRand() + ext
}

// shortRand returns a 6-character alphanumeric random suffix. Not
// cryptographic — just enough to disambiguate two uploads with the same
// stem in the same second.
func shortRand() string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	buf := make([]byte, 6)
	// Use ulid for entropy since we already depend on it elsewhere.
	id := ulidMake()
	for i := 0; i < 6; i++ {
		buf[i] = alphabet[int(id[i])%len(alphabet)]
	}
	return string(buf)
}
