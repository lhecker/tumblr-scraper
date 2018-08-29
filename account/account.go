package account

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/lhecker/tumblr-scraper/config"
)

const (
	consentURL    = "https://www.tumblr.com/privacy/consent"
	consentSvcURL = "https://www.tumblr.com/svc/privacy/consent"
	loginURL      = "https://www.tumblr.com/login"
	logoutURL     = "https://www.tumblr.com/logout"
)

var (
	formKeyRegexp = regexp.MustCompile(`name="tumblr-form-key".+?content="([^"]+)`)

	sharedClient *http.Client
	sharedConfig *config.Config

	loginState uint32
	loginLock  sync.Mutex
)

func Setup(client *http.Client, cfg *config.Config) {
	sharedClient = client
	sharedConfig = cfg
}

func LoginOnce() error {
	if len(sharedConfig.Username) == 0 || len(sharedConfig.Password) == 0 {
		return errors.New("missing username/password")
	}

	return transitionLoginState(0, 1, func() error {
		log.Printf("logging in as %s", sharedConfig.Username)

		err := consent()
		if err != nil {
			return err
		}

		return login()
	})
}

func Logout() error {
	return transitionLoginState(1, 0, func() error {
		log.Println("logging out")

		return logout()
	})
}

func transitionLoginState(from, to uint32, f func() error) error {
	if atomic.LoadUint32(&loginState) != from {
		return nil
	}

	loginLock.Lock()
	defer loginLock.Unlock()

	if loginState != from {
		return nil
	}

	err := f()
	if err != nil {
		return err
	}

	atomic.StoreUint32(&loginState, to)
	return nil
}

func getFormKey(url string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	res, err := sharedClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return "", fmt.Errorf("bad status code: %d %s", res.StatusCode, res.Status)
	}

	bodyBuilder := &strings.Builder{}
	_, err = io.Copy(bodyBuilder, res.Body)
	if err != nil {
		return "", err
	}
	body := bodyBuilder.String()

	m := formKeyRegexp.FindStringSubmatch(body)
	if len(m) == 0 {
		return "", errors.New("failed to find form key")
	}

	return m[1], nil
}

func consent() error {
	formKey, err := getFormKey(consentURL)
	if err != nil {
		return err
	}

	consentData := &struct {
		EuResident               bool `json:"eu_resident"`
		GdprIsAcceptableAge      bool `json:"gdpr_is_acceptable_age"`
		GdprConsentCore          bool `json:"gdpr_consent_core"`
		GdprConsentFirstPartyAds bool `json:"gdpr_consent_first_party_ads"`
		GdprConsentThirdPartyAds bool `json:"gdpr_consent_third_party_ads"`
		GdprConsentSearchHistory bool `json:"gdpr_consent_search_history"`
	}{
		true,
		true,
		true,
		true,
		false,
		true,
	}

	postData, err := json.Marshal(consentData)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, consentSvcURL, bytes.NewReader(postData))
	if err != nil {
		return err
	}

	req.Header.Set("Referer", consentURL)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("X-tumblr-form-key", formKey)

	res, err := sharedClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return fmt.Errorf("bad status code: %d %s", res.StatusCode, res.Status)
	}

	return nil
}

func login() error {
	formKey, err := getFormKey(loginURL)
	if err != nil {
		return err
	}

	postData := url.Values{
		"version":        {"STANDARD"},
		"form_key":       {formKey},
		"user[email]":    {sharedConfig.Username},
		"user[password]": {sharedConfig.Password},
	}.Encode()

	req, err := http.NewRequest(http.MethodPost, loginURL, strings.NewReader(postData))
	if err != nil {
		return err
	}

	req.Header.Set("Referer", loginURL)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	res, err := sharedClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return fmt.Errorf("bad status code: %d %s", res.StatusCode, res.Status)
	}

	return nil
}

func logout() error {
	req, err := http.NewRequest(http.MethodGet, logoutURL, nil)
	if err != nil {
		return err
	}

	res, err := sharedClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return fmt.Errorf("bad status code: %d %s", res.StatusCode, res.Status)
	}

	return nil
}
