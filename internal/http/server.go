package http

import (
	"context"
	"errors"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cheatsnake/airstation/internal/config"
	"github.com/cheatsnake/airstation/internal/pkg/ffmpeg"
	"github.com/cheatsnake/airstation/internal/pkg/hls"
	"github.com/cheatsnake/airstation/internal/pkg/sse"
	"github.com/cheatsnake/airstation/internal/playback"
	"github.com/cheatsnake/airstation/internal/playlist"
	"github.com/cheatsnake/airstation/internal/queue"
	"github.com/cheatsnake/airstation/internal/station"
	"github.com/cheatsnake/airstation/internal/storage"
	"github.com/cheatsnake/airstation/internal/track"
	"github.com/rs/cors"
)

type Server struct {
	playbackState   *playback.State
	eventsEmitter   *sse.Emitter
	trackService    *track.Service
	queueService    *queue.Service
	playbackService *playback.Service
	playlistService *playlist.Service
	stationService  *station.Service
	storage         storage.Storage
	config          *config.Config
	logger          *slog.Logger
	router          *http.ServeMux
	httpServer      *http.Server
	loginLimiter    *tokenBucket
}

func NewServer(store storage.Storage, conf *config.Config, logger *slog.Logger) *Server {
	ffmpegCLI := ffmpeg.NewCLI()
	ts := track.NewService(store, ffmpegCLI, logger.WithGroup("trackservice"))
	qs := queue.NewService(store)
	ps := playback.NewService(store)
	pls := playlist.NewService(store)
	ss := station.NewService(store)
	state := playback.NewState(ts, qs, ps, conf.TmpDir, logger.WithGroup("playback"))

	router := http.NewServeMux()
	emitter := sse.NewEmitter()
	emitter.SetLogger(logger.WithGroup("sse"))
	s := &Server{
		playbackState:   state,
		eventsEmitter:   emitter,
		trackService:    ts,
		queueService:    qs,
		playbackService: ps,
		playlistService: pls,
		stationService:  ss,
		storage:         store,
		config:          conf,
		logger:          logger.WithGroup("http"),
		router:          router,
	}
	s.loginLimiter = newTokenBucket(
		float64(maxInt(conf.LoginRateBurst, 1)),
		float64(maxInt(conf.LoginRateRefillPerMin, 1))/60.0,
	)
	s.httpServer = &http.Server{
		Addr:    ":" + conf.HTTPPort,
		Handler: requestID(configuredCORS(conf).Handler(router)),
	}
	return s
}

// maxInt returns the larger of a and b, or 1 if both are non-positive. Used
// to guarantee the rate limiter has non-zero parameters even if config
// values are misconfigured.
func maxInt(a, b int) int {
	if a > b {
		if a > 0 {
			return a
		}
		return 1
	}
	if b > 0 {
		return b
	}
	return 1
}

// configuredCORS returns the CORS handler. If SILKRADDY_CORS_ORIGINS is
// empty, we preserve the legacy permissive default so existing self-host
// deploys are unaffected. In production, set the env to a comma-separated
// origin allowlist.
func configuredCORS(conf *config.Config) *cors.Cors {
	if conf.CORSAllowedOrigins == "" {
		return cors.Default()
	}
	origins := splitAndTrim(conf.CORSAllowedOrigins, ",")
	return cors.New(cors.Options{
		AllowedOrigins:   origins,
		AllowCredentials: true,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type", "Authorization", "X-Request-Id"},
	})
}

