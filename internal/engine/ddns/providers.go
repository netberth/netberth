// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package ddns

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/netberth/netberth/internal/model"
)

// Aliyun DNS (Alibaba Cloud) — https://help.aliyun.com/document_detail/29776.html

func (e *Engine) updateAliyun(cfg model.DDNSConfig, ip string) error {
	accessKeyID := cfg.Credentials["access_key_id"]
	accessKeySecret := cfg.Credentials["access_key_secret"]
	if accessKeyID == "" || accessKeySecret == "" {
		return fmt.Errorf("missing aliyun credentials: access_key_id, access_key_secret")
	}
	recordID, err := e.aliyunGetRecord(cfg, accessKeyID, accessKeySecret)
	if err != nil {
		return fmt.Errorf("aliyun get record: %w", err)
	}
	return e.aliyunUpdateRecord(cfg, ip, recordID, accessKeyID, accessKeySecret)
}

func (e *Engine) aliyunGetRecord(cfg model.DDNSConfig, keyID, keySecret string) (string, error) {
	params := map[string]string{
		"Action":           "DescribeDomainRecords",
		"DomainName":       cfg.Domain,
		"RRKeyWord":        cfg.SubDomain,
		"TypeKeyWord":      cfg.RecordType,
		"Format":           "JSON",
		"Version":          "2015-01-09",
		"AccessKeyId":      keyID,
		"SignatureMethod":  "HMAC-SHA1",
		"Timestamp":        time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		"SignatureVersion": "1.0",
		"SignatureNonce":   fmt.Sprintf("%d", rand.Int63()),
	}
	sig := aliyunSign(params, keySecret, "GET")
	params["Signature"] = sig

	query := aliyunBuildQuery(params)
	resp, err := http.Get("https://alidns.aliyuncs.com/?" + query)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var result struct {
		DomainRecords struct {
			Record []struct {
				RecordID string `json:"RecordId"`
			} `json:"Record"`
		} `json:"DomainRecords"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parse aliyun response: %w", err)
	}
	if len(result.DomainRecords.Record) == 0 {
		// Record doesn't exist, create it
		return e.aliyunCreateRecord(cfg, keyID, keySecret)
	}
	return result.DomainRecords.Record[0].RecordID, nil
}

func (e *Engine) aliyunCreateRecord(cfg model.DDNSConfig, keyID, keySecret string) (string, error) {
	rr := cfg.SubDomain
	if rr == "@" {
		rr = ""
	}
	params := map[string]string{
		"Action":           "AddDomainRecord",
		"DomainName":       cfg.Domain,
		"RR":               rr,
		"Type":             cfg.RecordType,
		"Value":            "0.0.0.0",
		"TTL":              fmt.Sprintf("%d", cfg.TTL),
		"Format":           "JSON",
		"Version":          "2015-01-09",
		"AccessKeyId":      keyID,
		"SignatureMethod":  "HMAC-SHA1",
		"Timestamp":        time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		"SignatureVersion": "1.0",
		"SignatureNonce":   fmt.Sprintf("%d", rand.Int63()),
	}
	sig := aliyunSign(params, keySecret, "GET")
	params["Signature"] = sig

	query := aliyunBuildQuery(params)
	resp, err := http.Get("https://alidns.aliyuncs.com/?" + query)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var result struct {
		RecordID string `json:"RecordId"`
	}
	json.Unmarshal(body, &result)
	if result.RecordID == "" {
		return "", fmt.Errorf("failed to create aliyun record: %s", string(body))
	}
	return result.RecordID, nil
}

func (e *Engine) aliyunUpdateRecord(cfg model.DDNSConfig, ip, recordID, keyID, keySecret string) error {
	rr := cfg.SubDomain
	if rr == "@" {
		rr = ""
	}
	params := map[string]string{
		"Action":           "UpdateDomainRecord",
		"RecordId":         recordID,
		"RR":               rr,
		"Type":             cfg.RecordType,
		"Value":            ip,
		"TTL":              fmt.Sprintf("%d", cfg.TTL),
		"Format":           "JSON",
		"Version":          "2015-01-09",
		"AccessKeyId":      keyID,
		"SignatureMethod":  "HMAC-SHA1",
		"Timestamp":        time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		"SignatureVersion": "1.0",
		"SignatureNonce":   fmt.Sprintf("%d", rand.Int63()),
	}
	sig := aliyunSign(params, keySecret, "GET")
	params["Signature"] = sig

	query := aliyunBuildQuery(params)
	resp, err := http.Get("https://alidns.aliyuncs.com/?" + query)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("aliyun update failed: %s", string(body))
	}
	return nil
}

func aliyunSign(params map[string]string, keySecret, method string) string {
	sortedQuery := aliyunSortedQuery(params)
	strToSign := method + "&" + url.QueryEscape("/") + "&" + url.QueryEscape(sortedQuery)
	mac := hmac.New(sha1.New, []byte(keySecret+"&"))
	mac.Write([]byte(strToSign))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func aliyunBuildQuery(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = url.QueryEscape(k) + "=" + url.QueryEscape(params[k])
	}
	return strings.Join(parts, "&")
}

func aliyunSortedQuery(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	escaped := make([]string, len(keys))
	for i, k := range keys {
		escaped[i] = url.QueryEscape(k) + "=" + url.QueryEscape(params[k])
	}
	return strings.Join(escaped, "&")
}

// DNSPod (Tencent Cloud) — https://cloud.tencent.com/document/api/1427/56166

func (e *Engine) updateDNSPod(cfg model.DDNSConfig, ip string) error {
	secretID := cfg.Credentials["secret_id"]
	secretKey := cfg.Credentials["secret_key"]
	if secretID == "" || secretKey == "" {
		return fmt.Errorf("missing dnspod credentials: secret_id, secret_key")
	}
	domain := cfg.Domain
	subDomain := cfg.SubDomain
	recordType := cfg.RecordType

	recordID, err := e.dnspodGetRecord(domain, subDomain, recordType, secretID, secretKey)
	if err != nil {
		return fmt.Errorf("dnspod get record: %w", err)
	}
	if recordID == 0 {
		return e.dnspodCreateRecord(domain, subDomain, recordType, ip, cfg.TTL, secretID, secretKey)
	}
	return e.dnspodUpdateRecord(domain, subDomain, recordType, ip, recordID, cfg.TTL, secretID, secretKey)
}

func (e *Engine) dnspodGetRecord(domain, subDomain, recordType, secretID, secretKey string) (int, error) {
	payload := map[string]interface{}{
		"Domain":     domain,
		"Subdomain":  subDomain,
		"RecordType": recordType,
	}
	resp, err := dnspodCall("DescribeRecordList", payload, secretID, secretKey)
	if err != nil {
		return 0, err
	}
	var result struct {
		Response struct {
			RecordList []struct {
				RecordID int    `json:"RecordId"`
				Value    string `json:"Value"`
			} `json:"RecordList"`
		} `json:"Response"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return 0, fmt.Errorf("parse dnspod response: %w", err)
	}
	if len(result.Response.RecordList) > 0 {
		return result.Response.RecordList[0].RecordID, nil
	}
	return 0, nil
}

