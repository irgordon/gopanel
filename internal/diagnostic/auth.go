package diagnostic

import "github.com/irgordon/gopanel/internal/auth"

func AuthFailureRecorder(recorder *Recorder) auth.FailureRecorder {
	return func(failure auth.DiagnosticFailure) string {
		record := recorder.Record(Input{
			Event:           failure.Event,
			Component:       "auth",
			PublicMessage:   failure.PublicMessage,
			TechnicalDetail: failure.TechnicalDetail,
			UserID:          failure.UserID,
			Action:          failure.Event,
			HTTPStatus:      failure.HTTPStatus,
		})
		return record.ID
	}
}
