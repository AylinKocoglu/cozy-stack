package auth

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/cozy/cozy-stack/config"
	"github.com/cozy/cozy-stack/instance"
	"github.com/cozy/cozy-stack/web"
	"github.com/cozy/cozy-stack/web/apps"
	"github.com/cozy/cozy-stack/web/errors"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo"
	"github.com/stretchr/testify/assert"
)

type renderer struct {
	t *template.Template
}

func (r *renderer) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return r.t.ExecuteTemplate(w, name, data)
}

const domain = "cozy.example.net"

var ts *httptest.Server
var registerToken []byte
var instanceURL *url.URL

// Stupid http.CookieJar which always returns all cookies.
// NOTE golang stdlib uses cookies for the URL (ie the testserver),
// not for the host (ie the instance), so we do it manually
type testJar struct {
	Jar *cookiejar.Jar
}

func (j *testJar) Cookies(u *url.URL) (cookies []*http.Cookie) {
	return j.Jar.Cookies(instanceURL)
}

func (j *testJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	j.Jar.SetCookies(instanceURL, cookies)
}

var jar *testJar
var client *http.Client
var clientID string

func TestIsLoggedInWhenNotLoggedIn(t *testing.T) {
	content, err := getTestURL()
	assert.NoError(t, err)
	assert.Equal(t, "who_are_you", content)
}

func TestRegisterWrongToken(t *testing.T) {
	res1, err := postForm("/register", &url.Values{
		"passphrase":    {"MyPassphrase"},
		"registerToken": {"BADBEEF"},
	})
	assert.NoError(t, err)
	defer res1.Body.Close()
	assert.Equal(t, "400 Bad Request", res1.Status)

	res2, err := postForm("/register", &url.Values{
		"passphrase":    {"MyPassphrase"},
		"registerToken": {"XYZ"},
	})
	assert.NoError(t, err)
	defer res2.Body.Close()
	assert.Equal(t, "400 Bad Request", res2.Status)
}

func TestRegisterCorrectToken(t *testing.T) {
	res, err := postForm("/register", &url.Values{
		"passphrase":    {"MyPassphrase"},
		"registerToken": {hex.EncodeToString(registerToken)},
	})
	assert.NoError(t, err)
	defer res.Body.Close()
	if assert.Equal(t, "303 See Other", res.Status) {
		assert.Equal(t, "https://onboarding.cozy.example.net/",
			res.Header.Get("Location"))
		cookies := res.Cookies()
		assert.Len(t, cookies, 1)
		assert.Equal(t, cookies[0].Name, SessionCookieName)
		assert.NotEmpty(t, cookies[0].Value)
	}
}

func TestIsLoggedInAfterRegister(t *testing.T) {
	content, err := getTestURL()
	assert.NoError(t, err)
	assert.Equal(t, "logged_in", content)
}

func TestLogout(t *testing.T) {
	req, _ := http.NewRequest("DELETE", ts.URL+"/auth/login", nil)
	req.Host = domain
	res, err := client.Do(req)
	assert.NoError(t, err)
	defer res.Body.Close()
	if assert.Equal(t, "303 See Other", res.Status) {
		assert.Equal(t, "https://cozy.example.net/auth/login",
			res.Header.Get("Location"))
		cookies := jar.Cookies(instanceURL)
		assert.Len(t, cookies, 0)
	}
}

func TestIsLoggedOutAfterLogout(t *testing.T) {
	content, err := getTestURL()
	assert.NoError(t, err)
	assert.Equal(t, "who_are_you", content)
}

func TestShowLoginPage(t *testing.T) {
	req, _ := http.NewRequest("GET", ts.URL+"/auth/login", nil)
	req.Host = domain
	res, err := client.Do(req)
	defer res.Body.Close()
	assert.NoError(t, err)
	assert.Equal(t, "200 OK", res.Status)
	assert.Equal(t, "text/html; charset=utf-8", res.Header.Get("Content-Type"))
	body, _ := ioutil.ReadAll(res.Body)
	assert.Contains(t, string(body), "Please enter your passphrase")
}

