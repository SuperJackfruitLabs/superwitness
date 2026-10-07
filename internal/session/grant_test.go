package session

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SuperJackfruitLabs/superwitness/internal/auth"
	"github.com/SuperJackfruitLabs/superwitness/internal/testutil"
)

// stale is past a plane access token's five-minute life.
const stale = 5*time.Minute + time.Second

func (r *rig) resolve() (auth.Principal, error) {
	return r.login.Sessions.Resolve(context.Background(), r.sessionCookie())
}

// stored is the session row behind the browser's cookie, and the refresh token it holds.
func (r *rig) stored(t *testing.T) (Session, string) {
	t.Helper()
	id, ok := hashToken(r.sessionCookie())
	if !ok {
		t.Fatal("no session cookie")
	}
	s, err := r.store.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("the session row: %v", err)
	}
	rt, err := openRefresh(r.sessionCookie(), id, s.Grant.Sealed)
	if err != nil {
		t.Fatalf("the stored refresh token does not open with the cookie: %v", err)
	}
	return s, rt
}

func planeSignedIn(t *testing.T) (*rig, *testutil.PlaneStub) {
	t.Helper()
	r, plane := newPlaneRig(t)
	if code, body := r.signIn(t, "/"); code != 200 || body != "app:/" {
		t.Fatalf("sign-in: %d %q", code, body)
	}
	return r, plane
}

func (r *rig) logout(t *testing.T) int {
	t.Helper()
	req, _ := http.NewRequest("POST", r.sw.URL+"/auth/logout", nil)
	req.Header.Set("Origin", r.sw.URL)
	resp, err := r.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp.StatusCode
}

// The refresh token is held server side, sealed: the row never has it in the clear, and the
// browser only ever has the session cookie.
func TestPlaneSessionHoldsTheRefreshTokenServerSide(t *testing.T) {
	r, plane := planeSignedIn(t)
	live := plane.LiveRefreshTokens()
	if len(live) != 1 {
		t.Fatalf("live refresh tokens at the plane = %d; want 1", len(live))
	}
	s, rt := r.stored(t)
	if rt != live[0] {
		t.Errorf("the session holds %q; the plane issued %q", rt, live[0])
	}
	if strings.Contains(string(s.Grant.Sealed), rt) || r.sessionCookie() == rt {
		t.Error("the refresh token is stored or handed out in the clear")
	}
	if want := r.now.Add(5 * time.Minute); !s.Grant.AccessExpiresAt.Equal(want) {
		t.Errorf("access expires %v; want %v", s.Grant.AccessExpiresAt, want)
	}
	for _, c := range r.client.Jar.Cookies(mustURL(t, r.sw.URL)) {
		if strings.Contains(c.Value, rt) {
			t.Errorf("cookie %s carries the refresh token", c.Name)
		}
	}
}

// A fresh access token needs no refresh; a stale one is refreshed, and the rotated refresh token
// replaces the spent one in the store.
func TestPlaneRefreshRotatesAndPersists(t *testing.T) {
	r, plane := planeSignedIn(t)
	_, first := r.stored(t)
	if _, err := r.resolve(); err != nil || plane.RefreshRequests.Load() != 0 {
		t.Fatalf("fresh: %v, refreshes %d; want no refresh", err, plane.RefreshRequests.Load())
	}
	r.now = r.now.Add(stale)
	if p, err := r.resolve(); err != nil || p.ID != "prn_human01" {
		t.Fatalf("stale: %+v %v", p, err)
	}
	s, second := r.stored(t)
	if plane.Refreshes.Load() != 1 || plane.LastToken.Get("refresh_token") != first || second == first {
		t.Fatalf("refreshes %d with %q; stored %q (first %q)", plane.Refreshes.Load(), plane.LastToken.Get("refresh_token"), second, first)
	}
	if plane.LastToken.Get("resource") != r.sw.URL || plane.LastToken.Get("client_id") != "superwitness-web" {
		t.Errorf("refresh form = %v", plane.LastToken)
	}
	if want := r.now.Add(5 * time.Minute); !s.Grant.AccessExpiresAt.Equal(want) {
		t.Errorf("access expires %v; want %v", s.Grant.AccessExpiresAt, want)
	}
	r.now = r.now.Add(stale)
	if _, err := r.resolve(); err != nil || plane.LastToken.Get("refresh_token") != second {
		t.Errorf("the next refresh used %q; want the rotated %q (%v)", plane.LastToken.Get("refresh_token"), second, err)
	}
}

