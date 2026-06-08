package main

import (
	"fmt"
	"strings"
)

// StrategyVector holds all variable parameters for DPI bypass strategies
type StrategyVector struct {
	DesyncMethod    string
	RepeatsTCP      int
	RepeatsUDP      int
	Fooling         string
	SplitPos        string
	TLSMode         string
	TLSFiles        []string
	TLSMod          string
	SeqOvl          int
	SeqOvlPattern   string
	HostFakeMod     string
	Cutoff          string
	BadseqIncrement int
	QuicBin         string
	AnyProtocol     bool
	IPID            string
	AutoTTL         string
}

// SearchSpace defines all possible values for each parameter
// Used by optimizer to generate candidate vectors
var SearchSpace = struct {
	DesyncMethod    []string
	RepeatsTCP      []int
	RepeatsUDP      []int
	Fooling         []string
	SplitPos        []string
	TLSMode         []string
	TLSFiles        [][]string
	TLSMod          []string
	SeqOvl          []int
	SeqOvlPattern   []string
	HostFakeMod     []string
	Cutoff          []string
	BadseqIncrement []int
	QuicBin         []string
	IPID            []string
	AutoTTL         []string
}{
	DesyncMethod: []string{
		"fake", "fake,fakedsplit", "fake,multisplit", "fake,hostfakesplit",
		"fake,multidisorder", "syndata,multidisorder", "syndata", "hostfakesplit",
		"multidisorder", "disorder", "split", "multisplit", "disorder,split", "split2",
	},
	RepeatsTCP: []int{0, 4, 6, 8, 10, 11, 12, 14},
	RepeatsUDP: []int{4, 6, 10, 11, 12},
	Fooling:    []string{"", "ts", "badseq", "ts,md5sig", "badsum", "md5sig", "ts,badseq"},
	SplitPos:   []string{"1", "2", "1,2", "2,sniext+1", "1,sniext+1", "1,midsld", "10,midsld", "7,sld+1", "2,sld", "1,midsld,endhost-1"},
	TLSMode:    []string{"file", "tls-mod", "none"},
	TLSFiles: [][]string{
		{"tls_clienthello_www_google_com.bin"},
		{"stun.bin"},
		{"stun.bin", "tls_clienthello_www_google_com.bin"},
		{"stun.bin", "tls_clienthello_max_ru.bin"},
		{"tls_clienthello_4pda_to.bin"},
		{"tls_clienthello_max_ru.bin"},
	},
	TLSMod:          []string{"rnd,dupsid,sni=www.google.com", "rnd,dupsid,sni=ya.ru", "rnd,dupsid,sni=ggpht.com"},
	SeqOvl:          []int{0, 1, 4, 336, 568, 620, 652, 654, 664, 679, 681, 2108},
	SeqOvlPattern:   []string{"tls_clienthello_www_google_com.bin", "tls_clienthello_4pda_to.bin", "tls_clienthello_max_ru.bin", "stun.bin"},
	HostFakeMod:     []string{"www.google.com", "ya.ru", "ozon.ru"},
	Cutoff:          []string{"", "n2", "n3", "n4", "n5"},
	BadseqIncrement: []int{2, 1000, 10000000},
	QuicBin:         []string{"quic_initial_www_google_com.bin", "quic_initial_dbankcloud_ru.bin", "quic_initial_yandex_ru.bin"},
	IPID:            []string{"zero", ""},
	AutoTTL:         []string{"", "2:2-12", "2:1-10"},
}

// buildTLSArgs constructs TLS-related winws arguments from a strategy vector
func buildTLSArgs(v StrategyVector) []string {
	args := []string{}
	if !strings.Contains(v.DesyncMethod, "fake") {
		return args
	}
	switch v.TLSMode {
	case "file":
		for _, f := range v.TLSFiles {
			args = append(args, fmt.Sprintf("--dpi-desync-fake-tls=%s", fake(f)))
		}
	case "tls-mod":
		args = append(args, fmt.Sprintf("--dpi-desync-fake-tls-mod=%s", v.TLSMod))
	}
	return args
}