func (e *Engine) dnspodCreateRecord(domain, subDomain, recordType, value string, ttl int, secretID, secretKey string) error {
	payload := map[string]interface{}{
		"Domain":     domain,
		"SubDomain":  subDomain,
		"RecordType": recordType,
		"RecordLine": "默认",
		"Value":      value,
		"TTL":        ttl,
	}
	resp, err := dnspodCall("CreateRecord", payload, secretID, secretKey)
	if err != nil {
		return err
	}
	var result struct {
		Response struct {
			Error struct {
				Code    string `json:"Code"`
				Message string `json:"Message"`
			} `json:"Error"`
		} `json:"Response"`
	}
	json.Unmarshal(resp, &result)
	if result.Response.Error.Code != "" {
		return fmt.Errorf("dnspod create error: %s", result.Response.Error.Message)
	}
	return nil
}

func (e *Engine) dnspodUpdateRecord(domain, subDomain, recordType, value string, recordID, ttl int, secretID, secretKey string) error {
	payload := map[string]interface{}{
		"Domain":     domain,
		"SubDomain":  subDomain,
		"RecordType": recordType,
		"RecordLine": "默认",
		"Value":      value,
		"RecordId":   recordID,
		"TTL":        ttl,
	}
	resp, err := dnspodCall("ModifyRecord", payload, secretID, secretKey)
	if err != nil {
		return err
	}
	var result struct {
		Response struct {
			Error struct {
				Code string `json:"Code"`
			} `json:"Error"`
		} `json:"Response"`
	}
	json.Unmarshal(resp, &result)
	if result.Response.Error.Code != "" {
		return fmt.Errorf("dnspod update failed")
	}
	return nil
}