// Signing the app out at the plane ends its grant; the next refresh is refused and the session
// ends with it.
func TestPlaneRefusedRefreshEndsTheSession(t *testing.T) {
	r, plane := planeSignedIn(t)
	plane.EndGrants()
	if _, err := r.resolve(); err != nil {
		t.Fatalf("before the access token is stale the session stands: %v", err)
	}
	r.now = r.now.Add(stale)
	if _, err := r.resolve(); !errors.Is(err, auth.ErrSessionInvalid) {
		t.Fatalf("after a refused refresh: %v; want ErrSessionInvalid", err)
	}
	if r.store.Len() != 0 {
		t.Errorf("sessions = %d; the refused session stays in the store", r.store.Len())
	}
	if !strings.Contains(r.logs.String(), `"msg":"auth.session_ended","principal":"prn_human01","reason":"grant_refused"`) {
		t.Errorf("logs = %s", r.logs)
	}
}

// A workspace that turns superwitness off ends the session at the next refresh, too.
func TestPlaneRefreshWithoutTheProductEndsTheSession(t *testing.T) {
	r, plane := planeSignedIn(t)
	plane.SetEnt([]string{"superpipeline"})
	r.now = r.now.Add(stale)
	if _, err := r.resolve(); !errors.Is(err, auth.ErrSessionInvalid) || r.store.Len() != 0 {
		t.Errorf("%v, sessions %d", err, r.store.Len())
	}
}

// A refresh that names another principal is not this session's grant: the session ends.
func TestPlaneRefreshForSomeoneElseEndsTheSession(t *testing.T) {
	r, plane := planeSignedIn(t)
	plane.RefreshSubOverride = "prn_human02"
	r.now = r.now.Add(stale)
	if _, err := r.resolve(); !errors.Is(err, auth.ErrSessionInvalid) || r.store.Len() != 0 {
		t.Errorf("%v, sessions %d", err, r.store.Len())
	}
}

// An unreachable plane is not a refusal: the session stands for UnreachableGrace, retrying at
// most every RetryEvery, and ends only when the plane stays away past it.
func TestPlaneUnreachableKeepsTheSessionForAGrace(t *testing.T) {
	r, plane := planeSignedIn(t)
	plane.Down.Store(true)
	r.now = r.now.Add(stale)
	start := r.now
	if _, err := r.resolve(); err != nil {
		t.Fatalf("a plane outage signed the session out: %v", err)
	}
	if !strings.Contains(r.logs.String(), `"msg":"auth.refresh_unavailable"`) {
		t.Errorf("the outage is not logged: %s", r.logs)
	}
	asked := plane.RefreshRequests.Load()
	if _, err := r.resolve(); err != nil || plane.RefreshRequests.Load() != asked {
		t.Errorf("a second request at once asked the plane again (%d -> %d) %v", asked, plane.RefreshRequests.Load(), err)
	}
	r.now = start.Add(UnreachableGrace - time.Second)
	if _, err := r.resolve(); err != nil {
		t.Fatalf("inside the grace: %v", err)
	}
	if s, _ := r.stored(t); !s.Grant.UnreachableSince.Equal(start) {
		t.Errorf("unreachable since %v; want the first failure %v", s.Grant.UnreachableSince, start)
	}
	r.now = start.Add(UnreachableGrace)
	if _, err := r.resolve(); !errors.Is(err, auth.ErrSessionInvalid) || r.store.Len() != 0 {
		t.Errorf("past the grace: %v, sessions %d", err, r.store.Len())
	}
	if !strings.Contains(r.logs.String(), `"reason":"issuer_unreachable"`) {
		t.Errorf("logs = %s", r.logs)
	}
}

