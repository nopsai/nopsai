package app

import "github.com/rs/zerolog"

type StepSessionCleanupRequest struct {
	Sessions []StepSession
	Reason   string
	Logger   *zerolog.Logger
	Cleanup  func(StepSession)
}

func CleanupStepSessions(req StepSessionCleanupRequest) {
	if req.Cleanup == nil {
		return
	}
	for _, session := range req.Sessions {
		if req.Logger != nil {
			req.Logger.Info().Str("session", session.Name).Str("reason", req.Reason).Msg("Cleaning up session container")
		}
		req.Cleanup(session)
	}
}
