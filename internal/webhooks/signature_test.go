package webhooks

import "testing"

func TestSignAndVerify(t *testing.T) {
	secret := "whsec_test"
	body := []byte(`{"id":"evt_1","type":"email.accepted"}`)
	sig := Sign(secret, 1755600000, body)

	if !Verify(secret, 1755600000, body, sig) {
		t.Fatal("valid signature rejected")
	}
	if Verify(secret, 1755600001, body, sig) {
		t.Fatal("signature accepted with wrong timestamp")
	}
	if Verify("whsec_other", 1755600000, body, sig) {
		t.Fatal("signature accepted with wrong secret")
	}
	if Verify(secret, 1755600000, []byte(`tampered`), sig) {
		t.Fatal("signature accepted with tampered body")
	}
}

func TestBackoffSchedule(t *testing.T) {
	if backoff(1) >= backoff(4) {
		t.Fatal("backoff must grow")
	}
	if backoff(100) != backoff(8) {
		t.Fatal("backoff must cap at the last step")
	}
}
