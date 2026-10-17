package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"testing"
	"time"
)

func TestWebhookSignatureReplayAndTampering(t *testing.T) {
	now := time.Unix(1800000000, 0)
	stamp := strconv.FormatInt(now.Unix(), 10)
	raw := []byte(`{"value":1}`)
	key := "event_one"
	mac := hmac.New(sha256.New, []byte("fake-secret"))
	_, _ = mac.Write([]byte(stamp + "." + key + "."))
	_, _ = mac.Write(raw)
	signature := hex.EncodeToString(mac.Sum(nil))
	if !validSignature("fake-secret", stamp, key, signature, raw, now) {
		t.Fatal("valid signature rejected")
	}
	if validSignature("fake-secret", stamp, key, signature, raw, now.Add(301*time.Second)) {
		t.Fatal("expired request accepted")
	}
	if validSignature("fake-secret", stamp, "event_two", signature, raw, now) {
		t.Fatal("unsigned key accepted")
	}
	if validSignature("fake-secret", stamp, key, signature, []byte(`{}`), now) {
		t.Fatal("tampered body accepted")
	}
	if validSignature("fake-secret", stamp, key, signature, raw, now.Add(-31*time.Second)) {
		t.Fatal("future request accepted")
	}
}