// A plane that comes back inside the grace clears the outage.
func TestPlaneBackInsideTheGraceClearsTheOutage(t *testing.T) {
	r, plane := planeSignedIn(t)
	plane.Down.Store(true)
	r.now = r.now.Add(stale)
	if _, err := r.resolve(); err != nil {
		t.Fatal(err)
	}
	plane.Down.Store(false)
	r.now = r.now.Add(RetryEvery)
	if _, err := r.resolve(); err != nil {
		t.Fatal(err)
	}
	if s, _ := r.stored(t); !s.Grant.UnreachableSince.IsZero() || plane.Refreshes.Load() != 1 {
		t.Errorf("after recovery: unreachable since %v, refreshes %d", s.Grant.UnreachableSince, plane.Refreshes.Load())
	}
}

// Concurrent requests on one stale session refresh once: two refreshes of a rotating token would
// spend it twice.
func TestPlaneConcurrentRequestsRefreshOnce(t *testing.T) {
	r, plane := planeSignedIn(t)
	plane.RefreshDelay = 50 * time.Millisecond
	plane.ReplayWindow = time.Nanosecond // a second refresh of the spent token is refused
	r.now = r.now.Add(stale)
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := r.resolve()
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("a concurrent request: %v", err)
		}
	}
	if n := plane.RefreshRequests.Load(); n != 1 {
		t.Errorf("refresh requests = %d; want 1", n)
	}
}

// A rotation the store failed to persist is not a sign-out: the request says retry, and the retry
// within the plane's 30-second replay window gets the same rotation back.
func TestPlaneRotationLostToTheStoreIsRecoveredByReplay(t *testing.T) {
	r, plane := planeSignedIn(t)
	_, first := r.stored(t)
	r.store.SetGrantFailures = 1
	r.now = r.now.Add(stale)
	if _, err := r.resolve(); !errors.Is(err, auth.ErrSessionUnavailable) {
		t.Fatalf("a failed persist: %v; want ErrSessionUnavailable", err)
	}
	if _, rt := r.stored(t); rt != first || r.store.Len() != 1 {
		t.Fatalf("after the failed persist: stored %q, sessions %d", rt, r.store.Len())
	}
	if _, err := r.resolve(); err != nil {
		t.Fatalf("the retry: %v", err)
	}
	if _, rt := r.stored(t); rt == first || plane.RefreshRequests.Load() != 2 {
		t.Errorf("after the retry: stored %q (first %q), refresh requests %d", rt, first, plane.RefreshRequests.Load())
	}
}

// Signing out of the app revokes its refresh token at the plane, then drops the session.
func TestPlaneSignOutRevokesTheGrant(t *testing.T) {
	r, plane := planeSignedIn(t)
	_, rt := r.stored(t)
	if code := r.logout(t); code != 204 || r.store.Len() != 0 {
		t.Fatalf("sign-out: %d, sessions %d", code, r.store.Len())
	}
	if got := plane.Revoked(); len(got) != 1 || got[0] != rt {
		t.Errorf("revoked at the plane: %q; want [%q]", got, rt)
	}
	if len(plane.LiveRefreshTokens()) != 0 {
		t.Error("a live grant survived sign-out")
	}
}

