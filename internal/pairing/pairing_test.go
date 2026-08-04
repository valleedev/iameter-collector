package pairing

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/iameter/collector/internal/httpclient"
)

func testDevice() DeviceInfo {
	return DeviceInfo{Name: "test-device", OS: "linux", Arch: "amd64", CollectorVersion: "0.1.0-test"}
}

func serverReturning(t *testing.T, status int, body string) (*httptest.Server, *httpclient.Client) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, httpclient.New(srv.URL)
}

func TestPairSuccess(t *testing.T) {
	var gotBody pairRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != pairEndpoint {
			t.Errorf("path = %q, want %q", r.URL.Path, pairEndpoint)
		}
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(Result{DeviceID: "dev_91Jaf3", DeviceToken: "iam_device_xxx", UserID: "usr_xxx"})
	}))
	defer srv.Close()

	client := httpclient.New(srv.URL)
	result, err := Pair(context.Background(), client, "CM-7X4P2Q", testDevice())
	if err != nil {
		t.Fatalf("Pair() error = %v", err)
	}
	if result.DeviceID != "dev_91Jaf3" || result.DeviceToken != "iam_device_xxx" || result.UserID != "usr_xxx" {
		t.Errorf("result = %+v", result)
	}
	if gotBody.PairingCode != "CM-7X4P2Q" {
		t.Errorf("PairingCode sent = %q", gotBody.PairingCode)
	}
	if gotBody.Device.OS != "linux" {
		t.Errorf("Device sent = %+v", gotBody.Device)
	}
}

func TestPairInvalidFormat(t *testing.T) {
	_, client := serverReturning(t, http.StatusBadRequest, `{"error":"invalid format"}`)
	_, err := Pair(context.Background(), client, "not-a-code", testDevice())
	if !errors.Is(err, ErrInvalidFormat) {
		t.Errorf("error = %v, want ErrInvalidFormat", err)
	}
}

func TestPairExpired(t *testing.T) {
	_, client := serverReturning(t, http.StatusNotFound, `{}`)
	_, err := Pair(context.Background(), client, "CM-EXPIRED", testDevice())
	if !errors.Is(err, ErrExpiredOrNotFound) {
		t.Errorf("error = %v, want ErrExpiredOrNotFound", err)
	}
}

func TestPairAlreadyUsed(t *testing.T) {
	_, client := serverReturning(t, http.StatusConflict, `{}`)
	_, err := Pair(context.Background(), client, "CM-USED", testDevice())
	if !errors.Is(err, ErrAlreadyUsed) {
		t.Errorf("error = %v, want ErrAlreadyUsed", err)
	}
}

func TestPairDeviceAlreadyPaired(t *testing.T) {
	_, client := serverReturning(t, http.StatusForbidden, `{}`)
	_, err := Pair(context.Background(), client, "CM-VALID", testDevice())
	if !errors.Is(err, ErrAlreadyPaired) {
		t.Errorf("error = %v, want ErrAlreadyPaired", err)
	}
}

func TestPairServerError(t *testing.T) {
	_, client := serverReturning(t, http.StatusInternalServerError, `{}`)
	_, err := Pair(context.Background(), client, "CM-VALID", testDevice())
	if !errors.Is(err, ErrServer) {
		t.Errorf("error = %v, want ErrServer", err)
	}
}

func TestPairInvalidResponseJSON(t *testing.T) {
	_, client := serverReturning(t, http.StatusOK, `not json`)
	_, err := Pair(context.Background(), client, "CM-VALID", testDevice())
	if !errors.Is(err, ErrInvalidResponse) {
		t.Errorf("error = %v, want ErrInvalidResponse", err)
	}
}

func TestPairInvalidResponseMissingFields(t *testing.T) {
	_, client := serverReturning(t, http.StatusOK, `{"user_id":"usr_x"}`)
	_, err := Pair(context.Background(), client, "CM-VALID", testDevice())
	if !errors.Is(err, ErrInvalidResponse) {
		t.Errorf("error = %v, want ErrInvalidResponse", err)
	}
}

func TestPairNetworkError(t *testing.T) {
	client := httpclient.New("http://127.0.0.1:1")
	_, err := Pair(context.Background(), client, "CM-VALID", testDevice())
	if err == nil {
		t.Fatal("Pair() error = nil, want error for unreachable server")
	}
}