func TestShowLoginPageWithRedirectBadURL(t *testing.T) {
	req1, _ := http.NewRequest("GET", ts.URL+"/auth/login?redirect="+url.QueryEscape(" "), nil)
	req1.Host = domain
	res1, err := client.Do(req1)
	defer res1.Body.Close()
	assert.NoError(t, err)
	assert.Equal(t, "400 Bad Request", res1.Status)
	assert.Equal(t, "text/plain; charset=utf-8", res1.Header.Get("Content-Type"))

	req2, _ := http.NewRequest("GET", ts.URL+"/auth/login?redirect="+url.QueryEscape("foo.bar"), nil)
	req2.Host = domain
	res2, err := client.Do(req2)
	defer res2.Body.Close()
	assert.NoError(t, err)
	assert.Equal(t, "400 Bad Request", res2.Status)
	assert.Equal(t, "text/plain; charset=utf-8", res2.Header.Get("Content-Type"))

	req3, _ := http.NewRequest("GET", ts.URL+"/auth/login?redirect="+url.QueryEscape("ftp://sub."+domain+"/foo"), nil)
	req3.Host = domain
	res3, err := client.Do(req3)
	defer res3.Body.Close()
	assert.NoError(t, err)
	assert.Equal(t, "400 Bad Request", res3.Status)
	assert.Equal(t, "text/plain; charset=utf-8", res3.Header.Get("Content-Type"))

	req4, _ := http.NewRequest("GET", ts.URL+"/auth/login?redirect="+url.QueryEscape("https://"+domain+"/foo/bar"), nil)
	req4.Host = domain
	res4, err := client.Do(req4)
	defer res4.Body.Close()
	assert.NoError(t, err)
	assert.Equal(t, "400 Bad Request", res4.Status)
	assert.Equal(t, "text/plain; charset=utf-8", res4.Header.Get("Content-Type"))

	req5, _ := http.NewRequest("GET", ts.URL+"/auth/login?redirect="+url.QueryEscape("https://."+domain+"/foo/bar"), nil)
	req5.Host = domain
	res5, err := client.Do(req5)
	defer res5.Body.Close()
	assert.NoError(t, err)
	assert.Equal(t, "400 Bad Request", res5.Status)
	assert.Equal(t, "text/plain; charset=utf-8", res5.Header.Get("Content-Type"))
}

func TestShowLoginPageWithRedirectXSS(t *testing.T) {
	req, _ := http.NewRequest("GET", ts.URL+"/auth/login?redirect="+url.QueryEscape("https://sub."+domain+"/<script>alert('foo')</script>"), nil)
	req.Host = domain
	res, err := client.Do(req)
	defer res.Body.Close()
	assert.NoError(t, err)
	assert.Equal(t, "200 OK", res.Status)
	assert.Equal(t, "text/html; charset=utf-8", res.Header.Get("Content-Type"))
	body, _ := ioutil.ReadAll(res.Body)
	assert.NotContains(t, string(body), "<script>")
	assert.Contains(t, string(body), "%3Cscript%3Ealert%28%27foo%27%29%3C/script%3E")
}

func TestShowLoginPageWithRedirectFragment(t *testing.T) {
	req, _ := http.NewRequest("GET", ts.URL+"/auth/login?redirect="+url.QueryEscape("https://sub."+domain+"/#myfragment"), nil)
	req.Host = domain
	res, err := client.Do(req)
	defer res.Body.Close()
	assert.NoError(t, err)
	assert.Equal(t, "200 OK", res.Status)
	assert.Equal(t, "text/html; charset=utf-8", res.Header.Get("Content-Type"))
	body, _ := ioutil.ReadAll(res.Body)
	assert.NotContains(t, string(body), "myfragment")
	assert.Contains(t, string(body), `<input type="hidden" name="redirect" value="https://sub.cozy.example.net/#" />`)
}

func TestShowLoginPageWithRedirectSuccess(t *testing.T) {
	req, _ := http.NewRequest("GET", ts.URL+"/auth/login?redirect="+url.QueryEscape("https://sub."+domain+"/foo/bar?query=foo#myfragment"), nil)
	req.Host = domain
	res, err := client.Do(req)
	defer res.Body.Close()
	assert.NoError(t, err)
	assert.Equal(t, "200 OK", res.Status)
	assert.Equal(t, "text/html; charset=utf-8", res.Header.Get("Content-Type"))
	body, _ := ioutil.ReadAll(res.Body)
	assert.NotContains(t, string(body), "myfragment")
	assert.Contains(t, string(body), `<input type="hidden" name="redirect" value="https://sub.cozy.example.net/foo/bar?query=foo#" />`)
}

