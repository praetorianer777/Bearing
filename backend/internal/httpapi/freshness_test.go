package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/praetorianer777/bearing/backend/internal/db"
)

type memFresh map[string]db.LSN

func (m memFresh) Note(_ context.Context, key string, lsn db.LSN) { m[key] = max(m[key], lsn) }
func (m memFresh) Required(_ context.Context, key string) db.LSN  { return m[key] }

func TestAWriteMintsAClientCookieAndAReadDoesNot(t *testing.T) {
	s := newServer(fakeDB{})
	s.Fresh = memFresh{}
	read := httptest.NewRecorder()
	if key := s.freshnessKey(read, httptest.NewRequest(http.MethodGet, "/", nil)); key != "" || len(read.Result().Cookies()) != 0 {
		t.Errorf("a read got key %q and cookies %v", key, read.Result().Cookies())
	}
	write := httptest.NewRecorder()
	key := s.freshnessKey(write, httptest.NewRequest(http.MethodPost, "/", nil))
	cookies := write.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != ClientCookie || !cookies[0].HttpOnly || key != "c:"+cookies[0].Value {
		t.Fatalf("key %q cookies %v", key, cookies)
	}
	again := httptest.NewRequest(http.MethodGet, "/", nil)
	again.AddCookie(cookies[0])
	if got := s.freshnessKey(httptest.NewRecorder(), again); got != key {
		t.Errorf("the cookie came back as %q, want %q", got, key)
	}
}

func TestAForgedCookieIsIgnored(t *testing.T) {
	s := newServer(fakeDB{})
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: ClientCookie, Value: strings.Repeat("z", 32)})
	if key := s.freshnessKey(httptest.NewRecorder(), r); key != "" {
		t.Errorf("a forged cookie gave key %q", key)
	}
}

func TestAWriteIsNotedAndPinsTheNextRequest(t *testing.T) {
	fresh := memFresh{}
	s := newServer(fakeDB{})
	s.Fresh = fresh
	var cookie *http.Cookie
	write := s.readYourWrites(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { noteWrite(r.Context(), 500) }))
	rec := httptest.NewRecorder()
	write.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))
	cookie = rec.Result().Cookies()[0]
	if fresh["c:"+cookie.Value] != 500 {
		t.Fatalf("the write was not noted: %v", fresh)
	}
	var pinned context.Context
	read := s.readYourWrites(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { pinned = r.Context() }))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(cookie)
	read.ServeHTTP(httptest.NewRecorder(), req)
	if db.RequiredLSN(pinned) != 500 {
		t.Errorf("the next read requires %d, want 500", db.RequiredLSN(pinned))
	}
}
