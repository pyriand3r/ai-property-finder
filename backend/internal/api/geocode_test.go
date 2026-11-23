package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGeocodeHandler_Get_MissingAddressIs400(t *testing.T) {
	for name, target := range map[string]string{
		"absent": "/api/geocode",
		"blank":  "/api/geocode?address=%20%20",
	} {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()

			(&GeocodeHandler{}).Get(recorder, httptest.NewRequest(http.MethodGet, target, http.NoBody))

			if recorder.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400; body %s", recorder.Code, recorder.Body)
			}
		})
	}
}