func dnspodCall(action string, payload map[string]interface{}, secretID, secretKey string) ([]byte, error) {
	service := "dnspod"
	host := "dnspod.tencentcloudapi.com"
	version := "2021-03-23"
	region := ""

	body, _ := json.Marshal(payload)
	timestamp := time.Now().Unix()
	date := time.Now().UTC().Format("2006-01-02")

	// Step 1: Canonical Request
	httpMethod := "POST"
	canonicalURI := "/"
	canonicalQuery := ""
	canonicalHeaders := fmt.Sprintf("content-type:application/json\nhost:%s\n", host)
	signedHeaders := "content-type;host"
	hashedPayload := sha256Hex(body)
	canonicalReq := fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n%s",
		httpMethod, canonicalURI, canonicalQuery, canonicalHeaders, signedHeaders, hashedPayload)

	// Step 2: String to Sign
	algorithm := "TC3-HMAC-SHA256"
	credentialScope := fmt.Sprintf("%s/%s/tc3_request", date, service)
	hashedCanonicalReq := sha256Hex([]byte(canonicalReq))
	strToSign := fmt.Sprintf("%s\n%d\n%s\n%s",
		algorithm, timestamp, credentialScope, hashedCanonicalReq)

	// Step 3: Signature
	secretDate := hmacSHA256([]byte("TC3"+secretKey), []byte(date))
	secretService := hmacSHA256(secretDate, []byte(service))
	secretSigning := hmacSHA256(secretService, []byte("tc3_request"))
	signature := hex.EncodeToString(hmacSHA256(secretSigning, []byte(strToSign)))

	// Step 4: Authorization header
	authorization := fmt.Sprintf("%s Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		algorithm, secretID, credentialScope, signedHeaders, signature)

	req, _ := http.NewRequest("POST", "https://"+host+"/", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Host", host)
	req.Header.Set("X-TC-Action", action)
	req.Header.Set("X-TC-Version", version)
	req.Header.Set("X-TC-Timestamp", fmt.Sprintf("%d", timestamp))
	if region != "" {
		req.Header.Set("X-TC-Region", region)
	}
	req.Header.Set("Authorization", authorization)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func hmacSHA256(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}

// ===== Additional DDNS Providers =====

// GoDaddy: https://developer.godaddy.com/doc/endpoint/domains
func godaddyUpdate(cfg model.DDNSConfig, ip string) error {
	key := cfg.Credentials["api_key"]
	secret := cfg.Credentials["api_secret"]
	if key == "" || secret == "" {
		return fmt.Errorf("godaddy: need api_key and api_secret")
	}
	recordType := cfg.RecordType
	name := cfg.SubDomain
	if name == "@" {
		name = cfg.Domain
	} else {
		name = name + "." + cfg.Domain
	}
	url := fmt.Sprintf("https://api.godaddy.com/v1/domains/%s/records/%s/%s", cfg.Domain, recordType, cfg.SubDomain)
	body := fmt.Sprintf(`[{"data":"%s","ttl":%d}]`, ip, cfg.TTL)
	req, _ := http.NewRequest("PUT", url, strings.NewReader(body))
	req.Header.Set("Authorization", "sso-key "+key+":"+secret)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("godaddy API: %d", resp.StatusCode)
	}
	return nil
}

// DuckDNS: https://www.duckdns.org/spec.jsp
func duckdnsUpdate(cfg model.DDNSConfig, ip string) error {
	token := cfg.Credentials["token"]
	if token == "" {
		return fmt.Errorf("duckdns: need token")
	}
	domains := cfg.SubDomain
	if domains == "@" {
		domains = cfg.Domain
	}
	url := fmt.Sprintf("https://www.duckdns.org/update?domains=%s&token=%s&ip=%s", domains, token, ip)
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.HasPrefix(string(body), "OK") {
		return fmt.Errorf("duckdns: %s", string(body))
	}
	return nil
}

// No-IP: https://www.noip.com/integrate/request
func noipUpdate(cfg model.DDNSConfig, ip string) error {
	user := cfg.Credentials["username"]
	pass := cfg.Credentials["password"]
	if user == "" || pass == "" {
		return fmt.Errorf("noip: need username and password")
	}
	hostname := cfg.SubDomain
	if hostname == "@" {
		hostname = cfg.Domain
	} else {
		hostname = hostname + "." + cfg.Domain
	}
	url := fmt.Sprintf("https://dynupdate.no-ip.com/nic/update?hostname=%s&myip=%s", hostname, ip)
	req, _ := http.NewRequest("GET", url, nil)
	req.SetBasicAuth(user, pass)
	req.Header.Set("User-Agent", "NetBerth/0.1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	s := string(body)
	if !strings.HasPrefix(s, "good ") && !strings.HasPrefix(s, "nochg ") {
		return fmt.Errorf("noip: %s", s)
	}
	return nil
}

// Dynv6: https://dynv6.com/docs/apis
func dynv6Update(cfg model.DDNSConfig, ip string) error {
	token := cfg.Credentials["token"]
	if token == "" {
		return fmt.Errorf("dynv6: need token")
	}
	zone := cfg.Domain
	if cfg.SubDomain != "@" {
		zone = cfg.SubDomain + "." + cfg.Domain
	}
	url := fmt.Sprintf("https://dynv6.com/api/update?zone=%s&token=%s&ipv4=%s", zone, token, ip)
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	s := string(body)
	if !strings.HasPrefix(s, "addresses updated") && !strings.HasPrefix(s, "addresses unchanged") {
		return fmt.Errorf("dynv6: %s", s)
	}
	return nil
}

// Namecheap: https://www.namecheap.com/support/knowledgebase/article.aspx/29/11/how-to-dynamically-update-the-hosts-ip-with-an-http-request
func namecheapUpdate(cfg model.DDNSConfig, ip string) error {
	pass := cfg.Credentials["password"]
	if pass == "" {
		return fmt.Errorf("namecheap: need password (Dynamic DNS Password from dashboard)")
	}
	host := cfg.SubDomain
	if host == "@" {
		host = ""
	}
	url := fmt.Sprintf("https://dynamicdns.park-your-domain.com/update?host=%s&domain=%s&password=%s&ip=%s", host, cfg.Domain, pass, ip)
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// ClouDNS: https://www.cloudns.net/wiki/article/42/
func cloudnsUpdate(cfg model.DDNSConfig, ip string) error {
	authID := cfg.Credentials["auth_id"]
	authPass := cfg.Credentials["auth_password"]
	subID := cfg.Credentials["sub_auth_id"]
	if authID == "" {
		return fmt.Errorf("cloudns: need auth_id or sub_auth_id")
	}
	url := fmt.Sprintf("https://ipv4.cloudns.net/api/dynamicURL/?q=")
	if subID != "" {
		url = fmt.Sprintf("https://ipv4.cloudns.net/api/dynamicURL/?q=%s", subID)
	} else {
		url = fmt.Sprintf("https://ipv4.cloudns.net/api/dynamicURL/?q=%s&auth-id=%s&auth-password=%s", authID, authID, authPass)
	}
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}