func TestLoginWithBadPassphrase(t *testing.T) {
	res, err := postForm("/auth/login", &url.Values{
		"passphrase": {"Nope"},
	})
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, "401 Unauthorized", res.Status)
}

func TestLoginWithGoodPassphrase(t *testing.T) {
	res, err := postForm("/auth/login", &url.Values{
		"passphrase": {"MyPassphrase"},
	})
	assert.NoError(t, err)
	defer res.Body.Close()
	if assert.Equal(t, "303 See Other", res.Status) {
		assert.Equal(t, "https://home.cozy.example.net/#",
			res.Header.Get("Location"))
		cookies := res.Cookies()
		assert.Len(t, cookies, 1)
		assert.Equal(t, cookies[0].Name, SessionCookieName)
		assert.NotEmpty(t, cookies[0].Value)
	}
}

func TestLoginWithRedirect(t *testing.T) {
	res1, err := postForm("/auth/login", &url.Values{
		"passphrase": {"MyPassphrase"},
		"redirect":   {"foo.bar"},
	})
	assert.NoError(t, err)
	defer res1.Body.Close()
	assert.Equal(t, "400 Bad Request", res1.Status)

	res2, err := postForm("/auth/login", &url.Values{
		"passphrase": {"MyPassphrase"},
		"redirect":   {"https://sub." + domain + "/#myfragment"},
	})
	assert.NoError(t, err)
	defer res2.Body.Close()
	if assert.Equal(t, "303 See Other", res2.Status) {
		assert.Equal(t, "https://sub.cozy.example.net/#",
			res2.Header.Get("Location"))
	}
}

func TestIsLoggedInAfterLogin(t *testing.T) {
	content, err := getTestURL()
	assert.NoError(t, err)
	assert.Equal(t, "logged_in", content)
}

func TestRegisterClientNotJSON(t *testing.T) {
	res, err := postForm("/auth/register", &url.Values{"foo": {"bar"}})
	assert.NoError(t, err)
	assert.Equal(t, "400 Bad Request", res.Status)
	res.Body.Close()
}

func TestRegisterClientNoRedirectURI(t *testing.T) {
	res, err := postJSON("/auth/register", echo.Map{
		"client_name": "cozy-test",
		"software_id": "github.com/cozy/cozy-test",
	})
	assert.NoError(t, err)
	assert.Equal(t, "400 Bad Request", res.Status)
	var body map[string]string
	err = json.NewDecoder(res.Body).Decode(&body)
	assert.NoError(t, err)
	assert.Equal(t, "invalid_redirect_uri", body["error"])
	assert.Equal(t, "redirect_uris is mandatory", body["error_description"])
}

func TestRegisterClientInvalidRedirectURI(t *testing.T) {
	res, err := postJSON("/auth/register", echo.Map{
		"redirect_uris": []string{"http://example.org/foo#bar"},
		"client_name":   "cozy-test",
		"software_id":   "github.com/cozy/cozy-test",
	})
	assert.NoError(t, err)
	assert.Equal(t, "400 Bad Request", res.Status)
	var body map[string]string
	err = json.NewDecoder(res.Body).Decode(&body)
	assert.NoError(t, err)
	assert.Equal(t, "invalid_redirect_uri", body["error"])
	assert.Equal(t, "http://example.org/foo#bar is invalid", body["error_description"])
}

func TestRegisterClientNoClientName(t *testing.T) {
	res, err := postJSON("/auth/register", echo.Map{
		"redirect_uris": []string{"https://example.org/oauth/callback"},
		"software_id":   "github.com/cozy/cozy-test",
	})
	assert.NoError(t, err)
	assert.Equal(t, "400 Bad Request", res.Status)
	var body map[string]string
	err = json.NewDecoder(res.Body).Decode(&body)
	assert.NoError(t, err)
	assert.Equal(t, "invalid_client_metadata", body["error"])
	assert.Equal(t, "client_name is mandatory", body["error_description"])
}

