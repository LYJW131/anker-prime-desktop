// Package cloud is the optional Anker cloud client: login and the list of
// covers stored on the account. Nothing else in the app needs it; BLE and
// the local cover library work without any account. Logging in can sign the
// phone app out, so call Login only from an explicit user action.
package cloud

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// DefaultHost is the CN Anker Power API.
const DefaultHost = "https://aiot-api-cn.anker.com.cn"

const (
	pathLogin = "/passport/login"
	pathList  = "/mini_power/v1/app/style/get_manual_clock_screensavers"
	maxBody   = 32 << 20
)

// Error is a cloud failure; Code is the HTTP status or the body's code.
type Error struct {
	Code    int
	Message string
}

func (e *Error) Error() string { return e.Message }

// IsUnauthorized reports whether err means the session expired.
func IsUnauthorized(err error) bool {
	var ce *Error
	return errors.As(err, &ce) && ce.Code == 401
}

// Session is a logged-in account. UserID is the 40-char id BLE needs.
type Session struct {
	UserID    string
	AuthToken string
	ExpiresAt float64 // as returned by the server (unit unverified, likely Unix seconds)
}

// Picture is one cover on the account.
type Picture struct {
	ID       int
	Seq      int
	Name     string
	HashCode uint32
	ImgURL   string
}

// Client talks to one Anker host.
type Client struct {
	Host string       // default DefaultHost
	HTTP *http.Client // default: 25 s timeout

	now func() time.Time
}

func (c *Client) host() string {
	if c.Host == "" {
		return DefaultHost
	}
	return strings.TrimRight(c.Host, "/")
}

func (c *Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 25 * time.Second}
}

func (c *Client) clock() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

// Login signs in with the account (phone or email) and password.
func (c *Client) Login(ctx context.Context, account, password string) (Session, error) {
	account = strings.TrimSpace(account)
	if account == "" || password == "" {
		return Session{}, &Error{Message: "请先填写 Anker 账号和密码。"}
	}
	env, err := PasswordEnvelope(password)
	if err != nil {
		return Session{}, err
	}
	now := c.clock()
	_, off := now.Zone()
	body := map[string]any{
		"ab":                 "CN",
		"client_secret_info": map[string]any{"public_key": env.ClientPublicKeyHex},
		"enc":                0,
		"email":              account,
		"password":           env.EncryptedPassword,
		"time_zone":          off * 1000,
		"transaction":        strconv.FormatInt(now.UnixMilli(), 10),
	}
	obj, err := c.post(ctx, pathLogin, body, nil)
	if err != nil {
		return Session{}, err
	}
	data, _ := obj["data"].(map[string]any)
	if data == nil {
		return Session{}, &Error{Message: "Anker 云端返回了无法解析的响应。"}
	}
	uid := str(data["user_id"])
	if len(uid) != 40 {
		return Session{}, &Error{Message: "登录成功但没有返回用户 ID。"}
	}
	tok := str(data["auth_token"])
	if tok == "" {
		return Session{}, &Error{Message: "登录成功但没有返回会话令牌。"}
	}
	exp, _ := num(data["token_expires_at"])
	return Session{UserID: uid, AuthToken: tok, ExpiresAt: exp}, nil
}

// ListCovers returns the account's custom covers for the device with serial
// number sn, ordered by seq then id.
func (c *Client) ListCovers(ctx context.Context, sess Session, sn string) ([]Picture, error) {
	sn = strings.TrimSpace(sn)
	if sn == "" {
		return nil, &Error{Message: "还没有读到充电头序列号，连上后再拉封面。"}
	}
	obj, err := c.post(ctx, pathList, map[string]any{"sn": sn}, &sess)
	if err != nil {
		return nil, err
	}
	data, _ := obj["data"].(map[string]any)
	if data == nil {
		return nil, &Error{Message: "Anker 云端返回了无法解析的响应。"}
	}
	rows, _ := data["list"].([]any)
	var out []Picture
	for _, r := range rows {
		row, _ := r.(map[string]any)
		idf, ok := num(row["id"])
		if !ok {
			continue
		}
		id := int(idf)
		hash, ok := ParseHashCode(str(row["hash_code"]))
		if !ok {
			return nil, &Error{Message: fmt.Sprintf("封面 %d 没有 hash_code，无法切换。", id)}
		}
		seq, _ := num(row["seq"])
		out = append(out, Picture{
			ID: id, Seq: int(seq), Name: strings.TrimSpace(str(row["name"])),
			HashCode: hash, ImgURL: str(row["img_url"]),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Seq != out[j].Seq {
			return out[i].Seq < out[j].Seq
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// Download fetches a signed image URL (no Anker headers needed).
func (c *Client) Download(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, &Error{Code: resp.StatusCode, Message: fmt.Sprintf("封面图片下载失败（HTTP %d）", resp.StatusCode)}
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxBody))
}

func (c *Client) post(ctx context.Context, path string, body map[string]any, sess *Session) (map[string]any, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.host()+path, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	for k, v := range map[string]string{
		"content-type": "application/json",
		"app-name":     "anker_power",
		"app-version":  "3.22.2",
		"os-type":      "iOS",
		"model-type":   "PHONE",
		"country":      "CN",
		"language":     "zh",
		"user-agent":   "ktor-client",
	} {
		req.Header.Set(k, v)
	}
	if sess != nil {
		sum := md5.Sum([]byte(sess.UserID))
		req.Header.Set("x-auth-token", sess.AuthToken)
		req.Header.Set("uid", sess.UserID)
		req.Header.Set("gtoken", hex.EncodeToString(sum[:]))
	}
	resp, err := c.http().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, err
	}
	var obj map[string]any
	_ = json.Unmarshal(data, &obj)
	msg := str(obj["msg"])
	code, hasCode := num(obj["code"])
	if resp.StatusCode == 401 || (hasCode && int(code) == 401) {
		if msg == "" {
			msg = "Anker 登录已过期，请重新登录。"
		}
		return nil, &Error{Code: 401, Message: msg}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		if msg == "" {
			msg = fmt.Sprintf("Anker 云端请求失败（HTTP %d）", resp.StatusCode)
		}
		return nil, &Error{Code: resp.StatusCode, Message: msg}
	}
	if obj == nil {
		return nil, &Error{Message: "Anker 云端返回了无法解析的响应。"}
	}
	if !hasCode || int(code) != 0 {
		if msg == "" {
			msg = fmt.Sprintf("Anker 云端返回 %d", int(code))
		}
		return nil, &Error{Code: int(code), Message: msg}
	}
	return obj, nil
}

func str(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	}
	return ""
}

func num(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
		return f, err == nil
	}
	return 0, false
}