// Signing in again revokes the old session's grant, as it ends the old session.
func TestPlaneSignInAgainRevokesTheOldGrant(t *testing.T) {
	r, plane := planeSignedIn(t)
	_, old := r.stored(t)
	if code, _ := r.signIn(t, "/"); code != 200 {
		t.Fatalf("second sign-in: %d", code)
	}
	if got := plane.Revoked(); len(got) != 1 || got[0] != old || len(plane.LiveRefreshTokens()) != 1 {
		t.Errorf("revoked %q (old %q), live %d", got, old, len(plane.LiveRefreshTokens()))
	}
}

// The revocation endpoint comes from discovery and must be on the plane's origin; one elsewhere is
// never sent the token. The session ends all the same.
func TestPlaneRevocationEndpointMustBeSameOrigin(t *testing.T) {
	r, plane := planeSignedIn(t)
	var sent int
	elsewhere := httptestServer(t, func(http.ResponseWriter, *http.Request) { sent++ })
	plane.RevocationEndpoint = elsewhere + "/api/auth/oauth2/revoke"
	if code := r.logout(t); code != 204 || r.store.Len() != 0 {
		t.Fatalf("sign-out: %d, sessions %d", code, r.store.Len())
	}
	if sent != 0 || plane.Revocations.Load() != 0 {
		t.Errorf("a revocation was sent: elsewhere %d, plane %d", sent, plane.Revocations.Load())
	}
	if !strings.Contains(r.logs.String(), `"msg":"auth.revoke_failed"`) {
		t.Errorf("logs = %s", r.logs)
	}
}

// A plane that answers the code with no refresh token cannot bind the session: no sign-in.
func TestPlaneSignInWithoutARefreshTokenIsRefused(t *testing.T) {
	r, _ := newPlaneRig(t)
	r.login.Endpoints = noOfflineEndpoints{r.login.Endpoints.(PlaneEndpoints)}
	if code, _ := r.signIn(t, "/"); code != 400 || r.store.Len() != 0 || !strings.Contains(r.logs.String(), `"reason":"no_refresh_token"`) {
		t.Errorf("%d, sessions %d, logs %s", code, r.store.Len(), r.logs)
	}
}

// A session with no grant (one issued before sessions held one) does not stand under the plane.
func TestPlaneSessionWithoutAGrantEnds(t *testing.T) {
	r, _ := newPlaneRig(t)
	tok, err := r.login.Sessions.Issue(context.Background(), auth.Principal{ID: "prn_human01", Kind: auth.KindHuman, Tenant: "org_01"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.login.Sessions.Resolve(context.Background(), tok); !errors.Is(err, auth.ErrSessionInvalid) || r.store.Len() != 0 {
		t.Errorf("a grantless session under the plane: %v, sessions %d", err, r.store.Len())
	}
	if !strings.Contains(r.logs.String(), `"reason":"no_grant"`) {
		t.Errorf("logs = %s", r.logs)
	}
}

// A request that read the session stale while another request's refresh was finishing does not
// refresh again: the flight reads the row afresh first.
func TestPlaneRefreshAlreadyDoneIsNotRepeated(t *testing.T) {
	r, plane := planeSignedIn(t)
	r.now = r.now.Add(stale)
	if _, err := r.resolve(); err != nil {
		t.Fatal(err)
	}
	id, _ := hashToken(r.sessionCookie())
	if err := r.login.Sessions.renew(context.Background(), r.sessionCookie(), id); err != nil {
		t.Fatal(err)
	}
	if n := plane.RefreshRequests.Load(); n != 1 {
		t.Errorf("refresh requests = %d; want 1", n)
	}
}

// noOfflineEndpoints asks for openid alone, so the plane issues no refresh token.
type noOfflineEndpoints struct{ PlaneEndpoints }

func (e noOfflineEndpoints) AuthorizeURL(clientID, redirectURI, state, challenge string) string {
	return strings.Replace(e.PlaneEndpoints.AuthorizeURL(clientID, redirectURI, state, challenge), "+offline_access", "", 1)
}

func mustURL(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func httptestServer(t *testing.T, h http.HandlerFunc) string {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv.URL
}