func TestRegisterClientNoSoftwareID(t *testing.T) {
	res, err := postJSON("/auth/register", echo.Map{
		"redirect_uris": []string{"https://example.org/oauth/callback"},
		"client_name":   "cozy-test",
	})
	assert.NoError(t, err)
	assert.Equal(t, "400 Bad Request", res.Status)
	var body map[string]string
	err = json.NewDecoder(res.Body).Decode(&body)
	assert.NoError(t, err)
	assert.Equal(t, "invalid_client_metadata", body["error"])
	assert.Equal(t, "software_id is mandatory", body["error_description"])
}

func TestRegisterClientSuccessWithJustMandatoryFields(t *testing.T) {
	res, err := postJSON("/auth/register", echo.Map{
		"redirect_uris": []string{"https://example.org/oauth/callback"},
		"client_name":   "cozy-test",
		"software_id":   "github.com/cozy/cozy-test",
	})
	assert.NoError(t, err)
	assert.Equal(t, "201 Created", res.Status)
	var client Client
	err = json.NewDecoder(res.Body).Decode(&client)
	assert.NoError(t, err)
	assert.NotEqual(t, client.ClientID, "")
	assert.NotEqual(t, client.ClientID, "ignored")
	assert.NotEqual(t, client.ClientSecret, "")
	assert.NotEqual(t, client.ClientSecret, "ignored")
	assert.NotEqual(t, client.RegistrationToken, "")
	assert.NotEqual(t, client.RegistrationToken, "ignored")
	assert.Equal(t, client.SecretExpiresAt, 0)
	assert.Equal(t, client.RedirectURIs, []string{"https://example.org/oauth/callback"})
	assert.Equal(t, client.GrantTypes, []string{"authorization_code", "refresh_token"})
	assert.Equal(t, client.ResponseTypes, []string{"code"})
	assert.Equal(t, client.ClientName, "cozy-test")
	assert.Equal(t, client.SoftwareID, "github.com/cozy/cozy-test")
	clientID = client.ClientID
}

func TestRegisterClientSuccessWithAllFields(t *testing.T) {
	res, err := postJSON("/auth/register", echo.Map{
		"_id":                       "ignored",
		"_rev":                      "ignored",
		"client_id":                 "ignored",
		"client_secret":             "ignored",
		"client_secret_expires_at":  42,
		"registration_access_token": "ignored",
		"redirect_uris":             []string{"https://example.org/oauth/callback"},
		"grant_types":               []string{"ignored"},
		"response_types":            []string{"ignored"},
		"client_name":               "cozy-test",
		"client_kind":               "test",
		"client_uri":                "https://github.com/cozy/cozy-test",
		"logo_uri":                  "https://raw.github.com/cozy/cozy-setup/gh-pages/assets/images/happycloud.png",
		"policy_uri":                "https://github/com/cozy/cozy-test/master/policy.md",
		"software_id":               "github.com/cozy/cozy-test",
		"software_version":          "v0.1.2",
	})
	assert.NoError(t, err)
	assert.Equal(t, "201 Created", res.Status)
	var client Client
	err = json.NewDecoder(res.Body).Decode(&client)
	assert.NoError(t, err)
	assert.Equal(t, client.CouchID, "")
	assert.Equal(t, client.CouchRev, "")
	assert.NotEqual(t, client.ClientID, "")
	assert.NotEqual(t, client.ClientID, "ignored")
	assert.NotEqual(t, client.ClientID, clientID)
	assert.NotEqual(t, client.ClientSecret, "")
	assert.NotEqual(t, client.ClientSecret, "ignored")
	assert.NotEqual(t, client.RegistrationToken, "")
	assert.NotEqual(t, client.RegistrationToken, "ignored")
	assert.Equal(t, client.SecretExpiresAt, 0)
	assert.Equal(t, client.RedirectURIs, []string{"https://example.org/oauth/callback"})
	assert.Equal(t, client.GrantTypes, []string{"authorization_code", "refresh_token"})
	assert.Equal(t, client.ResponseTypes, []string{"code"})
	assert.Equal(t, client.ClientName, "cozy-test")
	assert.Equal(t, client.ClientKind, "test")
	assert.Equal(t, client.ClientURI, "https://github.com/cozy/cozy-test")
	assert.Equal(t, client.LogoURI, "https://raw.github.com/cozy/cozy-setup/gh-pages/assets/images/happycloud.png")
	assert.Equal(t, client.PolicyURI, "https://github/com/cozy/cozy-test/master/policy.md")
	assert.Equal(t, client.SoftwareID, "github.com/cozy/cozy-test")
	assert.Equal(t, client.SoftwareVersion, "v0.1.2")
}