// buildTCPRule builds args for a generic TCP rule
func buildTCPRule(v StrategyVector) []string {
	// clean methods (split/disorder/multisplit etc.) don't use fake-specific params
	if !strings.Contains(v.DesyncMethod, "fake") {
		v.Fooling = ""
		v.RepeatsTCP = 0
		v.AutoTTL = ""
	}

	args := []string{}
	args = append(args, fmt.Sprintf("--dpi-desync=%s", v.DesyncMethod))

	if v.RepeatsTCP > 0 {
		args = append(args, fmt.Sprintf("--dpi-desync-repeats=%d", v.RepeatsTCP))
	}
	if v.Fooling != "" {
		args = append(args, fmt.Sprintf("--dpi-desync-fooling=%s", v.Fooling))
	}
	if v.SplitPos != "" {
		args = append(args, fmt.Sprintf("--dpi-desync-split-pos=%s", v.SplitPos))
	}
	if strings.Contains(v.Fooling, "badseq") && v.BadseqIncrement != 0 {
		args = append(args, fmt.Sprintf("--dpi-desync-badseq-increment=%d", v.BadseqIncrement))
	}
	if v.SeqOvl > 0 && (strings.Contains(v.DesyncMethod, "multisplit") || strings.Contains(v.DesyncMethod, "split2")) {
		args = append(args, fmt.Sprintf("--dpi-desync-split-seqovl=%d", v.SeqOvl))
		args = append(args, fmt.Sprintf("--dpi-desync-split-seqovl-pattern=%s", fake(v.SeqOvlPattern)))
	}
	if v.AutoTTL != "" {
		args = append(args, fmt.Sprintf("--dpi-desync-autottl=%s", v.AutoTTL))
	}
	if strings.Contains(v.DesyncMethod, "hostfakesplit") && v.HostFakeMod != "" {
		args = append(args, fmt.Sprintf("--dpi-desync-hostfakesplit-mod=host=%s,altorder=1", v.HostFakeMod))
	}
	args = append(args, buildTLSArgs(v)...)
	if strings.Contains(v.DesyncMethod, "fake") {
		args = append(args, fmt.Sprintf("--dpi-desync-fake-http=%s", fake("tls_clienthello_max_ru.bin")))
	}

	return args
}