func splitAndTrim(s, sep string) []string {
	parts := strings.Split(s, sep)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (s *Server) Run() {
	s.registerMP2TMimeType()

	// Health probes (public, no auth, no logging noise expected)
	s.router.HandleFunc("GET /healthz", s.handleHealth)
	s.router.HandleFunc("GET /readyz", s.handleReady)

	// Public handlers
	s.router.HandleFunc("GET /stream", s.handleHLSPlaylist)
	s.router.HandleFunc("GET /api/v1/events", s.handleEvents)
	s.router.HandleFunc("GET /api/v1/station/info", s.handleStationInfo)
	s.router.Handle("POST /api/v1/login", s.rateLimit(http.HandlerFunc(s.handleLogin), s.loginLimiter, clientIP))
	s.router.Handle("GET /static/tmp/", s.handleStaticDirWithoutCache("/static/tmp", s.config.TmpDir))
	s.router.Handle("GET /api/v1/playback", http.HandlerFunc(s.handlePlaybackState))
	s.router.Handle("GET /api/v1/playback/history", http.HandlerFunc(s.handlePlaybackHistory))

	// Protected handlers
	s.router.Handle("POST /api/v1/tracks", s.jwtAuth(http.HandlerFunc(s.handleTracksUpload)))
	s.router.Handle("GET /api/v1/tracks", s.jwtAuth(http.HandlerFunc(s.handleTracks)))
	s.router.Handle("DELETE /api/v1/tracks", s.jwtAuth(http.HandlerFunc(s.handleDeleteTracks)))
	s.router.Handle("GET /api/v1/queue", s.jwtAuth(http.HandlerFunc(s.handleQueue)))
	s.router.Handle("POST /api/v1/queue", s.jwtAuth(http.HandlerFunc(s.handleAddToQueue)))
	s.router.Handle("PUT /api/v1/queue", s.jwtAuth(http.HandlerFunc(s.handleReorderQueue)))
	s.router.Handle("DELETE /api/v1/queue", s.jwtAuth(http.HandlerFunc(s.handleRemoveFromQueue)))
	s.router.Handle("POST /api/v1/playback/pause", s.jwtAuth(http.HandlerFunc(s.handlePausePlayback)))
	s.router.Handle("POST /api/v1/playback/play", s.jwtAuth(http.HandlerFunc(s.handlePlayPlayback)))
	s.router.Handle("POST /api/v1/playlist", s.jwtAuth(http.HandlerFunc(s.handleAddPlaylist)))
	s.router.Handle("GET /api/v1/playlists", s.jwtAuth(http.HandlerFunc(s.handlePlaylists)))
	s.router.Handle("GET /api/v1/playlist/{id}/", s.jwtAuth(http.HandlerFunc(s.handlePlaylist)))
	s.router.Handle("PUT /api/v1/playlist/{id}/", s.jwtAuth(http.HandlerFunc(s.handleEditPlaylist)))
	s.router.Handle("DELETE /api/v1/playlist/{id}/", s.jwtAuth(http.HandlerFunc(s.handleDeletePlaylist)))
	s.router.Handle("GET /static/tracks/", s.jwtAuth(s.handleStaticDir("/static/tracks", s.config.TracksDir)))
	s.router.Handle("PUT /api/v1/station/info", s.jwtAuth(http.HandlerFunc(s.handleEditStationInfo)))

	s.router.Handle("GET /studio/", s.handleStaticDir("/studio/", s.config.StudioDir))
	s.router.Handle("GET /", s.handleStaticDir("/", s.config.PlayerDir))

	s.listenEvents()

	err := s.playbackState.Play()
	if err != nil {
		s.logger.Warn("Auto start playing failed: " + err.Error())
	}

	go s.playbackState.Run()
	go s.trackService.LoadTracksFromDisk(s.config.TracksDir)
	s.playbackService.DeleteOldPlaybackHistory()

	s.logger.Info("Server starts on http://localhost:" + s.config.HTTPPort)
	err = s.httpServer.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		s.logger.Error("Listen and serve failed", slog.String("info", err.Error()))
	}
}

// Shutdown gracefully stops the HTTP server, allowing in-flight requests to
// finish until ctx is cancelled. Cloud Run sends SIGTERM with a 10s grace
// window before SIGKILL, so callers should pass a context with a slightly
// shorter timeout.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) registerMP2TMimeType() {
	err := mime.AddExtensionType(hls.SegmentExtension, "video/mp2t")
	if err != nil {
		s.logger.Error("MP2T mime type registration failed", slog.String("info", err.Error()))
	}
}

func (s *Server) countListeners() *sse.Event {
	count := s.eventsEmitter.CountSubscribers()
	return sse.NewEvent(eventCountListeners, strconv.Itoa(count))
}

func (s *Server) listenEvents() {
	countConnectionTicker := time.Tick(5 * time.Second)

	// TODO: add context for gracefull shutdown

	go func() {
		for range countConnectionTicker {
			event := s.countListeners()
			s.eventsEmitter.RegisterEvent(event.Name, event.Data)
		}
	}()

	go func() {
		for {
			select {
			case <-s.playbackState.PlayNotify:
				s.eventsEmitter.RegisterEvent(eventPlay, s.playbackState.CurrentTrack.Name)
			case <-s.playbackState.PauseNotify:
				s.eventsEmitter.RegisterEvent(eventPause, " ")
			case trackName := <-s.playbackState.NewTrackNotify:
				s.eventsEmitter.RegisterEvent(eventNewTrack, trackName)
			case loadedTracks := <-s.trackService.LoadedTracksNotify:
				s.eventsEmitter.RegisterEvent(eventLoadedTracks, strconv.Itoa(loadedTracks))
			}
		}
	}()
}