func TestAuthorizeFormRedirectsWhenNotLoggedIn(t *testing.T) {
	anonymousClient := &http.Client{CheckRedirect: noRedirect}
	u := url.QueryEscape("https://example.org/oauth/callback")
	req, _ := http.NewRequest("GET", ts.URL+"/auth/authorize?response_type=code&state=123456&scope=files:read&redirect_uri="+u+"&client_id="+clientID, nil)
	req.Host = domain
	res, err := anonymousClient.Do(req)
	defer res.Body.Close()
	assert.NoError(t, err)
	assert.Equal(t, "303 See Other", res.Status)
}

func TestAuthorizeFormBadResponseType(t *testing.T) {
	u := url.QueryEscape("https://example.org/oauth/callback")
	req, _ := http.NewRequest("GET", ts.URL+"/auth/authorize?response_type=token&state=123456&scope=files:read&redirect_uri="+u+"&client_id="+clientID, nil)
	req.Host = domain
	res, err := client.Do(req)
	defer res.Body.Close()
	assert.NoError(t, err)
	assert.Equal(t, "400 Bad Request", res.Status)
	assert.Equal(t, "text/html; charset=utf-8", res.Header.Get("Content-Type"))
	body, _ := ioutil.ReadAll(res.Body)
	assert.Contains(t, string(body), "Invalid response type")
}

func TestAuthorizeFormNoState(t *testing.T) {
	u := url.QueryEscape("https://example.org/oauth/callback")
	req, _ := http.NewRequest("GET", ts.URL+"/auth/authorize?response_type=code&scope=files:read&redirect_uri="+u+"&client_id="+clientID, nil)
	req.Host = domain
	res, err := client.Do(req)
	defer res.Body.Close()
	assert.NoError(t, err)
	assert.Equal(t, "400 Bad Request", res.Status)
	assert.Equal(t, "text/html; charset=utf-8", res.Header.Get("Content-Type"))
	body, _ := ioutil.ReadAll(res.Body)
	assert.Contains(t, string(body), "The state parameter is mandatory")
}

func TestAuthorizeFormNoClientId(t *testing.T) {
	u := url.QueryEscape("https://example.org/oauth/callback")
	req, _ := http.NewRequest("GET", ts.URL+"/auth/authorize?response_type=code&state=123456&scope=files:read&redirect_uri="+u, nil)
	req.Host = domain
	res, err := client.Do(req)
	defer res.Body.Close()
	assert.NoError(t, err)
	assert.Equal(t, "400 Bad Request", res.Status)
	assert.Equal(t, "text/html; charset=utf-8", res.Header.Get("Content-Type"))
	body, _ := ioutil.ReadAll(res.Body)
	assert.Contains(t, string(body), "The client_id parameter is mandatory")
}

func TestAuthorizeFormNoRedirectURI(t *testing.T) {
	req, _ := http.NewRequest("GET", ts.URL+"/auth/authorize?response_type=code&state=123456&scope=files:read&client_id="+clientID, nil)
	req.Host = domain
	res, err := client.Do(req)
	defer res.Body.Close()
	assert.NoError(t, err)
	assert.Equal(t, "400 Bad Request", res.Status)
	assert.Equal(t, "text/html; charset=utf-8", res.Header.Get("Content-Type"))
	body, _ := ioutil.ReadAll(res.Body)
	assert.Contains(t, string(body), "The redirect_uri parameter is mandatory")
}

func TestAuthorizeFormNoScope(t *testing.T) {
	u := url.QueryEscape("https://example.org/oauth/callback")
	req, _ := http.NewRequest("GET", ts.URL+"/auth/authorize?response_type=code&state=123456&redirect_uri="+u+"&client_id="+clientID, nil)
	req.Host = domain
	res, err := client.Do(req)
	defer res.Body.Close()
	assert.NoError(t, err)
	assert.Equal(t, "400 Bad Request", res.Status)
	assert.Equal(t, "text/html; charset=utf-8", res.Header.Get("Content-Type"))
	body, _ := ioutil.ReadAll(res.Body)
	assert.Contains(t, string(body), "The scope parameter is mandatory")
}