// Generate converts a strategy vector into winws command-line arguments
func Generate(v StrategyVector) []string {
	args := []string{}

	// WinDivert port capture — fixed
	args = append(args,
		"--wf-tcp=80,443,2053,2083,2087,2096,8443",
		"--wf-udp=443,19294-19344,50000-50100",
	)

	// Rule 1: UDP 443 — general hostlist — QUIC fake
	args = append(args,
		"--filter-udp=443",
		fmt.Sprintf("--hostlist=%s", lists("list-general.txt")),
		fmt.Sprintf("--hostlist-exclude=%s", lists("list-exclude.txt")),
		fmt.Sprintf("--ipset-exclude=%s", lists("ipset-exclude.txt")),
		"--dpi-desync=fake",
		fmt.Sprintf("--dpi-desync-repeats=%d", v.RepeatsUDP),
		fmt.Sprintf("--dpi-desync-fake-quic=%s", fake(v.QuicBin)),
		"--new",
	)

	// Rule 2: UDP Discord/STUN — fixed fake bins
	args = append(args,
		"--filter-udp=19294-19344,50000-50100",
		"--filter-l7=discord,stun",
		"--dpi-desync=fake",
		fmt.Sprintf("--dpi-desync-fake-discord=%s", fake("quic_initial_dbankcloud_ru.bin")),
		fmt.Sprintf("--dpi-desync-fake-stun=%s", fake("quic_initial_dbankcloud_ru.bin")),
		fmt.Sprintf("--dpi-desync-repeats=%d", v.RepeatsUDP),
		"--new",
	)

	// Rule 3: TCP discord.media ports
	r3 := []string{
		"--filter-tcp=2053,2083,2087,2096,8443",
		"--hostlist-domains=discord.media",
	}
	r3 = append(r3, buildTCPRule(v)...)
	r3 = append(r3, "--new")
	args = append(args, r3...)

	// Rule 4: TCP 443 — Google hostlist
	r4 := []string{
		"--filter-tcp=443",
		fmt.Sprintf("--hostlist=%s", lists("list-google.txt")),
	}
	if v.IPID == "zero" {
		r4 = append(r4, "--ip-id=zero")
	}
	r4 = append(r4, buildTCPRule(v)...)
	r4 = append(r4, "--new")
	args = append(args, r4...)

	// Rule 5: TCP 80,443 — general hostlist
	r5 := []string{
		"--filter-tcp=80,443",
		fmt.Sprintf("--hostlist=%s", lists("list-general.txt")),
		fmt.Sprintf("--hostlist-exclude=%s", lists("list-exclude.txt")),
		fmt.Sprintf("--ipset-exclude=%s", lists("ipset-exclude.txt")),
	}
	r5 = append(r5, buildTCPRule(v)...)
	r5 = append(r5, "--new")
	args = append(args, r5...)

	// Rule 6: UDP 443 — ipset-all — QUIC fake
	args = append(args,
		"--filter-udp=443",
		fmt.Sprintf("--ipset=%s", lists("ipset-all.txt")),
		fmt.Sprintf("--hostlist-exclude=%s", lists("list-exclude.txt")),
		fmt.Sprintf("--ipset-exclude=%s", lists("ipset-exclude.txt")),
		"--dpi-desync=fake",
		fmt.Sprintf("--dpi-desync-repeats=%d", v.RepeatsUDP),
		fmt.Sprintf("--dpi-desync-fake-quic=%s", fake(v.QuicBin)),
		"--new",
	)

	// Rule 7: TCP 80,443,8443 — ipset-all
	r7 := []string{
		"--filter-tcp=80,443,8443",
		fmt.Sprintf("--ipset=%s", lists("ipset-all.txt")),
		fmt.Sprintf("--hostlist-exclude=%s", lists("list-exclude.txt")),
		fmt.Sprintf("--ipset-exclude=%s", lists("ipset-exclude.txt")),
	}
	r7 = append(r7, buildTCPRule(v)...)
	r7 = append(r7, "--new")
	args = append(args, r7...)

	// Rule 8: TCP GameFilter — ipset-all — any-protocol
	r8 := []string{
		"--filter-tcp=1024-65535", // GameFilter default
		fmt.Sprintf("--ipset=%s", lists("ipset-all.txt")),
		fmt.Sprintf("--ipset-exclude=%s", lists("ipset-exclude.txt")),
	}
	if v.AnyProtocol {
		r8 = append(r8, "--dpi-desync-any-protocol=1")
	}
	if v.Cutoff != "" {
		r8 = append(r8, fmt.Sprintf("--dpi-desync-cutoff=%s", v.Cutoff))
	}
	r8 = append(r8, buildTCPRule(v)...)
	r8 = append(r8, "--new")
	args = append(args, r8...)

	// Rule 9: UDP GameFilter — ipset-all — any-protocol
	args = append(args,
		"--filter-udp=1024-65535",
		fmt.Sprintf("--ipset=%s", lists("ipset-all.txt")),
		fmt.Sprintf("--ipset-exclude=%s", lists("ipset-exclude.txt")),
		"--dpi-desync=fake",
		fmt.Sprintf("--dpi-desync-repeats=%d", v.RepeatsUDP),
	)
	if v.AnyProtocol {
		args = append(args, "--dpi-desync-any-protocol=1")
	}
	if v.Cutoff != "" {
		args = append(args, fmt.Sprintf("--dpi-desync-cutoff=%s", v.Cutoff))
	}
	args = append(args,
		fmt.Sprintf("--dpi-desync-fake-unknown-udp=%s", fake("quic_initial_dbankcloud_ru.bin")),
	)

	return args
}

// VectorToStrategy converts a vector to a named Strategy for display
func VectorToStrategy(v StrategyVector, id int) *Strategy {
	return &Strategy{
		Name: fmt.Sprintf("auto-%d [%s/%s/%s]", id, v.DesyncMethod, v.Fooling, v.TLSMode),
		Args: Generate(v),
	}
}
