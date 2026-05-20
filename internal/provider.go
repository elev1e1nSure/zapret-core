package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

// ProviderInfo holds ISP identification data for ASN-based strategy selection
type ProviderInfo struct {
	ASN    string // e.g. "AS12389"
	Org    string // e.g. "Rostelecom"
	Region string // e.g. "77" (Moscow)
}

// ipinfoResponse represents the JSON response from ipinfo.io API
type ipinfoResponse struct {
	Org    string `json:"org"`    // "AS12389 Rostelecom"
	Region string `json:"region"` // "Moscow Oblast"
}

// GetProvider fetches current ISP info from ipinfo.io
// Returns unknown provider on any error — optimizer will still work, just without ASN hint
func GetProvider() ProviderInfo {
	client := &http.Client{Timeout: 5 * time.Second}

	resp, err := client.Get("https://ipinfo.io/json")
	if err != nil {
		return ProviderInfo{ASN: "unknown", Org: "unknown", Region: "unknown"}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ProviderInfo{ASN: "unknown", Org: "unknown", Region: "unknown"}
	}

	var info ipinfoResponse
	if err := json.Unmarshal(body, &info); err != nil {
		return ProviderInfo{ASN: "unknown", Org: "unknown", Region: "unknown"}
	}

	// org format: "AS12389 Rostelecom PJSC" — split on first space
	asn := "unknown"
	org := info.Org
	if parts := strings.SplitN(info.Org, " ", 2); len(parts) == 2 {
		asn = parts[0]
		org = parts[1]
	}

	logInfo("[provider] ASN: %s  Org: %s  Region: %s", asn, org, info.Region)

	return ProviderInfo{
		ASN:    asn,
		Org:    org,
		Region: info.Region,
	}
}
