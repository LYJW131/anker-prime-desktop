package cloud

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestEncryptVector(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	ct, err := EncryptAES256CBC(key, key[:16], []byte("test-password"))
	if err != nil {
		t.Fatal(err)
	}
	if got := base64.StdEncoding.EncodeToString(ct); got != "/pwCUy+lmRxTF2oWJX1Qrg==" {
		t.Fatalf("got %s", got)
	}
}

func TestPasswordEnvelopeShape(t *testing.T) {
	e, err := PasswordEnvelope("x")
	if err != nil {
		t.Fatal(err)
	}
	if len(e.ClientPublicKeyHex) != 130 || !strings.HasPrefix(e.ClientPublicKeyHex, "04") {
		t.Fatalf("bad pubkey %q", e.ClientPublicKeyHex)
	}
}

func TestParseHashCode(t *testing.T) {
	cases := map[string]uint32{
		"0x1da2ddca": 0x1da2ddca,
		"1da2ddca":   0x1da2ddca,
		"255":        0x255,     // hex first
		"123456789":  123456789, // too wide for 32-bit hex, falls back to decimal
	}
	for in, want := range cases {
		if got, ok := ParseHashCode(in); !ok || got != want {
			t.Errorf("%s: got %x ok=%v", in, got, ok)
		}
	}
	for _, bad := range []string{" ", "zz", "99999999999"} {
		if _, ok := ParseHashCode(bad); ok {
			t.Errorf("%q should fail", bad)
		}
	}
}

func TestLoginAndList(t *testing.T) {
	uid := strings.Repeat("a", 40)
	var loginBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("app-name") != "anker_power" || r.Header.Get("country") != "CN" {
			t.Errorf("headers: %v", r.Header)
		}
		switch r.URL.Path {
		case "/passport/login":
			json.NewDecoder(r.Body).Decode(&loginBody)
			w.Write([]byte(`{"code":0,"data":{"user_id":"` + uid + `","auth_token":"tok","token_expires_at":1800000000}}`))
		case "/mini_power/v1/app/style/get_manual_clock_screensavers":
			sum := md5.Sum([]byte(uid))
			if r.Header.Get("uid") != uid || r.Header.Get("x-auth-token") != "tok" || r.Header.Get("gtoken") != hex.EncodeToString(sum[:]) {
				t.Errorf("auth headers: %v", r.Header)
			}
			var b map[string]any
			json.NewDecoder(r.Body).Decode(&b)
			if b["sn"] != "SN1" {
				t.Errorf("sn %v", b)
			}
			w.Write([]byte(`{"code":0,"data":{"list":[
			 {"id":9,"seq":2,"name":" b ","hash_code":"0x0000000a","img_url":"u2"},
			 {"id":3,"seq":1,"name":"a","hash_code":"255","img_url":"u1"}]}}`))
		}
	}))
	defer srv.Close()
	c := &Client{Host: srv.URL, now: func() time.Time { return time.UnixMilli(1700000000123).In(time.FixedZone("x", 8*3600)) }}
	sess, err := c.Login(context.Background(), " me@x.com ", "pw")
	if err != nil {
		t.Fatal(err)
	}
	if sess.UserID != uid || sess.AuthToken != "tok" || sess.ExpiresAt != 1800000000 {
		t.Fatalf("%+v", sess)
	}
	if loginBody["email"] != "me@x.com" || loginBody["ab"] != "CN" || loginBody["transaction"] != "1700000000123" || loginBody["time_zone"] != float64(28800000) {
		t.Fatalf("%v", loginBody)
	}
	if loginBody["password"] == "pw" || loginBody["client_secret_info"].(map[string]any)["public_key"] == "" {
		t.Fatalf("%v", loginBody)
	}
	pics, err := c.ListCovers(context.Background(), sess, "SN1")
	if err != nil {
		t.Fatal(err)
	}
	if len(pics) != 2 || pics[0].ID != 3 || pics[0].HashCode != 0x255 || pics[1].HashCode != 10 || pics[1].Name != "b" {
		t.Fatalf("%+v", pics)
	}
}

func TestErrors(t *testing.T) {
	status, body := 200, ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	defer srv.Close()
	c := &Client{Host: srv.URL}
	ctx := context.Background()
	try := func(s int, b string) error {
		status, body = s, b
		_, err := c.Login(ctx, "a", "b")
		return err
	}
	if err := try(401, `{}`); !IsUnauthorized(err) {
		t.Errorf("401: %v", err)
	}
	if err := try(200, `{"code":401,"msg":"x"}`); !IsUnauthorized(err) {
		t.Errorf("code 401: %v", err)
	}
	if err := try(200, `{"code":10007,"msg":"bad pw"}`); err == nil || err.Error() != "bad pw" {
		t.Errorf("code!=0: %v", err)
	}
	if err := try(500, `oops`); err == nil {
		t.Error("500")
	}
	if err := try(200, `{"code":0,"data":{"user_id":"short","auth_token":"t"}}`); err == nil {
		t.Error("short uid")
	}
	if _, err := c.Login(ctx, "", "x"); err == nil {
		t.Error("empty account")
	}
	status, body = 200, `{"code":0,"data":{"list":[{"id":1,"hash_code":""}]}}`
	if _, err := c.ListCovers(ctx, Session{UserID: strings.Repeat("a", 40), AuthToken: "t"}, "s"); err == nil {
		t.Error("missing hash")
	}
}

func TestDownload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ok" {
			w.Write([]byte("jpg"))
			return
		}
		w.WriteHeader(403)
	}))
	defer srv.Close()
	c := &Client{}
	b, err := c.Download(context.Background(), srv.URL+"/ok")
	if err != nil || string(b) != "jpg" {
		t.Fatal(b, err)
	}
	if _, err := c.Download(context.Background(), srv.URL+"/no"); err == nil {
		t.Fatal("want error")
	}
}