func TestAuthorizeFormInvalidClient(t *testing.T) {
	u := url.QueryEscape("https://example.org/oauth/callback")
	req, _ := http.NewRequest("GET", ts.URL+"/auth/authorize?response_type=code&state=123456&scope=files:read&redirect_uri="+u+"&client_id=f00", nil)
	req.Host = domain
	res, err := client.Do(req)
	defer res.Body.Close()
	assert.NoError(t, err)
	assert.Equal(t, "400 Bad Request", res.Status)
	assert.Equal(t, "text/html; charset=utf-8", res.Header.Get("Content-Type"))
	body, _ := ioutil.ReadAll(res.Body)
	assert.Contains(t, string(body), "The client must be registered")
}

func TestAuthorizeFormInvalidRedirectURI(t *testing.T) {
	u := url.QueryEscape("https://evil.com/")
	req, _ := http.NewRequest("GET", ts.URL+"/auth/authorize?response_type=code&state=123456&scope=files:read&redirect_uri="+u+"&client_id="+clientID, nil)
	req.Host = domain
	res, err := client.Do(req)
	defer res.Body.Close()
	assert.NoError(t, err)
	assert.Equal(t, "400 Bad Request", res.Status)
	assert.Equal(t, "text/html; charset=utf-8", res.Header.Get("Content-Type"))
	body, _ := ioutil.ReadAll(res.Body)
	assert.Contains(t, string(body), "The redirect_uri parameter doesn&#39;t match the registered ones")
}

func TestAuthorizeFormSuccess(t *testing.T) {
	u := url.QueryEscape("https://example.org/oauth/callback")
	req, _ := http.NewRequest("GET", ts.URL+"/auth/authorize?response_type=code&state=123456&scope=files:read&redirect_uri="+u+"&client_id="+clientID, nil)
	req.Host = domain
	res, err := client.Do(req)
	defer res.Body.Close()
	assert.NoError(t, err)
	assert.Equal(t, "200 OK", res.Status)
	assert.Equal(t, "text/html; charset=utf-8", res.Header.Get("Content-Type"))
	body, _ := ioutil.ReadAll(res.Body)
	assert.Contains(t, string(body), "would like permission to access your Cozy")
}

func TestMain(m *testing.M) {
	instanceURL, _ = url.Parse("https://" + domain + "/")
	j, _ := cookiejar.New(nil)
	jar = &testJar{
		Jar: j,
	}
	client = &http.Client{
		CheckRedirect: noRedirect,
		Jar:           jar,
	}
	config.UseTestFile()
	instance.Destroy(domain)
	i, _ := instance.Create(domain, "en", nil)
	registerToken = i.RegisterToken

	r := echo.New()
	r.HTTPErrorHandler = errors.ErrorHandler
	r.Renderer = &renderer{
		t: template.Must(template.ParseGlob("../../assets/templates/*.html")),
	}
	Routes(r.Group("", middlewares.NeedInstance))

	r.GET("/test", func(c echo.Context) error {
		var content string
		if IsLoggedIn(c) {
			content = "logged_in"
		} else {
			content = "who_are_you"
		}
		return c.String(http.StatusOK, content)
	}, middlewares.NeedInstance)

	handler, err := web.Create(r, apps.Serve)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	ts = httptest.NewServer(handler)
	res := m.Run()
	ts.Close()
	instance.Destroy(domain)
	os.Exit(res)
}

func noRedirect(*http.Request, []*http.Request) error {
	return http.ErrUseLastResponse
}

func postForm(u string, v *url.Values) (*http.Response, error) {
	req, _ := http.NewRequest("POST", ts.URL+u, bytes.NewBufferString(v.Encode()))
	req.Host = domain
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	return client.Do(req)
}

func postJSON(u string, v echo.Map) (*http.Response, error) {
	body, _ := json.Marshal(v)
	req, _ := http.NewRequest("POST", ts.URL+u, bytes.NewBuffer(body))
	req.Host = domain
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")
	return client.Do(req)
}

func getTestURL() (string, error) {
	req, _ := http.NewRequest("GET", ts.URL+"/test", nil)
	req.Host = domain
	res, err := client.Do(req)
	defer res.Body.Close()
	if err != nil {
		return "", err
	}
	content, _ := ioutil.ReadAll(res.Body)
	return string(content), nil
}
